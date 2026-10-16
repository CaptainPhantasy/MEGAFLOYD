#!/usr/bin/env python3
"""Floyd Code auth backend.

Serves, behind a TLS fronting proxy:
  - RFC 8628 device authorization grant for the `f7` CLI:
      POST /api/oauth/device_authorization
      POST /api/oauth/token            (device_code + refresh_token grants)
  - the managed API surface the client calls:
      GET  /me  /usages  /models   POST /feedback  /feedback/upload_url
      /feedback/upload_complete  /tools      (stub) /search /fetch
  - a server-rendered admin UI (Basic auth + CSRF nonces) for user and
    device-approval management.

Storage: SQLite (default journal mode). The service binds loopback only.
"""

import base64
import hashlib
import hmac
import html
import json
import logging
import os
import secrets
import sqlite3
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

BASE_DIR = Path(__file__).resolve().parent
DATA_DIR = BASE_DIR / "data"
UPLOAD_DIR = BASE_DIR / "uploads"
DB_PATH = DATA_DIR / "floyd-auth.db"
CONFIG_PATH = BASE_DIR / "config.json"

BIND_HOST = os.environ.get("FLOYD_AUTH_BIND", "127.0.0.1")
BIND_PORT = int(os.environ.get("FLOYD_AUTH_PORT", "18443"))

DEVICE_CODE_TTL = 15 * 60
POLL_INTERVAL = 5
ACCESS_TOKEN_TTL = 30 * 24 * 3600
REFRESH_TOKEN_TTL = 365 * 24 * 3600
CLIENT_ID_KNOWN = "17e5f671-d194-4dfb-9706-5516cb48c098"

log = logging.getLogger("floyd-auth")


def load_config():
    with open(CONFIG_PATH, "r", encoding="utf-8") as fh:
        return json.load(fh)


CONFIG = load_config()
PUBLIC_BASE = CONFIG.get("public_base_url", "").rstrip("/")
MODELS = CONFIG.get(
    "models",
    [
        {
            "id": "floyd-managed",
            "display_name": "Floyd Managed",
            "context_length": 200000,
            "supports_reasoning": False,
            "supports_image_in": False,
            "supports_video_in": False,
            "supports_tool_use": True,
            "supports_dynamic_tools": False,
        }
    ],
)

PBKDF2_ITERATIONS = 240_000


def verify_admin_password(password: str) -> bool:
    current = load_config()
    salt = bytes.fromhex(current["password_salt"])
    expected = bytes.fromhex(current["password_hash"])
    candidate = hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt, PBKDF2_ITERATIONS)
    return hmac.compare_digest(candidate, expected)


def csrf_token(ttl: int = 3600) -> str:
    exp = int(time.time()) + ttl
    mac = hmac.new(
        CONFIG["csrf_secret"].encode("utf-8"), f"csrf:{exp}".encode("utf-8"), hashlib.sha256
    ).hexdigest()
    return f"{exp}:{mac}"


def csrf_valid(token: str) -> bool:
    try:
        exp_raw, mac = token.split(":", 1)
        exp = int(exp_raw)
    except (ValueError, AttributeError):
        return False
    if exp < int(time.time()):
        return False
    expected = hmac.new(
        CONFIG["csrf_secret"].encode("utf-8"), f"csrf:{exp}".encode("utf-8"), hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(mac, expected)


def sha256_hex(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def now() -> int:
    return int(time.time())


def iso(ts: float | None = None) -> str:
    if ts is None:
        ts = time.time()
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(ts))


USER_CODE_ALPHABET = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"


def new_user_code() -> str:
    chars = "".join(secrets.choice(USER_CODE_ALPHABET) for _ in range(8))
    return f"{chars[:4]}-{chars[4:]}"


def db() -> sqlite3.Connection:
    conn = sqlite3.connect(DB_PATH, timeout=15)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA busy_timeout = 15000")
    conn.execute("PRAGMA foreign_keys = ON")
    return conn


SCHEMA = """
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE COLLATE NOCASE,
    note TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    last_login_at INTEGER
);
CREATE TABLE IF NOT EXISTS deleted_users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at INTEGER,
    deleted_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS device_codes (
    device_code TEXT PRIMARY KEY,
    user_code TEXT NOT NULL UNIQUE,
    client_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    platform TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    approved_at INTEGER,
    denied_at INTEGER,
    exchanged_at INTEGER,
    last_poll_at INTEGER
);
CREATE TABLE IF NOT EXISTS access_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    revoked INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS refresh_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    revoked INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_access_user ON access_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_user ON refresh_tokens(user_id);
CREATE TABLE IF NOT EXISTS kv (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
"""


def init_db():
    DATA_DIR.mkdir(parents=True, exist_ok=True)
    UPLOAD_DIR.mkdir(parents=True, exist_ok=True)
    conn = db()
    try:
        conn.executescript(SCHEMA)
        conn.commit()
    finally:
        conn.close()


def gc_sweep():
    while True:
        try:
            t = now()
            conn = db()
            try:
                conn.execute(
                    "UPDATE device_codes SET status='expired'"
                    " WHERE status='pending' AND expires_at < ?",
                    (t,),
                )
                conn.execute(
                    "DELETE FROM device_codes WHERE expires_at < ?", (t - 7 * 24 * 3600,)
                )
                conn.execute(
                    "DELETE FROM access_tokens WHERE expires_at < ?", (t - 30 * 24 * 3600,)
                )
                conn.execute(
                    "DELETE FROM refresh_tokens WHERE expires_at < ? AND revoked=1",
                    (t - 30 * 24 * 3600,),
                )
                conn.commit()
            finally:
                conn.close()
        except Exception:
            log.exception("gc sweep failed")
        time.sleep(60)


# ── helpers shared by handlers ──────────────────────────────────────────


class ApiError(Exception):
    def __init__(self, status, payload):
        self.status = status
        self.payload = payload


def issue_tokens(conn, user_id, device_code=None):
    access = "fc_" + secrets.token_urlsafe(32)
    refresh = "fr_" + secrets.token_urlsafe(40)
    t = now()
    conn.execute(
        "INSERT INTO access_tokens(token_hash,user_id,created_at,expires_at,device_code) VALUES(?,?,?,?,?)",
        (sha256_hex(access), user_id, t, t + ACCESS_TOKEN_TTL, device_code),
    )
    conn.execute(
        "INSERT INTO refresh_tokens(token_hash,user_id,created_at,expires_at,device_code) VALUES(?,?,?,?,?)",
        (sha256_hex(refresh), user_id, t, t + REFRESH_TOKEN_TTL, device_code),
    )
    return {
        "access_token": access,
        "token_type": "Bearer",
        "expires_in": ACCESS_TOKEN_TTL,
        "refresh_token": refresh,
        "scope": "floyd-code",
    }


def bearer_token(headers) -> str | None:
    auth = headers.get("Authorization") or headers.get("authorization") or ""
    if auth.lower().startswith("bearer "):
        return auth[7:].strip()
    return None


def user_for_access_token(conn, token: str):
    if not token:
        return None
    row = conn.execute(
        "SELECT u.* FROM access_tokens a JOIN users u ON u.id = a.user_id"
        " WHERE a.token_hash = ? AND a.revoked = 0 AND a.expires_at > ? AND u.enabled = 1",
        (sha256_hex(token), now()),
    ).fetchone()
    return row


def require_bearer_user(conn, headers):
    token = bearer_token(headers)
    user = user_for_access_token(conn, token or "")
    if user is None:
        raise ApiError(
            401,
            {"error": "invalid_token", "error_description": "Access token is invalid or expired."},
        )
    return user


def user_payload(user) -> dict:
    return {
        "user_id": user["id"],
        "nickname": user["username"],
        "status": "USER_STATUS_NORMAL",
        "region": "REGION_CN",
        "user_level": 1,
        "user_level_name": "Floyd",
        "domain": 1,
        "domain_name": "DOMAIN_FLOYD",
        "global_id": user["id"],
        "username": user["username"],
        "email": f"{user['username']}@floyd.local",
        "created_time": iso(user["created_at"]),
        "last_login_time": iso(user["last_login_at"]) if user["last_login_at"] else None,
    }


# ── OAuth handlers (contract from packages/oauth/src/oauth.ts) ─────────


def handle_device_authorization(form, headers):
    client_id = (form.get("client_id") or [""])[0].strip()
    if not client_id:
        return 400, {"error": "invalid_client", "error_description": "client_id is required."}
    platform = headers.get("X-Msh-Platform") or headers.get("x-msh-platform") or ""
    device_code = "dc_" + secrets.token_urlsafe(32)
    user_code = new_user_code()
    t = now()
    approve_url = f"{PUBLIC_BASE}/approve?user_code={user_code}"
    conn = db()
    try:
        conn.execute(
            "INSERT INTO device_codes(device_code,user_code,client_id,status,platform,"
            "created_at,expires_at) VALUES(?,?,?,'pending',?,?,?)",
            (device_code, user_code, client_id, platform, t, t + DEVICE_CODE_TTL),
        )
        conn.commit()
    finally:
        conn.close()
    log.info("device_authorization: user_code=%s platform=%s", user_code, platform)
    return (
        200,
        {
            "device_code": device_code,
            "user_code": user_code,
            "verification_uri": f"{PUBLIC_BASE}/approve",
            "verification_uri_complete": approve_url,
            "expires_in": DEVICE_CODE_TTL,
            "interval": POLL_INTERVAL,
        },
    )


def handle_token(form, headers):
    grant_type = (form.get("grant_type") or [""])[0]
    client_id = (form.get("client_id") or [""])[0].strip()
    if not client_id:
        return 400, {"error": "invalid_client", "error_description": "client_id is required."}
    if grant_type == "urn:ietf:params:oauth:grant-type:device_code":
        return token_device_grant(form)
    if grant_type == "refresh_token":
        return token_refresh_grant(form)
    return (
        400,
        {"error": "unsupported_grant_type", "error_description": f"Unsupported grant: {grant_type}"},
    )


def token_device_grant(form):
    device_code = (form.get("device_code") or [""])[0].strip()
    t = now()
    conn = db()
    try:
        conn.execute("BEGIN IMMEDIATE")
        row = conn.execute(
            "SELECT * FROM device_codes WHERE device_code = ?", (device_code,)
        ).fetchone()
        if row is None:
            return 400, {"error": "invalid_grant", "error_description": "Unknown device_code."}
        if row["client_id"] != (form.get("client_id") or [""])[0].strip():
            return 400, {"error": "invalid_grant", "error_description": "Device code belongs to a different client."}
        conn.execute(
            "UPDATE device_codes SET last_poll_at = ? WHERE device_code = ?", (t, device_code)
        )
        if row["expires_at"] <= t:
            conn.execute(
                "UPDATE device_codes SET status='expired' WHERE device_code=?", (device_code,)
            )
            conn.commit()
            return 400, {"error": "expired_token", "error_description": "Device code expired."}
        if row["status"] == "denied":
            return (
                400,
                {"error": "access_denied", "error_description": "Authorization was denied."},
            )
        if row["status"] == "exchanged":
            return (
                400,
                {
                    "error": "expired_token",
                    "error_description": "Device code already redeemed.",
                },
            )
        if row["status"] == "approved" and row["user_id"] is not None:
            user = conn.execute(
                "SELECT * FROM users WHERE id = ? AND enabled = 1", (row["user_id"],)
            ).fetchone()
            if user is not None:
                conn.execute(
                    "UPDATE device_codes SET status='exchanged', exchanged_at=?"
                    " WHERE device_code=?",
                    (t, device_code),
                )
                conn.execute(
                    "UPDATE users SET last_login_at = ? WHERE id = ?", (t, user["id"])
                )
                payload = issue_tokens(conn, user["id"], device_code)
                conn.commit()
                log.info("token: issued to user=%s via user_code=%s", user["username"], row["user_code"])
                return 200, payload
            return (
                400,
                {
                    "error": "access_denied",
                    "error_description": "Approving user is disabled.",
                },
            )
        conn.commit()
        last = row["last_poll_at"]
        if last is not None and t - last < POLL_INTERVAL - 1:
            return 400, {"error": "slow_down", "error_description": "Poll too frequently."}
        return 400, {"error": "authorization_pending", "error_description": "Awaiting approval."}
    finally:
        conn.close()


def token_refresh_grant(form):
    refresh_token = (form.get("refresh_token") or [""])[0].strip()
    conn = db()
    try:
        conn.execute("BEGIN IMMEDIATE")
        row = conn.execute(
            "SELECT u.* , r.token_hash AS rhash, r.device_code AS token_device FROM refresh_tokens r"
            " JOIN users u ON u.id = r.user_id"
            " WHERE r.token_hash = ? AND r.revoked = 0 AND r.expires_at > ? AND u.enabled = 1",
            (sha256_hex(refresh_token), now()),
        ).fetchone()
        if row is None:
            return (
                400,
                {
                    "error": "invalid_grant",
                    "error_description": "Refresh token is invalid, expired, or revoked.",
                },
            )
        if row["token_device"]:
            device = conn.execute("SELECT client_id FROM device_codes WHERE device_code=?", (row["token_device"],)).fetchone()
            if device and device["client_id"] != (form.get("client_id") or [""])[0].strip():
                return 400, {"error": "invalid_grant", "error_description": "Sign-in belongs to a different client."}
        conn.execute("UPDATE refresh_tokens SET revoked=1 WHERE token_hash=?", (row["rhash"],))
        payload = issue_tokens(conn, row["id"], row["token_device"])
        conn.commit()
        return 200, payload
    finally:
        conn.close()


# ── managed API handlers ────────────────────────────────────────────────


def handle_me(conn, headers):
    user = require_bearer_user(conn, headers)
    return 200, user_payload(user)


def handle_usages(conn, headers):
    require_bearer_user(conn, headers)
    zero = {"used_ratio": 0.0}
    return (
        200,
        {
            "usages": {
                "limit_5h": dict(zero, reset_time=iso(now() + 5 * 3600)),
                "limit_7d": dict(zero, reset_time=iso(now() + 7 * 24 * 3600)),
                "limit_month_total": dict(zero, reset_time=iso(now() + 30 * 24 * 3600)),
                "limit_month_code": dict(zero, reset_time=iso(now() + 30 * 24 * 3600)),
            },
            "boosterWallet": None,
        },
    )


def handle_models(conn, headers):
    require_bearer_user(conn, headers)
    return 200, {"data": PLATFORM.managed_models()}


def handle_feedback(conn, headers, body):
    user = require_bearer_user(conn, headers)
    seq_row = conn.execute("SELECT value FROM kv WHERE key='feedback_seq'").fetchone()
    seq = int(seq_row["value"]) + 1 if seq_row else 1
    conn.execute(
        "INSERT INTO kv(key,value) VALUES('feedback_seq',?)"
        " ON CONFLICT(key) DO UPDATE SET value=excluded.value",
        (str(seq),),
    )
    conn.commit()
    log.info("feedback #%s from %s", seq, user["username"])
    return 200, {"feedback_id": seq}


def handle_upload_url(conn, headers, body):
    user = require_bearer_user(conn, headers)
    feedback_id = body.get("feedback_id") if isinstance(body, dict) else None
    if not isinstance(feedback_id, int):
        return 400, {"error": "bad_request", "error_description": "feedback_id must be an integer."}
    upload_id = int(time.time() * 1000) % 1_000_000_000
    part_url = f"{PUBLIC_BASE}/feedback/upload/{upload_id}"
    log.info("feedback upload_url id=%s feedback=%s user=%s", upload_id, feedback_id, user["username"])
    return (
        200,
        {
            "upload": {
                "id": upload_id,
                "parts": [
                    {"part_number": 1, "url": part_url, "method": "PUT", "size": 10 * 1024 * 1024}
                ],
            }
        },
    )


def handle_upload_complete(conn, headers, body):
    require_bearer_user(conn, headers)
    return 200, {}


def handle_tools(conn, headers, body):
    require_bearer_user(conn, headers)
    method = body.get("method") if isinstance(body, dict) else None
    if method == "chat_title":
        content = ""
        params = body.get("params")
        if isinstance(params, dict):
            content = str(params.get("chat_content") or "")
        first_line = content.strip().splitlines()[0] if content.strip() else "Session"
        title = first_line.strip()[:48] or "Session"
        return 200, {"title": title}
    raise ApiError(404, {"error": "unknown_tool", "error_description": "That company tool does not exist."})


def handle_stub(conn, headers, path):
    require_bearer_user(conn, headers)
    raise ApiError(503, {"error": "tool_unavailable", "error_description": "The company tool connection is not configured."})


# ── admin logic ─────────────────────────────────────────────────────────


def admin_check(headers) -> bool:
    auth = headers.get("Authorization") or headers.get("authorization") or ""
    if auth.lower().startswith("basic "):
        try:
            decoded = base64.b64decode(auth[6:].strip()).decode("utf-8")
            _, _, password = decoded.partition(":")
        except Exception:
            return False
        return verify_admin_password(password)
    if auth.lower().startswith("bearer "):
        return verify_admin_password(auth[7:].strip())
    return False


def create_user(conn, username: str, note: str = ""):
    username = username.strip()
    if not username or len(username) > 64:
        raise ApiError(400, {"error": "bad_username", "error_description": "Username required (<=64 chars)."})
    existing = conn.execute("SELECT id FROM users WHERE username = ?", (username,)).fetchone()
    if existing:
        raise ApiError(409, {"error": "conflict", "error_description": "Username already exists."})
    user_id = "u_" + secrets.token_hex(8)
    conn.execute(
        "INSERT INTO users(id,username,note,enabled,created_at) VALUES(?,?,?,1,?)",
        (user_id, username, note.strip(), now()),
    )
    conn.commit()
    return conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()


def device_row_public(row) -> dict:
    return {
        "device_code": row["device_code"],
        "user_code": row["user_code"],
        "status": row["status"],
        "platform": row["platform"],
        "created_at": iso(row["created_at"]),
        "expires_at": iso(row["expires_at"]),
        "approved_for": row["user_id"],
    }


def user_row_public(row) -> dict:
    return {
        "id": row["id"],
        "username": row["username"],
        "note": row["note"],
        "enabled": bool(row["enabled"]),
        "created_at": iso(row["created_at"]),
        "last_login_at": iso(row["last_login_at"]) if row["last_login_at"] else None,
    }


# ── HTTP plumbing ───────────────────────────────────────────────────────


class Handler(BaseHTTPRequestHandler):
    server_version = "floyd-auth/1.0"
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        cleaned = tuple(value.replace(self.path, urlparse(self.path).path) if isinstance(value, str) else value for value in args)
        log.info("%s " + fmt, self.address_string(), *cleaned)

    # helpers
    def send_json(self, status: int, payload, extra_headers=None):
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.security_headers()
        for key, value in (extra_headers or {}).items():
            self.send_header(key, value)
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def send_html(self, status: int, markup: str, extra_headers=None):
        body = markup.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.security_headers()
        for key, value in (extra_headers or {}).items():
            self.send_header(key, value)
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def security_headers(self):
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("X-Frame-Options", "DENY")
        self.send_header("Referrer-Policy", "no-referrer")
        self.send_header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")

    def read_body(self) -> bytes:
        if hasattr(self, "_body"):
            return self._body
        try:
            length = int(self.headers.get("Content-Length") or 0)
        except ValueError:
            raise ApiError(400, {"error": "bad_length", "error_description": "Invalid request size."})
        if length <= 0:
            return b""
        if length > 32 * 1024 * 1024:
            raise ApiError(413, {"error": "too_large", "error_description": "Body too large."})
        self.connection.settimeout(30)
        self._body = self.rfile.read(length)
        if len(self._body) != length:
            raise ApiError(400, {"error": "incomplete_body", "error_description": "Request was interrupted."})
        return self._body

    def read_form(self) -> dict:
        raw = self.read_body().decode("utf-8", "replace")
        return parse_qs(raw, keep_blank_values=True)

    def read_json(self):
        raw = self.read_body()
        if not raw:
            return {}
        try:
            payload = json.loads(raw)
            if not isinstance(payload, dict):
                raise ApiError(400, {"error": "bad_json", "error_description": "A request must contain named fields."})
            return payload
        except json.JSONDecodeError:
            raise ApiError(400, {"error": "bad_json", "error_description": "Invalid JSON body."})

    def dispatch(self, method: str):
        self.__dict__.pop('_body', None)
        parsed = urlparse(self.path)
        path = parsed.path.rstrip("/") or "/"
        query = parse_qs(parsed.query)
        try:
            status, payload, kind, extra = self.route(method, path, query)
        except ApiError as exc:
            status, payload, kind, extra = exc.status, exc.payload, "json", {}
        except Exception:
            log.exception("unhandled error for %s %s", method, path)
            status, payload, kind, extra = (
                500,
                {"error": "internal", "error_description": "Internal server error."},
                "json",
                {},
            )
        if kind == "sent":
            return
        if kind == "html":
            self.send_html(status, payload, extra)
        elif kind == "text":
            raw = payload.encode('utf-8')
            self.send_response(status)
            self.send_header('Content-Type', 'text/plain; charset=utf-8')
            self.send_header('Content-Length', str(len(raw)))
            self.send_header('Cache-Control', 'no-store')
            self.security_headers()
            self.end_headers()
            if self.command != 'HEAD':
                self.wfile.write(raw)
        else:
            self.send_json(status, payload, extra)

    def do_GET(self):
        self.dispatch("GET")

    def do_POST(self):
        self.dispatch("POST")

    def do_PUT(self):
        self.dispatch("PUT")

    def do_DELETE(self):
        self.dispatch("DELETE")

    def do_HEAD(self):
        self.dispatch("GET")

    # routing
    def route(self, method, path, query):
        handled = PLATFORM.route(self, method, path, query)
        if handled is not None:
            return handled
        if method == "POST" and path == "/api/oauth/device_authorization":
            return (*handle_device_authorization(self.read_form(), self.headers), "json", {})
        if method == "POST" and path == "/api/oauth/token":
            payload = handle_token(self.read_form(), self.headers)
            return (*payload, "json", {})
        if path.startswith("/feedback/upload/"):
            self.read_body()
            return 200, {}, "json", {}
        if method == "GET" and path == "/healthz":
            return 200, {"ok": True}, "json", {}

        managed = {
            "/me": lambda: handle_me(conn, self.headers),
            "/usages": lambda: handle_usages(conn, self.headers),
            "/models": lambda: handle_models(conn, self.headers),
        }
        conn = db()
        try:
            if method == "GET" and path in managed:
                status, payload = managed[path]()
                return status, payload, "json", {}
            if method == "POST" and path == "/feedback":
                return (*handle_feedback(conn, self.headers, self.read_json()), "json", {})
            if method == "POST" and path == "/feedback/upload_url":
                return (*handle_upload_url(conn, self.headers, self.read_json()), "json", {})
            if method == "POST" and path == "/feedback/upload_complete":
                return (*handle_upload_complete(conn, self.headers, self.read_json()), "json", {})
            if method == "POST" and path == "/tools":
                return (*handle_tools(conn, self.headers, self.read_json()), "json", {})
            if path in ("/search", "/fetch"):
                return (*handle_stub(conn, self.headers, path), "json", {})
            if path.startswith("/admin") or path == "/approve":
                return self.route_admin(method, path, query, conn)
            return (
                404,
                {"error": "not_found", "error_description": f"No route for {method} {path}"},
                "json",
                {},
            )
        finally:
            conn.close()

    # admin routes
    def route_admin(self, method, path, query, conn):
        if path == "/approve" and method == "GET":
            if not PLATFORM.is_admin(self):
                return self.admin_challenge()
            return self.page_approve(query.get("user_code", [""])[0], conn)
        if not PLATFORM.is_admin(self):
            if path.startswith("/admin/api/"):
                raise ApiError(
                    401, {"error": "unauthorized", "error_description": "Admin credential required."}
                )
            return self.admin_challenge()

        if method == "GET" and path == "/admin":
            return self.page_admin(conn)
        if method == "GET" and path == "/admin/api/users":
            rows = conn.execute("SELECT * FROM users ORDER BY created_at").fetchall()
            return 200, {"users": [user_row_public(r) for r in rows]}, "json", {}
        if method == "POST" and path == "/admin/api/users":
            body = self.read_json()
            row = create_user(conn, str(body.get("username", "")), str(body.get("note", "")))
            return 201, {"user": user_row_public(row)}, "json", {}
        if method == "POST" and path.startswith("/admin/api/users/") and path.endswith("/enable"):
            return self.user_toggle(conn, path.split("/")[4], 1)
        if method == "POST" and path.startswith("/admin/api/users/") and path.endswith("/disable"):
            return self.user_toggle(conn, path.split("/")[4], 0)
        if method == "DELETE" and path.startswith("/admin/api/users/"):
            return self.user_delete(conn, path.split("/")[4])
        if method == "GET" and path == "/admin/api/devices":
            status_filter = (query.get("status") or ["pending"])[0]
            if status_filter == "all":
                rows = conn.execute(
                    "SELECT * FROM device_codes ORDER BY created_at DESC LIMIT 100"
                ).fetchall()
            else:
                rows = conn.execute(
                    "SELECT * FROM device_codes WHERE status = ? ORDER BY created_at DESC LIMIT 100",
                    (status_filter,),
                ).fetchall()
            return 200, {"devices": [device_row_public(r) for r in rows]}, "json", {}
        if method == "POST" and path.startswith("/admin/api/devices/") and path.endswith("/approve"):
            device_code = path.split("/")[4].removesuffix("/approve")
            body = self.read_json()
            return self.device_approve(conn, device_code, str(body.get("user_id", "")), body)
        if method == "POST" and path.startswith("/admin/api/devices/") and path.endswith("/deny"):
            device_code = path.split("/")[4].removesuffix("/deny")
            return self.device_deny(conn, device_code)

        # HTML form actions
        form = None
        if method == "POST":
            form = self.read_form()
            nonce = (form.get("csrf") or [""])[0]
            if not csrf_valid(nonce):
                raise ApiError(403, {"error": "csrf", "error_description": "Invalid form token."})
        if method == "POST" and path == "/admin/users":
            username = (form.get("username") or [""])[0]
            note = (form.get("note") or [""])[0]
            try:
                create_user(conn, username, note)
            except ApiError as exc:
                return self.page_admin(conn, banner=f"Error: {exc.payload.get('error_description')}")
            return self.page_admin(conn, banner=f"Created user {username!r}.")
        parts = path.split("/")
        if method == "POST" and len(parts) == 5 and parts[1] == "admin" and parts[2] == "users":
            user_id, action = parts[3], parts[4]
            if action in ("enable", "disable"):
                conn.execute(
                    "UPDATE users SET enabled = ? WHERE id = ?", (1 if action == "enable" else 0, user_id)
                )
                conn.commit()
                return self.page_admin(conn, banner=f"User {user_id} {action}d.")
            if action == "delete":
                row = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
                if row is not None:
                    conn.execute(
                        "INSERT OR REPLACE INTO deleted_users(id,username,note,created_at,deleted_at)"
                        " VALUES(?,?,?,?,?)",
                        (row["id"], row["username"], row["note"], row["created_at"], now()),
                    )
                    conn.execute("DELETE FROM users WHERE id = ?", (user_id,))
                    conn.commit()
                return self.page_admin(conn, banner=f"Deleted user {row['username'] if row else user_id}.")
        if method == "POST" and len(parts) == 5 and parts[1] == "admin" and parts[2] == "devices":
            device_code, action = parts[3], parts[4]
            if action == "approve":
                user_id = (form.get("user_id") or [""])[0]
                new_username = (form.get("new_username") or [""])[0]
                override = {"user_id": user_id}
                if new_username.strip():
                    override["new_username"] = new_username.strip()
                status, payload = self.device_approve(conn, device_code, user_id, override)
                banner = payload.get("error_description") or (
                    f"Approved {payload.get('user_code')} for {payload.get('username')}."
                )
                return self.page_admin(conn, banner=("Error: " if status != 200 else "") + str(banner))
            if action == "deny":
                status, payload = self.device_deny(conn, device_code)
                banner = payload.get("error_description") or f"Denied {payload.get('user_code')}."
                return self.page_admin(conn, banner=("Error: " if status != 200 else "") + str(banner))
        return (
            404,
            {"error": "not_found", "error_description": f"No admin route {method} {path}"},
            "json",
            {},
        )

    def admin_challenge(self):
        body = (
            "<html><head><title>Floyd auth admin</title></head><body>"
            "<h3>Admin authentication required.</h3>"
            "<p>Use the admin username/password (HTTP Basic) for this host.</p>"
            "</body></html>"
        )
        return 401, body, "html", {"WWW-Authenticate": 'Basic realm="floyd-auth admin"'}

    def user_toggle(self, conn, user_id, enabled):
        cur = conn.execute("UPDATE users SET enabled = ? WHERE id = ?", (enabled, user_id))
        if cur.rowcount == 0:
            raise ApiError(404, {"error": "not_found", "error_description": "No such user."})
        if not enabled:
            PLATFORM.cancel_access(conn, user_id)
        PLATFORM.audit(conn, "account enabled" if enabled else "account disabled", user_id)
        conn.commit()
        row = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
        return 200, {"user": user_row_public(row)}, "json", {}

    def user_delete(self, conn, user_id):
        row = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
        if row is None:
            raise ApiError(404, {"error": "not_found", "error_description": "No such user."})
        conn.execute(
            "INSERT OR REPLACE INTO deleted_users(id,username,note,created_at,deleted_at)"
            " VALUES(?,?,?,?,?)",
            (row["id"], row["username"], row["note"], row["created_at"], now()),
        )
        conn.execute("DELETE FROM users WHERE id = ?", (user_id,))
        conn.commit()
        return 200, {"deleted": row["username"], "id": user_id}, "json", {}

    def device_approve(self, conn, device_code, user_id, body):
        if not conn.in_transaction:
            conn.execute("BEGIN IMMEDIATE")
        row = conn.execute(
            "SELECT * FROM device_codes WHERE device_code = ?", (device_code,)
        ).fetchone()
        if row is None:
            raise ApiError(404, {"error": "not_found", "error_description": "Unknown device code."})
        t = now()
        if row["status"] != "pending" or row["expires_at"] <= t:
            raise ApiError(
                409,
                {
                    "error": "not_pending",
                    "error_description": f"Device code is {row['status']} or expired.",
                },
            )
        username = None
        if not user_id and str(body.get("new_username", "")).strip():
            new_row = create_user(conn, str(body.get("new_username")))
            user_id = new_row["id"]
            username = new_row["username"]
        user = conn.execute(
            "SELECT * FROM users WHERE id = ? AND enabled = 1", (user_id,)
        ).fetchone()
        if user is None:
            raise ApiError(404, {"error": "not_found", "error_description": "No such enabled user."})
        changed = conn.execute(
            "UPDATE device_codes SET status='approved', user_id=?, approved_at=?"
            " WHERE device_code=? AND status='pending' AND expires_at>?",
            (user_id, t, device_code, t),
        )
        if changed.rowcount != 1:
            raise ApiError(409, {"error": "not_pending", "error_description": "This code was already used."})
        conn.commit()
        log.info("device approved: user_code=%s user=%s", row["user_code"], user["username"])
        return (
            200,
            {
                "approved": True,
                "user_code": row["user_code"],
                "user_id": user_id,
                "username": username or user["username"],
            },
            "json",
            {},
        )

    def device_deny(self, conn, device_code):
        row = conn.execute(
            "SELECT * FROM device_codes WHERE device_code = ?", (device_code,)
        ).fetchone()
        if row is None:
            raise ApiError(404, {"error": "not_found", "error_description": "Unknown device code."})
        if row["user_id"]:
            PLATFORM.cancel_access(conn, row["user_id"], device_code)
        conn.execute(
            "UPDATE device_codes SET status='denied', denied_at=? WHERE device_code=?",
            (now(), device_code),
        )
        conn.commit()
        log.info("device denied: user_code=%s", row["user_code"])
        return 200, {"denied": True, "user_code": row["user_code"]}, "json", {}

    # ── HTML pages ──────────────────────────────────────────────────────

    PAGE_CSS = """
    body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;max-width:960px;
      margin:2rem auto;padding:0 1rem;color:#1a1a2e;background:#f7f7fb}
    h1{font-size:1.4rem} h2{font-size:1.1rem;margin-top:2rem}
    table{border-collapse:collapse;width:100%;margin:0.75rem 0;background:#fff}
    th,td{border:1px solid #ddd;padding:0.45rem 0.6rem;text-align:left;font-size:0.92rem}
    th{background:#eef} code{background:#eee;padding:0.1rem 0.35rem;border-radius:4px}
    .banner{background:#e7f7e7;border:1px solid #9c9;padding:0.5rem 0.75rem;border-radius:6px}
    .banner.err{background:#fdecec;border-color:#c99}
    form.inline{display:inline}
    button{cursor:pointer}
    .muted{color:#667;font-size:0.85rem}
    input[type=text]{padding:0.35rem;border:1px solid #bbb;border-radius:4px}
    """

    def page(self, title: str, body: str) -> str:
        return (
            "<!doctype html><html><head><meta charset='utf-8'>"
            f"<title>{html.escape(title)}</title>"
            f"<style>{self.PAGE_CSS}</style></head><body>{body}"
            "<p class='muted'>floyd-auth admin</p></body></html>"
        )

    def page_admin(self, conn, banner: str | None = None, error: bool = False):
        users = conn.execute("SELECT * FROM users ORDER BY created_at").fetchall()
        pending = conn.execute(
            "SELECT * FROM device_codes WHERE status='pending' ORDER BY created_at DESC"
        ).fetchall()
        recent = conn.execute(
            "SELECT * FROM device_codes WHERE status!='pending' ORDER BY created_at DESC LIMIT 15"
        ).fetchall()
        nonce = csrf_token()
        parts = []
        if banner:
            parts.append(f"<p class='banner{' err' if error else ''}'>{html.escape(banner)}</p>")
        parts.append(
            "<h1>Floyd Code auth admin</h1>"
            f"<p>Endpoint base: <code>{html.escape(PUBLIC_BASE)}</code> &middot; "
            "<a href='/admin'>refresh</a></p>"
        )
        parts.append("<h2>Pending device approvals</h2>")
        if pending:
            rows = []
            for p in pending:
                user_opts = "".join(
                    f"<option value='{html.escape(u['id'])}'>{html.escape(u['username'])}</option>"
                    for u in users
                    if u["enabled"]
                )
                rows.append(
                    "<tr><td><code>"
                    + html.escape(p["user_code"])
                    + "</code></td><td>"
                    + html.escape(p["platform"] or "?")
                    + "</td><td>"
                    + html.escape(iso(p["created_at"]))
                    + "</td><td>"
                    + html.escape(iso(p["expires_at"]))
                    + "</td><td>"
                    + f"<form class='inline' method='post' action='/admin/devices/{html.escape(p['device_code'])}/approve'>"
                    + f"<input type='hidden' name='csrf' value='{nonce}'>"
                    + "<select name='user_id' required>"
                    + (user_opts or "<option value=''>no enabled users</option>")
                    + "</select> "
                    + "or new: <input type='text' name='new_username' size='10'> "
                    + "<button type='submit'>Approve</button></form> "
                    + f"<form class='inline' method='post' action='/admin/devices/{html.escape(p['device_code'])}/deny'>"
                    + f"<input type='hidden' name='csrf' value='{nonce}'>"
                    + "<button type='submit'>Deny</button></form></td></tr>"
                )
            parts.append(
                "<table><tr><th>User code</th><th>Platform</th><th>Requested</th>"
                "<th>Expires</th><th>Actions</th></tr>" + "".join(rows) + "</table>"
            )
        else:
            parts.append("<p>No pending device requests.</p>")
        parts.append("<h2>Users</h2>")
        if users:
            rows = []
            for u in users:
                state = "enabled" if u["enabled"] else "DISABLED"
                toggle_action = "disable" if u["enabled"] else "enable"
                rows.append(
                    "<tr><td><code>"
                    + html.escape(u["id"])
                    + "</code></td><td>"
                    + html.escape(u["username"])
                    + "</td><td>"
                    + html.escape(u["note"])
                    + "</td><td>"
                    + state
                    + "</td><td>"
                    + html.escape(iso(u["created_at"]))
                    + "</td><td>"
                    + (html.escape(iso(u["last_login_at"])) if u["last_login_at"] else "never")
                    + "</td><td>"
                    + f"<form class='inline' method='post' action='/admin/users/{html.escape(u['id'])}/{toggle_action}'>"
                    + f"<input type='hidden' name='csrf' value='{nonce}'>"
                    + f"<button type='submit'>{toggle_action}</button></form> "
                    + f"<form class='inline' method='post' action='/admin/users/{html.escape(u['id'])}/delete' "
                    + "onsubmit=\"return confirm('Delete this user?')\">"
                    + f"<input type='hidden' name='csrf' value='{nonce}'>"
                    + "<button type='submit'>delete</button></form></td></tr>"
                )
            parts.append(
                "<table><tr><th>ID</th><th>Username</th><th>Note</th><th>State</th>"
                "<th>Created</th><th>Last login</th><th>Actions</th></tr>"
                + "".join(rows)
                + "</table>"
            )
        else:
            parts.append("<p>No users yet — add one below.</p>")
        parts.append(
            "<h2>Add user</h2>"
            "<form method='post' action='/admin/users'>"
            f"<input type='hidden' name='csrf' value='{nonce}'>"
            "Username: <input type='text' name='username' required maxlength='64'> "
            "Note: <input type='text' name='note' size='30'> "
            "<button type='submit'>Create</button></form>"
        )
        parts.append("<h2>Recent device activity</h2><table>"
                     "<tr><th>User code</th><th>Status</th><th>Created</th></tr>")
        for r in recent:
            parts.append(
                "<tr><td><code>"
                + html.escape(r["user_code"])
                + "</code></td><td>"
                + html.escape(r["status"])
                + "</td><td>"
                + html.escape(iso(r["created_at"]))
                + "</td></tr>"
            )
        parts.append("</table>")
        return 200, self.page("floyd-auth admin", "".join(parts)), "html", {}

    def page_approve(self, user_code: str, conn):
        row = None
        if user_code:
            row = conn.execute(
                "SELECT * FROM device_codes WHERE user_code = ?", (user_code.strip().upper(),)
            ).fetchone()
        if row is None:
            return 404, self.page("unknown code", "<h1>Unknown user code</h1>"
                                  f"<p>No device request matches <code>{html.escape(user_code)}</code>.</p>"
                                  "<p><a href='/admin'>Back to admin</a></p>"), "html", {}
        nonce = csrf_token()
        users = conn.execute("SELECT * FROM users WHERE enabled=1 ORDER BY username").fetchall()
        opts = "".join(
            f"<option value='{html.escape(u['id'])}'>{html.escape(u['username'])}</option>"
            for u in users
        )
        if row["status"] == "pending":
            body = (
                f"<h1>Approve device sign-in</h1>"
                f"<p>User code: <code>{html.escape(row['user_code'])}</code> &middot; "
                f"platform: <code>{html.escape(row['platform'] or '?')}</code> &middot; "
                f"expires: {html.escape(iso(row['expires_at']))}</p>"
                f"<form method='post' action='/admin/devices/{html.escape(row['device_code'])}/approve'>"
                f"<input type='hidden' name='csrf' value='{nonce}'>"
                f"Approve as: <select name='user_id' required><option value=''>— pick user —</option>{opts}</select>"
                f" or create new user: <input type='text' name='new_username' maxlength='64'> "
                f"<button type='submit'>Approve</button></form>"
                f"<form method='post' action='/admin/devices/{html.escape(row['device_code'])}/deny'>"
                f"<input type='hidden' name='csrf' value='{nonce}'>"
                f"<button type='submit'>Deny this request</button></form>"
            )
        else:
            body = (
                "<h1>Device request</h1>"
                f"<p>User code <code>{html.escape(row['user_code'])}</code> is "
                f"<strong>{html.escape(row['status'])}</strong>.</p>"
                "<p><a href='/admin'>Back to admin</a></p>"
            )
        return 200, self.page("approve device", body), "html", {}


from company_platform import CompanyPlatform

PLATFORM = CompanyPlatform(sys.modules[__name__])


def main():
    os.umask(0o077)
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        stream=sys.stdout,
    )
    init_db()
    PLATFORM.initialize()
    threading.Thread(target=PLATFORM.scheduled_refresh, daemon=True).start()
    threading.Thread(target=gc_sweep, daemon=True).start()
    server = ThreadingHTTPServer((BIND_HOST, BIND_PORT), Handler)
    server.daemon_threads = True
    log.info("floyd-auth listening on %s:%s (public base %s)", BIND_HOST, BIND_PORT, PUBLIC_BASE)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
