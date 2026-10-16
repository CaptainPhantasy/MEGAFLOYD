"""Company-approved tools through the existing fleet gateway."""
import datetime
import ipaddress
import json
import os
import re
import signal
import subprocess
import threading
import urllib.parse
from pathlib import Path


class SharedTools:
    def __init__(self, platform):
        self.platform = platform
        self.app = platform.app
        self.capacity = threading.BoundedSemaphore(4)

    def initialize(self, conn):
        conn.executescript('''
            CREATE TABLE IF NOT EXISTS shared_tools (
                id TEXT PRIMARY KEY, name TEXT NOT NULL, server TEXT NOT NULL,
                tool TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1,
                schema_json TEXT NOT NULL DEFAULT '{}'
            );
            CREATE TABLE IF NOT EXISTS tool_use (
                id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL,
                tool TEXT NOT NULL, created_at INTEGER NOT NULL, outcome TEXT NOT NULL
            );
        ''')
        for row in [('web-search', 'Search the web', 'zai-web-search', 'web_search_prime'),
                    ('web-reader', 'Read a web page', 'zai-web-reader', 'webReader')]:
            conn.execute('INSERT OR IGNORE INTO shared_tools(id,name,server,tool) VALUES(?,?,?,?)', row)
        self.platform.media.initialize(conn)

    def bridge(self, method, arguments):
        config = self.platform.config()
        command = config.get('company_gateway_command') or [
            '/home/linuxbrew/.linuxbrew/bin/node', str(Path(__file__).with_name('gateway_bridge.mjs'))]
        if not self.capacity.acquire(blocking=False):
            self.platform.fail(429, 'Company tools are busy. Try again shortly.', 'tools_busy')
        process = None
        try:
            process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                       stderr=subprocess.DEVNULL, text=True, start_new_session=True)
            raw, _ = process.communicate(json.dumps({'method': method, 'arguments': arguments}), timeout=50)
            if process.returncode != 0 or len(raw) > 8 * 1024 * 1024:
                raise ValueError('Gateway did not return a complete result.')
            result = json.loads(raw)
            if not result.get('ok'):
                code = result.get('code', '')
                if 'LEASE' in code or 'AUTH' in code or 'TOKEN' in code:
                    self.platform.fail(503, 'This company tool needs its server access set up by the owner.', 'tool_access_missing')
                self.platform.fail(502, 'This company tool did not answer. Try again shortly.', 'tool_unavailable')
            return result['data']
        except (OSError, ValueError, subprocess.TimeoutExpired):
            if process is not None and process.poll() is None:
                os.killpg(process.pid, signal.SIGTERM)
                try:
                    process.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=3)
            self.platform.fail(502, 'The company tool connection failed. Try again shortly.', 'tool_unavailable')
        finally:
            self.capacity.release()

    def listed(self, owner=False):
        with self.platform.database() as conn:
            rows = conn.execute('SELECT * FROM shared_tools ORDER BY name').fetchall()
        return [{**dict(row), 'enabled': bool(row['enabled']), 'schema': json.loads(row['schema_json'])}
                for row in rows if owner or row['enabled']]

    def call(self, user, id_, arguments):
        if not isinstance(arguments, dict):
            self.platform.fail(400, 'Tool fields must be a named list.')
        with self.platform.database() as conn:
            row = conn.execute('SELECT * FROM shared_tools WHERE id=? AND enabled=1', (id_,)).fetchone()
            if row is None:
                self.platform.fail(404, 'That company tool is not enabled.', 'unknown_tool')
            event = conn.execute('INSERT INTO tool_use(user_id,tool,created_at,outcome) VALUES(?,?,?,?)',
                                 (user['id'], id_, self.app.now(), 'in_progress')).lastrowid
        outcome = 'failed'
        try:
            if row['server'] == 'company-media':
                result = self.platform.media.call(user, id_, arguments)
            else:
                result = self.bridge('call_tool', {'server': row['server'], 'tool': row['tool'],
                                             'arguments': arguments, 'timeout_ms': 40000,
                                             'harness_id': 'floyd-company-platform'})
            outcome = 'complete'
            return result
        finally:
            with self.platform.database() as conn:
                conn.execute('UPDATE tool_use SET outcome=? WHERE id=?', (outcome, event))

    def text(self, result):
        blocks = result.get('content', []) if isinstance(result, dict) else []
        text = '\n'.join(block.get('text', '') for block in blocks if block.get('type') == 'text')
        for _ in range(3):
            try:
                parsed = json.loads(text)
            except (ValueError, TypeError):
                return text
            if not isinstance(parsed, str):
                return parsed
            text = parsed
        return text

    def route(self, handler, method, path, query):
        if path.startswith('/admin/api/tools'):
            self.platform.admin(handler, write=method not in ['GET', 'HEAD'])
            if path == '/admin/api/tools' and method == 'GET':
                return 200, {'tools': self.listed(True)}, 'json', {}
            if path == '/admin/api/tools/search' and method == 'GET':
                term = (query.get('query') or [''])[0][:200]
                result = self.bridge('search_tools', {'query': term, 'limit': 12})
                return 200, result, 'json', {}
            if path == '/admin/api/tools' and method == 'POST':
                body = handler.read_json()
                id_ = body.get('id')
                if not isinstance(id_, str) or not re.fullmatch('[a-zA-Z0-9_-]{1,64}', id_):
                    self.platform.fail(400, 'Use a short tool name.')
                with self.platform.database() as conn:
                    previous = conn.execute('SELECT * FROM shared_tools WHERE id=?', (id_,)).fetchone()
                server = body.get('server', previous['server'] if previous else '')
                tool = body.get('tool', previous['tool'] if previous else '')
                if server == 'company-media':
                    from media_tools import SCHEMAS
                    if id_ not in SCHEMAS or tool != id_:
                        self.platform.fail(400, 'Choose an installed company recording tool.')
                    schema = SCHEMAS[id_]
                else:
                    schema = self.bridge('describe_tool', {'server': server, 'tool': tool, 'harness_id': 'floyd-company-platform'})
                if not isinstance(schema.get('inputSchema'), dict):
                    self.platform.fail(502, 'This tool did not provide its required fields.')
                enabled = body.get('enabled', bool(previous['enabled']) if previous else True)
                if not isinstance(enabled, bool):
                    self.platform.fail(400, 'Choose whether the tool is enabled.')
                name = str(body.get('name', previous['name'] if previous else tool))[:120]
                with self.platform.database() as conn:
                    conn.execute('INSERT OR REPLACE INTO shared_tools VALUES(?,?,?,?,?,?)',
                                 (id_, name, server, tool, int(enabled), json.dumps(schema)))
                    self.platform.audit(conn, 'company tool saved', id_)
                return 200, {'id': id_}, 'json', {}
        if path not in ['/search', '/fetch', '/company/tools', '/company/tools/call']:
            return None
        with self.platform.database() as conn:
            user = self.app.require_bearer_user(conn, handler.headers)
        if path == '/company/tools' and method == 'GET':
            return 200, {'tools': self.listed()}, 'json', {}
        if method != 'POST':
            self.platform.fail(405, 'Use a submitted request for this action.')
        body = handler.read_json()
        if path == '/company/tools/call':
            return 200, self.call(user, body.get('id'), body.get('arguments', {})), 'json', {}
        if path == '/search':
            term = body.get('text_query')
            if not isinstance(term, str) or not term.strip() or len(term) > 2000:
                self.platform.fail(400, 'Enter a short search request.')
            result = self.text(self.call(user, 'web-search', {'search_query': term, 'location': 'us', 'content_size': 'medium'}))
            if not isinstance(result, (dict, list)):
                self.platform.fail(502, 'The search service returned an unreadable answer.')
            rows = result if isinstance(result, list) else result.get('search_results', result.get('results'))
            if not isinstance(rows, list):
                self.platform.fail(502, 'The search service did not return its results.')
            clean = [{'title': r.get('title', ''), 'url': r.get('url', r.get('link', '')),
                      'snippet': r.get('snippet', r.get('content', '')), 'date': r.get('date', ''),
                      'site_name': r.get('site_name', r.get('website', ''))} for r in rows if isinstance(r, dict)]
            return 200, {'search_results': clean}, 'json', {}
        address = body.get('url')
        if not isinstance(address, str) or len(address) > 4000:
            self.platform.fail(400, 'Enter a web page address.')
        parsed = urllib.parse.urlsplit(address)
        if parsed.scheme not in ['http', 'https'] or not parsed.hostname or parsed.username or parsed.password:
            self.platform.fail(400, 'Use a web page address without a password.')
        try:
            ip = ipaddress.ip_address(parsed.hostname)
            private = not ip.is_global
        except ValueError:
            private = parsed.hostname in ['localhost'] or parsed.hostname.endswith(('.local', '.home.arpa', '.ts.net'))
        if private:
            self.platform.fail(400, 'This tool reads public web pages. Choose a public address.')
        result = self.text(self.call(user, 'web-reader', {'url': address, 'return_format': 'markdown', 'retain_images': False, 'timeout': 20}))
        if isinstance(result, dict):
            result = result.get('reader_result', result.get('data', result))
            if isinstance(result, dict):
                result = result.get('content', result.get('markdown'))
        if not isinstance(result, str) or not result.strip():
            self.platform.fail(502, 'The page reader did not return readable page text.')
        return 200, result, 'text', {}
