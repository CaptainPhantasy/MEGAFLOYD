"""Persistent company account, administration and provider services."""

import base64
import hashlib
import hmac
import http.cookies
import json
import math
import os
import re
import ipaddress
import shlex
import secrets
import threading
import time
from contextlib import contextmanager
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise urllib.error.HTTPError(req.full_url, code, 'Unexpected redirect', headers, fp)


def read_json_url(url, *, headers=None, timeout=15, data=None):
    request = urllib.request.Request(url, data=data, headers={
        'Accept': 'application/json', 'User-Agent': 'Floyd-Code-Platform/1.0', **(headers or {})})
    with urllib.request.build_opener(NoRedirect).open(request, timeout=timeout) as response:
        raw = response.read(24 * 1024 * 1024 + 1)
        if len(raw) > 24 * 1024 * 1024:
            raise ValueError('Provider information is too large.')
        return json.loads(raw)


class CompanyPlatform:
    def __init__(self, app):
        self.app = app
        self.lock = threading.RLock()
        self.refresh_lock = threading.Lock()
        self.failures = {}
        from media_tools import MediaTools
        self.media = MediaTools(self)
        from model_access import ModelAccess
        self.model_access = ModelAccess(self)
        from feedback_store import FeedbackStore
        self.feedback = FeedbackStore(self)
        from shared_tools import SharedTools
        self.shared_tools = SharedTools(self)

    def initialize(self):
        with self.database() as conn:
            self.feedback.initialize(conn)
            self.shared_tools.initialize(conn)
            conn.executescript('''
                CREATE TABLE IF NOT EXISTS web_sessions (
                    token_hash TEXT PRIMARY KEY, kind TEXT NOT NULL,
                    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
                    csrf TEXT NOT NULL, version TEXT NOT NULL, expires_at INTEGER NOT NULL
                );
                CREATE TABLE IF NOT EXISTS audit (
                    id INTEGER PRIMARY KEY AUTOINCREMENT, created_at INTEGER NOT NULL,
                    actor TEXT NOT NULL, action TEXT NOT NULL, target TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS provider_settings (
                    id TEXT PRIMARY KEY, name TEXT NOT NULL, base_url TEXT NOT NULL,
                    protocol TEXT NOT NULL, secret TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
                    models_json TEXT NOT NULL DEFAULT '{}', updated_at INTEGER NOT NULL
                );
                CREATE TABLE IF NOT EXISTS invitations (
                    token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                    created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL, used_at INTEGER
                );
                CREATE TABLE IF NOT EXISTS usage_events (
                    id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL,
                    model TEXT NOT NULL, created_at INTEGER NOT NULL, input_count INTEGER,
                    output_count INTEGER, cost_usd REAL, outcome TEXT NOT NULL
                );
            ''')
            columns = {r['name'] for r in conn.execute('PRAGMA table_info(users)')}
            for name, definition in [('password_salt', "TEXT NOT NULL DEFAULT ''"),
                                     ('password_hash', "TEXT NOT NULL DEFAULT ''"),
                                     ('monthly_request_limit', 'INTEGER')]:
                if name not in columns:
                    conn.execute('ALTER TABLE users ADD COLUMN ' + name + ' ' + definition)
            for table in ['access_tokens', 'refresh_tokens']:
                if 'device_code' not in {r['name'] for r in conn.execute('PRAGMA table_info(' + table + ')')}:
                    conn.execute('ALTER TABLE ' + table + ' ADD COLUMN device_code TEXT')
            conn.execute('DELETE FROM web_sessions WHERE expires_at < ?', (self.app.now(),))
            conn.execute("UPDATE usage_events SET outcome='interrupted' WHERE outcome='in_progress'")
            company_base = self.config().get('company_model_base', 'http://100.78.109.105:8090/v1')
            if company_base:
                conn.execute('INSERT OR IGNORE INTO provider_settings(id,name,base_url,protocol,updated_at) VALUES(?,?,?,?,?)',
                             ('company-local', 'Company models on inference', company_base, 'openai', self.app.now()))

    def fail(self, status, message, code='request_failed'):
        raise self.app.ApiError(status, {'error': code, 'error_description': message})

    @contextmanager
    def database(self):
        conn = self.app.db()
        try:
            with conn:
                yield conn
        finally:
            conn.close()

    def config(self):
        return self.app.load_config()

    def version(self):
        return self.config()['password_hash'][:24]

    def audit(self, conn, action, target='', actor='owner'):
        conn.execute('INSERT INTO audit(created_at,actor,action,target) VALUES(?,?,?,?)',
                     (self.app.now(), actor, action, target))

    def value(self, key):
        with self.database() as conn:
            row = conn.execute('SELECT value FROM kv WHERE key=?', (key,)).fetchone()
            return json.loads(row['value']) if row else None

    def save(self, key, value):
        with self.database() as conn:
            conn.execute('INSERT OR REPLACE INTO kv(key,value) VALUES(?,?)', (key, json.dumps(value)))

    def cookie(self, name, token='', lifetime=43200):
        secure = '; Secure' if self.app.PUBLIC_BASE.startswith('https:') else ''
        return f'{name}={token}; Path=/; HttpOnly; SameSite=Strict; Max-Age={lifetime}{secure}'

    def session(self, handler, kind):
        cookies = http.cookies.SimpleCookie()
        try:
            cookies.load(handler.headers.get('Cookie', ''))
        except http.cookies.CookieError:
            return None
        name = 'floyd_admin' if kind == 'admin' else 'floyd_member'
        if name not in cookies:
            return None
        with self.database() as conn:
            row = conn.execute('SELECT * FROM web_sessions WHERE token_hash=? AND kind=? AND expires_at>?',
                               (self.app.sha256_hex(cookies[name].value), kind, self.app.now())).fetchone()
            if row is None:
                return None
            if kind == 'admin' and row['version'] != self.version():
                return None
            if kind == 'member':
                user = conn.execute('SELECT * FROM users WHERE id=? AND enabled=1', (row['user_id'],)).fetchone()
                if user is None:
                    return None
            return dict(row)

    def new_session(self, kind, user_id=None):
        token = secrets.token_urlsafe(40)
        csrf = secrets.token_urlsafe(32)
        with self.database() as conn:
            conn.execute('INSERT INTO web_sessions VALUES(?,?,?,?,?,?)',
                         (self.app.sha256_hex(token), kind, user_id, csrf,
                          self.version() if kind == 'admin' else '', self.app.now() + 43200))
        name = 'floyd_admin' if kind == 'admin' else 'floyd_member'
        return csrf, {'Set-Cookie': self.cookie(name, token)}

    def is_admin(self, handler):
        if self.session(handler, 'admin') is not None:
            return True
        cookies = http.cookies.SimpleCookie()
        try:
            cookies.load(handler.headers.get('Cookie', ''))
        except http.cookies.CookieError:
            return False
        if 'floyd_admin' in cookies:
            return False
        return self.app.admin_check(handler.headers)

    def admin(self, handler, write=False):
        if not self.is_admin(handler):
            self.fail(401, 'Enter your administrator key.', 'unauthorized')
        session = self.session(handler, 'admin')
        csrf = session['csrf'] if session else self.app.csrf_token()
        if write:
            self.check_origin(handler)
            supplied = handler.headers.get('X-CSRF-Token', '')
            if not supplied and 'application/x-www-form-urlencoded' in handler.headers.get('Content-Type', ''):
                supplied = (handler.read_form().get('csrf') or [''])[0]
            valid = hmac.compare_digest(supplied, csrf) if session else self.app.csrf_valid(supplied)
            if not valid:
                self.fail(403, 'Reload the page and try again.', 'csrf')
        return csrf

    def check_origin(self, handler):
        origin = handler.headers.get('Origin')
        if origin and origin.rstrip('/') != self.app.PUBLIC_BASE:
            self.fail(403, 'This request came from another site.', 'wrong_origin')

    def limited_login(self, handler, success=None):
        key = handler.client_address[0]
        if key in ['127.0.0.1', '::1']:
            forwarded = handler.headers.get('X-Forwarded-For', '').split(',')[-1].strip()
            try:
                key = str(ipaddress.ip_address(forwarded))
            except ValueError:
                pass
        now = self.app.now()
        with self.lock:
            recent = [t for t in self.failures.get(key, []) if t > now - 600]
            if success is True:
                self.failures.pop(key, None)
            elif success is False:
                self.failures[key] = recent + [now]
            elif len(recent) >= 10:
                self.fail(429, 'Too many unsuccessful sign-ins. Try again in ten minutes.', 'rate_limited')

    def rotate(self, handler):
        self.admin(handler, write=True)
        with self.lock:
            self.admin(handler, write=True)
            current = self.config()
            body = handler.read_json()
            key = body.get('new_key') or ('fa_' + secrets.token_urlsafe(40))
            if not isinstance(key, str) or not 32 <= len(key) <= 256:
                self.fail(400, 'The replacement key must be between 32 and 256 characters.')
            salt = secrets.token_bytes(24)
            current['password_salt'] = salt.hex()
            current['password_hash'] = hashlib.pbkdf2_hmac('sha256', key.encode(), salt, self.app.PBKDF2_ITERATIONS).hex()
            current['csrf_secret'] = secrets.token_hex(32)
            path = self.app.CONFIG_PATH
            temporary = path.with_name('config.rotation-' + secrets.token_hex(6))
            fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, 'w', encoding='utf-8') as stream:
                json.dump(current, stream, indent=2)
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(temporary, path)
            directory = os.open(path.parent, os.O_RDONLY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)
            self.app.CONFIG.update(current)
            with self.database() as conn:
                conn.execute("DELETE FROM web_sessions WHERE kind='admin'")
                self.audit(conn, 'administrator key replaced')
            raw_file = self.app.BASE_DIR / 'ADMIN-PASSWORD.txt'
            if raw_file.exists():
                quarantine = self.app.DATA_DIR / 'quarantine'
                quarantine.mkdir(mode=0o700, exist_ok=True)
                os.chmod(quarantine, 0o700)
                os.chmod(raw_file, 0o600)
                os.replace(raw_file, quarantine / ('retired-admin-key-' + str(self.app.now()) + '.txt'))
            csrf, headers = self.new_session('admin')
        return 200, {'new_key': key, 'csrf': csrf, 'message': 'Save this key now. The old key no longer works.'}, 'json', headers

    def catalog(self):
        saved = self.value('company_catalog')
        if not saved:
            return {'providers': {}, 'updated_at': None, 'source': self.config().get('catalog_url', 'https://models.dev/api.json'),
                    'last_error': None, 'stale': True}
        result = dict(saved)
        result['last_error'] = self.value('catalog_error')
        result['stale'] = self.app.now() - result.get('updated_at_epoch', 0) > 86400
        return result

    def refresh_catalog(self):
        if not self.refresh_lock.acquire(blocking=False):
            self.fail(409, 'The directory is already being refreshed.')
        try:
            url = self.config().get('catalog_url', 'https://models.dev/api.json')
            if not url:
                self.fail(503, 'No provider directory source is configured.')
            payload = read_json_url(url)
            if not isinstance(payload, dict) or not payload:
                raise ValueError('The provider source returned an empty directory.')
            clean = {}
            model_count = 0
            for id_, provider in payload.items():
                if not isinstance(provider, dict) or not isinstance(provider.get('models'), dict):
                    continue
                if not isinstance(provider.get('name'), str):
                    continue
                if not all(isinstance(model, dict) for model in provider['models'].values()):
                    raise ValueError('The provider source returned invalid model records.')
                json.dumps(provider, allow_nan=False)
                clean[id_] = provider
                model_count += len(provider['models'])
            if not clean or model_count == 0:
                raise ValueError('The provider source contained no models.')
            result = {'providers': clean, 'source': url, 'updated_at': self.app.iso(),
                      'updated_at_epoch': self.app.now(), 'provider_count': len(clean), 'model_count': model_count}
            self.save('company_catalog', result)
            self.save('catalog_error', None)
            with self.database() as conn:
                self.audit(conn, 'provider directory refreshed', str(len(clean)) + ' providers')
            return result
        except self.app.ApiError:
            raise
        except Exception as error:
            self.save('catalog_error', {'time': self.app.iso(), 'message': str(error)[:300]})
            self.fail(502, 'Refresh failed. The previous directory has been kept. See the saved error for details.')
        finally:
            self.refresh_lock.release()

    def overview(self, handler):
        csrf = self.admin(handler)
        cat = self.catalog()
        with self.database() as conn:
            counts = dict(conn.execute('SELECT COUNT(*) AS total, COALESCE(SUM(enabled),0) AS enabled FROM users').fetchone())
            pending = conn.execute("SELECT COUNT(*) AS n FROM device_codes WHERE status='pending' AND expires_at>?", (self.app.now(),)).fetchone()['n']
            audit = [dict(r) for r in conn.execute('SELECT * FROM audit ORDER BY id DESC LIMIT 50')]
        return {'csrf': csrf, 'accounts': counts, 'pending_devices': pending, 'audit': audit,
                'catalog': {k: v for k, v in cat.items() if k != 'providers'},
                'addresses': {'account': self.app.PUBLIC_BASE, 'models': self.app.PUBLIC_BASE + '/v1',
                              'directory': self.app.PUBLIC_BASE + '/catalog/api.json'},
                'admin_key_version': self.version(), 'time': self.app.iso()}

    def invite(self, handler):
        body = handler.read_json()
        with self.database() as conn:
            if body.get('user_id'):
                user = conn.execute('SELECT * FROM users WHERE id=?', (body['user_id'],)).fetchone()
                if user is None:
                    self.fail(404, 'That account does not exist.')
            else:
                user = self.app.create_user(conn, str(body.get('username', '')), str(body.get('note', '')))
            token = secrets.token_urlsafe(40)
            conn.execute('UPDATE invitations SET used_at=? WHERE user_id=? AND used_at IS NULL',
                         (self.app.now(), user['id']))
            conn.execute('INSERT INTO invitations VALUES(?,?,?,?,NULL)',
                         (self.app.sha256_hex(token), user['id'], self.app.now(), self.app.now() + 172800))
            self.audit(conn, 'invitation created', user['id'])
        return 201, {'invitation': token, 'url': self.app.PUBLIC_BASE + '/join#' + token,
                     'expires_at': self.app.iso(self.app.now() + 172800), 'user': self.app.user_row_public(user)}, 'json', {}

    def password(self, password):
        if not isinstance(password, str) or not 12 <= len(password) <= 1024:
            self.fail(400, 'Use a password of at least twelve characters.')
        salt = secrets.token_bytes(24)
        digest = hashlib.pbkdf2_hmac('sha256', password.encode(), salt, self.app.PBKDF2_ITERATIONS)
        return salt.hex(), digest.hex()

    def join(self, handler):
        self.check_origin(handler)
        body = handler.read_json()
        salt, digest = self.password(body.get('password'))
        with self.database() as conn:
            conn.execute('BEGIN IMMEDIATE')
            row = conn.execute('SELECT i.*,u.enabled FROM invitations i JOIN users u ON u.id=i.user_id '
                               'WHERE token_hash=? AND used_at IS NULL AND expires_at>?',
                               (self.app.sha256_hex(str(body.get('invitation', ''))), self.app.now())).fetchone()
            if row is None or not row['enabled']:
                self.fail(400, 'This invitation is expired or already used. Ask the owner for another.')
            conn.execute('UPDATE invitations SET used_at=? WHERE token_hash=?', (self.app.now(), row['token_hash']))
            conn.execute('UPDATE users SET password_salt=?,password_hash=? WHERE id=?', (salt, digest, row['user_id']))
            self.cancel_access(conn, row['user_id'])
            user = conn.execute('SELECT * FROM users WHERE id=?', (row['user_id'],)).fetchone()
            self.audit(conn, 'account joined', user['id'], user['id'])
        csrf, headers = self.new_session('member', user['id'])
        return 201, {'user': self.app.user_row_public(user), 'csrf': csrf}, 'json', headers

    def member_login(self, handler):
        self.check_origin(handler)
        self.limited_login(handler)
        body = handler.read_json()
        username = body.get('username', '')
        password = body.get('password', '')
        if not isinstance(username, str) or not isinstance(password, str) or len(username) > 64 or len(password) > 1024:
            self.fail(400, 'Check your account name and password.')
        with self.database() as conn:
            user = conn.execute('SELECT * FROM users WHERE username=? AND enabled=1', (username,)).fetchone()
        salt = bytes.fromhex(user['password_salt']) if user and user['password_salt'] else bytes(24)
        actual = hashlib.pbkdf2_hmac('sha256', password.encode(), salt, self.app.PBKDF2_ITERATIONS).hex()
        if user is None or not user['password_hash'] or not hmac.compare_digest(actual, user['password_hash']):
            self.limited_login(handler, False)
            self.fail(401, 'That account name or password was not accepted.', 'unauthorized')
        self.limited_login(handler, True)
        csrf, headers = self.new_session('member', user['id'])
        return 200, {'user': self.app.user_row_public(user), 'csrf': csrf}, 'json', headers

    def member(self, handler, write=False):
        session = self.session(handler, 'member')
        if session is None:
            self.fail(401, 'Sign into your account first.', 'unauthorized')
        if write:
            self.check_origin(handler)
            if not hmac.compare_digest(handler.headers.get('X-CSRF-Token', ''), session['csrf']):
                self.fail(403, 'Reload the page and try again.', 'csrf')
        return session

    def cancel_access(self, conn, user_id, device_code=None):
        for table in ['access_tokens', 'refresh_tokens']:
            if device_code:
                conn.execute('UPDATE ' + table + ' SET revoked=1 WHERE user_id=? AND (device_code=? OR device_code IS NULL)', (user_id, device_code))
            else:
                conn.execute('UPDATE ' + table + ' SET revoked=1 WHERE user_id=?', (user_id,))
        if device_code:
            conn.execute("UPDATE device_codes SET status='denied' WHERE user_id=? AND device_code=?", (user_id, device_code))
        else:
            conn.execute('DELETE FROM web_sessions WHERE user_id=?', (user_id,))
            conn.execute("UPDATE device_codes SET status='denied' WHERE user_id=? AND status IN ('pending','approved')", (user_id,))

    def member_route(self, handler, method, path):
        if path == '/account/api/login' and method == 'POST':
            return self.member_login(handler)
        if path == '/account/api/join' and method == 'POST':
            return self.join(handler)
        session = self.member(handler, write=method not in ['GET', 'HEAD'])
        user_id = session['user_id']
        with self.database() as conn:
            if path == '/account/api/me' and method == 'GET':
                user = conn.execute('SELECT * FROM users WHERE id=?', (user_id,)).fetchone()
                return 200, {'user': self.app.user_row_public(user), 'csrf': session['csrf'],
                             'model_address': self.app.PUBLIC_BASE + '/v1'}, 'json', {}
            if path == '/account/api/logout' and method == 'POST':
                conn.execute('DELETE FROM web_sessions WHERE token_hash=?', (session['token_hash'],))
                return 200, {}, 'json', {'Set-Cookie': self.cookie('floyd_member', lifetime=0)}
            if path == '/account/api/devices' and method == 'GET':
                rows = conn.execute('SELECT * FROM device_codes WHERE user_id=? ORDER BY created_at DESC LIMIT 100', (user_id,))
                return 200, {'devices': [self.app.device_row_public(r) for r in rows]}, 'json', {}
            if path == '/account/api/approve' and method == 'POST':
                code = str(handler.read_json().get('user_code', '')).upper().strip()
                row = conn.execute('SELECT * FROM device_codes WHERE user_code=?', (code,)).fetchone()
                if row is None or row['status'] != 'pending' or row['expires_at'] <= self.app.now():
                    self.fail(400, 'This device code is expired or already used.')
                if row['user_id'] and row['user_id'] != user_id:
                    self.fail(403, 'This device belongs to another account.')
                result = handler.device_approve(conn, row['device_code'], user_id, {})
                self.audit(conn, 'device approved', row['user_code'], user_id)
                return result
            if path == '/account/api/revoke' and method == 'POST':
                code = str(handler.read_json().get('device_code', ''))
                row = conn.execute('SELECT * FROM device_codes WHERE device_code=? AND user_id=?', (code, user_id)).fetchone()
                if row is None:
                    self.fail(404, 'That device does not belong to your account.')
                self.cancel_access(conn, user_id, code)
                self.audit(conn, 'device access cancelled', row['user_code'], user_id)
                return 200, {}, 'json', {}
            if path == '/account/api/password' and method == 'POST':
                body = handler.read_json()
                user = conn.execute('SELECT * FROM users WHERE id=?', (user_id,)).fetchone()
                old = str(body.get('current_password', ''))
                actual = hashlib.pbkdf2_hmac('sha256', old.encode(), bytes.fromhex(user['password_salt']), self.app.PBKDF2_ITERATIONS).hex()
                if not hmac.compare_digest(actual, user['password_hash']):
                    self.fail(403, 'Your current password was not accepted.')
                salt, digest = self.password(body.get('password'))
                conn.execute('UPDATE users SET password_salt=?,password_hash=? WHERE id=?', (salt, digest, user_id))
                self.cancel_access(conn, user_id)
                self.audit(conn, 'password replaced', user_id, user_id)
                return 200, {'message': 'Password replaced. Sign in again.'}, 'json', {'Set-Cookie': self.cookie('floyd_member', lifetime=0)}
        self.fail(404, 'That account action does not exist.')

    def markup(self):
        return (Path(__file__).parent / 'platform.html').read_text(encoding='utf-8')

    def connections(self):
        with self.database() as conn:
            rows = conn.execute('SELECT * FROM provider_settings ORDER BY name').fetchall()
        return [{'id': row['id'], 'name': row['name'], 'base_url': row['base_url'],
                 'protocol': row['protocol'], 'enabled': bool(row['enabled']),
                 'key_saved': bool(row['secret']), 'models': json.loads(row['models_json']),
                 'updated_at': row['updated_at']} for row in rows]

    def save_connection(self, handler, id_=None):
        body = handler.read_json()
        id_ = id_ or body.get('id')
        if not isinstance(id_, str) or not re.fullmatch(r'[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}', id_):
            self.fail(400, 'Use a short provider name with letters, numbers, dashes or underscores.')
        with self.database() as conn:
            row = conn.execute('SELECT * FROM provider_settings WHERE id=?', (id_,)).fetchone()
            if row is None and handler.command == 'PUT':
                self.fail(404, 'That connection does not exist.')
            if row is not None and handler.command == 'POST':
                self.fail(409, 'That provider name is already in use.')
            old = dict(row) if row else {}
            name = str(body.get('name', old.get('name', ''))).strip()
            address = str(body.get('base_url', old.get('base_url', ''))).rstrip('/')
            parsed = urllib.parse.urlsplit(address)
            if parsed.scheme not in ['https', 'http'] or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
                self.fail(400, 'Use the provider’s full address without a password or extra link fields.')
            if parsed.scheme == 'http':
                try:
                    private = ipaddress.ip_address(parsed.hostname).is_private or ipaddress.ip_address(parsed.hostname) in ipaddress.ip_network('100.64.0.0/10')
                except ValueError:
                    private = parsed.hostname == 'localhost'
                if not private:
                    self.fail(400, 'Outside providers need a secure address beginning with https.')
            protocol = body.get('protocol', old.get('protocol', 'openai'))
            if protocol not in ['openai', 'openai_responses', 'anthropic'] or not 1 <= len(name) <= 120:
                self.fail(400, 'Check the display name and request format.')
            secret = body.get('secret', old.get('secret', ''))
            if not isinstance(secret, str) or len(secret) > 4096 or '\n' in secret or '\r' in secret:
                self.fail(400, 'That provider key is invalid.')
            enabled = body.get('enabled', bool(old.get('enabled', 1)))
            if not isinstance(enabled, bool):
                self.fail(400, 'Choose whether this connection is enabled.')
            conn.execute('INSERT OR REPLACE INTO provider_settings VALUES(?,?,?,?,?,?,?,?)',
                         (id_, name, address, protocol, secret, int(enabled), old.get('models_json', '{}'), self.app.now()))
            self.audit(conn, 'company connection saved', id_)
        return 200 if row else 201, {'id': id_, 'message': 'Connection saved.'}, 'json', {}

    def refresh_models(self, id_):
        with self.database() as conn:
            row = conn.execute('SELECT * FROM provider_settings WHERE id=?', (id_,)).fetchone()
        if row is None:
            self.fail(404, 'That connection does not exist.')
        headers = {'Authorization': 'Bearer ' + row['secret']} if row['secret'] else {}
        if row['protocol'] == 'anthropic' and row['secret']:
            headers = {'x-api-key': row['secret'], 'anthropic-version': '2023-06-01'}
        try:
            payload = read_json_url(row['base_url'] + '/models', headers=headers)
            if not isinstance(payload, dict) or not isinstance(payload.get('data'), list) or not payload['data']:
                raise ValueError('Provider returned no model list.')
            previous = json.loads(row['models_json'])
            upstream = self.catalog()['providers'].get(id_, {}).get('models', {})
            settings = {}
            settings_path = Path(self.config().get('company_model_settings', '/srv/organized/services/llama-swap/config/config.yaml'))
            if id_ == 'company-local' and settings_path.exists():
                import yaml
                settings = yaml.safe_load(settings_path.read_text()).get('models', {})
            models = {}
            for item in payload['data']:
                if not isinstance(item, dict) or not isinstance(item.get('id'), str):
                    raise ValueError('Provider returned invalid model records.')
                model_id = item['id']
                model = dict(previous.get(model_id, {}))
                model.update(upstream.get(model_id, {}))
                model.update({k: v for k, v in item.items() if k not in ['object', 'owned_by', 'created', 'meta', 'status']})
                model['id'] = model_id
                if settings.get(model_id):
                    entry = settings[model_id]
                    command = shlex.split(entry.get('cmd', ''))
                    context = int(command[command.index('--ctx-size') + 1]) if '--ctx-size' in command else None
                    model.update(name=entry.get('name', model_id), description=entry.get('description', ''),
                                 limit={'context': context}, tool_call=entry.get('capabilities', {}).get('tools', '--jinja' in command),
                                 kind='embedding' if '--embedding' in command else 'coding',
                                 cost={'input': 0, 'output': 0}, price_basis='No outside provider charge',
                                 source=str(settings_path), last_updated=self.app.iso())
                    metadata = entry.get('metadata', {})
                    model['source'] = metadata.get('source') or str(settings_path)
                    model['native_context'] = metadata.get('native_context')
                    model['library_id'] = metadata.get('library_id')
                if 'name' not in model:
                    model['name'] = model_id
                if 'limit' not in model:
                    context = item.get('context_length')
                    model['limit'] = {'context': context} if isinstance(context, int) and context > 0 else {}
                models[model_id] = model
            library_path = self.app.DATA_DIR / 'local-library.json'
            if id_ == 'company-local' and library_path.exists():
                library = json.loads(library_path.read_text())
                for item in library['models']:
                    if item['kind'] not in ['speech', 'music']:
                        continue
                    models[item['id']] = {'id': item['id'], 'name': item['display_name'],
                        'kind': item['kind'], 'status': item['status'], 'ready': item.get('ready', False),
                        'company_tool': item.get('company_tool'), 'limit': {}, 'cost': {'input': 0, 'output': 0},
                        'source': item['hub_url'] or library['source'], 'last_updated': library['checked_at'],
                        'modalities': {'input': ['audio'] if item['kind'] == 'speech' else ['text'],
                                       'output': ['text'] if item['kind'] == 'speech' else ['audio']}}
            with self.database() as conn:
                conn.execute('UPDATE provider_settings SET models_json=?,updated_at=? WHERE id=?',
                             (json.dumps(models, allow_nan=False), self.app.now(), id_))
                self.audit(conn, 'company models refreshed', id_)
            return {'models': len(models), 'updated_at': self.app.iso()}
        except Exception:
            self.fail(502, 'This provider’s model list could not be refreshed. Its last saved list was kept.')

    def registry(self):
        cat = dict(self.catalog()['providers'])
        for provider in self.connections():
            if not provider['enabled']:
                continue
            models = {}
            for id_, model in provider['models'].items():
                value = dict(model)
                value['id'] = provider['id'] + '/' + id_
                models[value['id']] = value
            cat['company-' + provider['id']] = {'id': 'company-' + provider['id'],
                'name': provider['name'], 'api': self.app.PUBLIC_BASE + '/v1', 'type': provider['protocol'],
                'requires_account': True, 'doc': self.app.PUBLIC_BASE + '/account', 'models': models}
        return cat

    def managed_models(self):
        result = []
        for provider in self.connections():
            if not provider['enabled']:
                continue
            for id_, model in provider['models'].items():
                context = model.get('limit', {}).get('context')
                if model.get('kind') == 'embedding' or not isinstance(context, int) or context <= 0:
                    continue
                inputs = model.get('modalities', {}).get('input', [])
                record = {'id': provider['id'] + '/' + id_, 'display_name': model.get('name', id_),
                    'context_length': context, 'supports_reasoning': bool(model.get('reasoning')),
                    'supports_image_in': 'image' in inputs, 'supports_video_in': 'video' in inputs,
                    'supports_tool_use': bool(model.get('tool_call')), 'supports_dynamic_tools': False,
                    'provider_name': provider['name'], 'cost': model.get('cost'),
                    'last_updated': model.get('last_updated'), 'base_url': self.app.PUBLIC_BASE + '/v1'}
                if provider['protocol'] == 'anthropic':
                    record['protocol'] = 'anthropic'
                result.append(record)
        return result

    def route(self, handler, method, path, query):
        media = self.media.route(handler, method, path)
        if media is not None:
            return media
        shared = self.shared_tools.route(handler, method, path, query)
        if shared is not None:
            return shared
        if (method == 'POST' and path in ['/feedback', '/feedback/upload_url', '/feedback/upload_complete']) or path.startswith('/feedback/upload/'):
            return self.feedback.route(handler, method, path, query)
        if path == '/account/api/usage' and method == 'GET':
            session = self.member(handler)
            with self.database() as conn:
                user = conn.execute('SELECT * FROM users WHERE id=?', (session['user_id'],)).fetchone()
                return 200, self.model_access.usage(conn, user), 'json', {}
        if path == '/admin/api/feedback' and method == 'GET':
            self.admin(handler)
            with self.database() as conn:
                rows = conn.execute('SELECT f.id,f.user_id,u.username,f.created_at,f.body FROM feedback f '
                                    'LEFT JOIN users u ON u.id=f.user_id ORDER BY f.id DESC LIMIT 100')
                return 200, {'feedback': [{**dict(row), 'body': json.loads(row['body'])} for row in rows]}, 'json', {}
        if method == 'POST' and path in ['/chat/completions', '/v1/chat/completions', '/responses', '/v1/responses', '/messages', '/v1/messages', '/embeddings', '/v1/embeddings']:
            return self.model_access.relay(handler, path)
        if method == 'GET' and path in ['/v1/models', '/usages']:
            with self.database() as conn:
                user = self.app.require_bearer_user(conn, handler.headers)
                if path == '/usages':
                    payload = self.model_access.usage(conn, user)
                else:
                    payload = {'object': 'list', 'data': self.managed_models()}
            return 200, payload, 'json', {}
        if path == '/approve' and method == 'GET':
            return 200, self.markup(), 'html', {}
        if path.startswith('/account/api/'):
            return self.member_route(handler, method, path)
        if path in ['/admin/login', '/account', '/join', '/', '/catalog'] and method == 'GET':
            return 200, self.markup(), 'html', {}
        if path in ['/platform.js', '/platform.css'] and method == 'GET':
            file = Path(__file__).parent / path[1:]
            body = file.read_bytes()
            handler.send_response(200)
            handler.send_header('Content-Type', 'application/javascript; charset=utf-8' if path.endswith('.js') else 'text/css; charset=utf-8')
            handler.send_header('Content-Length', str(len(body)))
            handler.send_header('Cache-Control', 'no-cache')
            handler.security_headers()
            handler.end_headers()
            if handler.command != 'HEAD':
                handler.wfile.write(body)
            return 0, None, 'sent', {}
        if path == '/admin/api/login' and method == 'POST':
            self.check_origin(handler)
            self.limited_login(handler)
            body = handler.read_json()
            key = body.get('key', '')
            if not isinstance(key, str) or len(key) > 1024 or not self.app.verify_admin_password(key):
                self.limited_login(handler, False)
                self.fail(401, 'That administrator key was not accepted.', 'unauthorized')
            self.limited_login(handler, True)
            csrf, headers = self.new_session('admin')
            return 200, {'csrf': csrf}, 'json', headers
        if path == '/admin' and method == 'GET':
            if not self.is_admin(handler):
                return 303, {}, 'json', {'Location': '/admin/login'}
            return 200, self.markup(), 'html', {}
        if path.startswith('/admin'):
            self.admin(handler, write=method not in ['GET', 'HEAD'])
            if path == '/admin/api/invites' and method == 'POST':
                return self.invite(handler)
            if path == '/admin/api/connections':
                if method == 'GET':
                    return 200, {'providers': self.connections()}, 'json', {}
                if method == 'POST':
                    return self.save_connection(handler)
            if path.startswith('/admin/api/connections/'):
                id_ = path.split('/')[4]
                if method == 'PUT' and len(path.split('/')) == 5:
                    return self.save_connection(handler, id_)
                if method == 'POST' and path.endswith('/refresh'):
                    return 200, self.refresh_models(id_), 'json', {}
            if path == '/admin/api/logout' and method == 'POST':
                session = self.session(handler, 'admin')
                if session:
                    with self.database() as conn:
                        conn.execute('DELETE FROM web_sessions WHERE token_hash=?', (session['token_hash'],))
                return 200, {}, 'json', {'Set-Cookie': self.cookie('floyd_admin', 'signed-out')}
            if path == '/admin/api/key/rotate' and method == 'POST':
                return self.rotate(handler)
            if path == '/admin/api/overview' and method == 'GET':
                return 200, self.overview(handler), 'json', {}
            if path == '/admin/api/catalog/refresh' and method == 'POST':
                result = self.refresh_catalog()
                return 200, {k: v for k, v in result.items() if k != 'providers'}, 'json', {}
            if path == '/admin/api/catalog' and method == 'GET':
                return 200, self.catalog(), 'json', {}
        if path == '/catalog/api.json' and method == 'GET':
            cat = self.catalog()
            providers = self.registry()
            if not providers:
                self.fail(503, 'The provider directory has not been refreshed yet.')
            return 200, providers, 'json', {'X-Catalog-Updated': cat['updated_at'] or ''}
        if path == '/catalog/info' and method == 'GET':
            return 200, {k: v for k, v in self.catalog().items() if k != 'providers'}, 'json', {}
        return None

    def scheduled_refresh(self):
        while True:
            try:
                self.media.cleanup()
                if self.config().get('catalog_url', 'https://models.dev/api.json'):
                    self.refresh_catalog()
                for connection in self.connections():
                    if connection['enabled']:
                        try:
                            self.refresh_models(connection['id'])
                        except self.app.ApiError:
                            self.app.log.warning('Company model refresh failed for %s; kept saved list.', connection['id'])
            except Exception:
                self.app.log.warning('Provider directory refresh failed; kept saved copy.')
            time.sleep(3600)
