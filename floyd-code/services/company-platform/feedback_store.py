"""Private member feedback and bounded, checked attachments."""

import hashlib
import json
import os
import re
import secrets


class FeedbackStore:
    def __init__(self, platform):
        self.platform = platform
        self.app = platform.app

    def initialize(self, conn):
        conn.executescript('''
            CREATE TABLE IF NOT EXISTS feedback (
                id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL,
                created_at INTEGER NOT NULL, body TEXT NOT NULL
            );
            CREATE TABLE IF NOT EXISTS feedback_uploads (
                id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL,
                feedback_id INTEGER NOT NULL REFERENCES feedback(id), token_hash TEXT NOT NULL,
                expires_at INTEGER NOT NULL, file_name TEXT NOT NULL, file_size INTEGER NOT NULL,
                file_hash TEXT NOT NULL, status TEXT NOT NULL, etag TEXT
            );
        ''')

    def route(self, handler, method, path, query):
        if path.startswith('/feedback/upload/'):
            if method != 'PUT':
                self.platform.fail(405, 'Use the supplied upload method.')
            id_ = path.rsplit('/', 1)[-1]
            token = (query.get('ticket') or [''])[0]
            with self.platform.database() as conn:
                row = conn.execute('SELECT f.* FROM feedback_uploads f JOIN users u ON u.id=f.user_id '
                                   "WHERE f.id=? AND f.token_hash=? AND f.expires_at>? AND f.status='issued' AND u.enabled=1",
                                   (id_, self.app.sha256_hex(token), self.app.now())).fetchone()
            if row is None:
                self.platform.fail(403, 'This upload link is invalid or expired.')
            raw = handler.read_body()
            if len(raw) != row['file_size'] or hashlib.sha256(raw).hexdigest() != row['file_hash']:
                self.platform.fail(400, 'The attachment size or contents do not match the supplied file.')
            etag = '"' + row['file_hash'] + '"'
            with self.platform.database() as conn:
                conn.execute('BEGIN IMMEDIATE')
                changed = conn.execute("UPDATE feedback_uploads SET status='uploaded',etag=? WHERE id=? AND status='issued'",
                                       (etag, row['id']))
                if changed.rowcount != 1:
                    self.platform.fail(409, 'This attachment was already uploaded.')
                target = self.app.UPLOAD_DIR / (str(row['id']) + '.bin')
                with os.fdopen(os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'wb') as stream:
                    stream.write(raw)
                    stream.flush()
                    os.fsync(stream.fileno())
            return 200, {}, 'json', {'ETag': etag}
        with self.platform.database() as conn:
            user = self.app.require_bearer_user(conn, handler.headers)
            body = handler.read_json()
            if path == '/feedback':
                if not isinstance(body.get('content'), str) or not body['content'].strip() or len(json.dumps(body)) > 256000:
                    self.platform.fail(400, 'Write feedback of no more than 256,000 characters.')
                result = conn.execute('INSERT INTO feedback(user_id,created_at,body) VALUES(?,?,?)',
                                      (user['id'], self.app.now(), json.dumps(body)))
                return 200, {'feedback_id': result.lastrowid}, 'json', {}
            if path == '/feedback/upload_url':
                feedback = conn.execute('SELECT id FROM feedback WHERE id=? AND user_id=?',
                                        (body.get('feedback_id'), user['id'])).fetchone()
                if feedback is None:
                    self.platform.fail(404, 'That feedback does not belong to your account.')
                size = body.get('file_size')
                name = body.get('file_name')
                digest = body.get('file_hash')
                if not isinstance(size, int) or not 0 <= size <= 32 * 1024 * 1024 or not isinstance(name, str) or not 1 <= len(name) <= 256:
                    self.platform.fail(400, 'Attachments must be no larger than 32 million bytes and have a short file name.')
                if not isinstance(digest, str) or not re.fullmatch('[a-fA-F0-9]{64}', digest):
                    self.platform.fail(400, 'The attachment needs its full contents check value.')
                ticket = secrets.token_urlsafe(40)
                result = conn.execute('INSERT INTO feedback_uploads(user_id,feedback_id,token_hash,expires_at,file_name,file_size,file_hash,status) '
                                      'VALUES(?,?,?,?,?,?,?,?)', (user['id'], feedback['id'], self.app.sha256_hex(ticket),
                                      self.app.now() + 1800, name, size, digest.lower(), 'issued'))
                return 200, {'upload': {'id': result.lastrowid, 'parts': [{'part_number': 1,
                    'url': self.app.PUBLIC_BASE + '/feedback/upload/' + str(result.lastrowid) + '?ticket=' + ticket,
                    'method': 'PUT', 'size': size}]}}, 'json', {}
            if path == '/feedback/upload_complete':
                row = conn.execute('SELECT * FROM feedback_uploads WHERE id=? AND user_id=?',
                                   (body.get('upload_id'), user['id'])).fetchone()
                if row is None:
                    self.platform.fail(404, 'That attachment does not belong to your account.')
                parts = body.get('parts')
                if row['status'] not in ['uploaded', 'complete'] or not isinstance(parts, list) or len(parts) != 1 or parts[0] != {'part_number': 1, 'etag': row['etag']}:
                    self.platform.fail(400, 'The attachment has not been completely uploaded.')
                conn.execute("UPDATE feedback_uploads SET status='complete' WHERE id=?", (row['id'],))
                return 200, {'saved': True}, 'json', {}
        self.platform.fail(404, 'That feedback action does not exist.')
