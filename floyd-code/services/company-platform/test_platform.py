import base64
import hashlib
import importlib.util
import json
import shutil
import tempfile
import threading
import unittest
import urllib.error
import urllib.request
from http.server import ThreadingHTTPServer
from pathlib import Path


class PlatformTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='floyd-platform-test-')
        self.home = Path(self.temp.name)
        source = Path(__file__).parent
        for name in ['app.py', 'company_platform.py', 'model_access.py', 'feedback_store.py', 'shared_tools.py', 'media_tools.py', 'gateway_bridge.mjs', 'platform.html', 'platform.js', 'platform.css']:
            if (source / name).exists():
                shutil.copy2(source / name, self.home / name)
        self.key = 'test-owner-key-with-sufficient-length'
        salt = bytes.fromhex('18' * 24)
        config = {'public_base_url': 'http://127.0.0.1:18443', 'password_salt': salt.hex(),
                  'password_hash': hashlib.pbkdf2_hmac('sha256', self.key.encode(), salt, 240000).hex(),
                  'csrf_secret': 'test-csrf-secret', 'catalog_url': '', 'company_model_base': ''}
        (self.home / 'config.json').write_text(json.dumps(config))
        import sys
        sys.path.insert(0, str(self.home))
        sys.modules.pop('company_platform', None)
        sys.modules.pop('model_access', None)
        sys.modules.pop('feedback_store', None)
        sys.modules.pop('shared_tools', None)
        sys.modules.pop('media_tools', None)
        spec = importlib.util.spec_from_file_location('auth_under_test', self.home / 'app.py')
        self.app = importlib.util.module_from_spec(spec)
        sys.modules['auth_under_test'] = self.app
        spec.loader.exec_module(self.app)
        self.app.init_db()
        if hasattr(self.app, 'PLATFORM'):
            self.app.PLATFORM.initialize()
        self.server = ThreadingHTTPServer(('127.0.0.1', 0), self.app.Handler)
        self.origin = 'http://127.0.0.1:' + str(self.server.server_port)
        self.app.PUBLIC_BASE = self.origin
        self.app.CONFIG['public_base_url'] = self.origin
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()
        self.temp.cleanup()

    def request(self, path, method='GET', body=None, key=None, headers=None):
        hs = dict(headers or {})
        if key is not None:
            hs['Authorization'] = 'Bearer ' + key
        data = None
        if body is not None:
            data = json.dumps(body).encode()
            hs['Content-Type'] = 'application/json'
        req = urllib.request.Request(self.origin + path, data=data, method=method, headers=hs)
        try:
            response = urllib.request.urlopen(req, timeout=5)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read()
            payload = json.loads(raw) if 'json' in response.headers.get('Content-Type', '') else raw.decode()
            return response.status, payload, dict(response.headers)

    def form(self, path, body):
        request = urllib.request.Request(self.origin + path, data=urllib.parse.urlencode(body).encode())
        try:
            response = urllib.request.urlopen(request, timeout=5)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.status, json.load(response)

    def member(self, name):
        _, owner, _ = self.request('/admin/api/overview', key=self.key)
        _, invitation, _ = self.request('/admin/api/invites', 'POST', {'username': name}, self.key,
                                          {'X-CSRF-Token': owner['csrf']})
        status, account, headers = self.request('/account/api/join', 'POST',
            {'invitation': invitation['invitation'], 'password': 'a-long-member-test-password'})
        self.assertEqual(status, 201)
        return account, {'Cookie': headers['Set-Cookie'].split(';')[0], 'X-CSRF-Token': account['csrf']}

    def device(self, member_headers):
        _, device = self.form('/api/oauth/device_authorization', {'client_id': 'test-client'})
        self.assertEqual(self.request('/account/api/approve', 'POST', {'user_code': device['user_code']},
                                      headers=member_headers)[0], 200)
        status, token = self.form('/api/oauth/token', {'client_id': 'test-client',
            'grant_type': 'urn:ietf:params:oauth:grant-type:device_code', 'device_code': device['device_code']})
        self.assertEqual(status, 200)
        return device, token

    def test_admin_directory_is_protected_and_usable(self):
        status, _, _ = self.request('/admin/api/overview')
        self.assertEqual(status, 401)
        status, data, _ = self.request('/admin/api/overview', key=self.key)
        self.assertEqual(status, 200)
        self.assertIn('catalog', data)
        self.assertIn('csrf', data)
        self.assertNotIn(self.key, json.dumps(data))

    def test_each_request_on_a_kept_connection_checks_its_own_key(self):
        import http.client
        connection = http.client.HTTPConnection('127.0.0.1', self.server.server_port, timeout=5)
        try:
            for key, expected in [(self.key, 200), ('an-incorrect-key', 401)]:
                connection.request('POST', '/admin/api/login', json.dumps({'key': key}),
                    {'Content-Type': 'application/json'})
                response = connection.getresponse()
                response.read()
                self.assertEqual(response.status, expected)
        finally:
            connection.close()

    def test_media_tools_require_a_member_and_reject_unsafe_input(self):
        body = {'id': 'speech-transcribe', 'arguments': {'audio_base64': 'bad', 'filename': '../recording.wav'}}
        self.assertEqual(self.request('/company/tools/call', 'POST', body)[0], 401)
        _, headers = self.member('audio-member')
        _, token = self.device(headers)
        status, data, _ = self.request('/company/tools', key=token['access_token'])
        self.assertEqual(status, 200)
        self.assertTrue({'speech-transcribe', 'music-create'} <= {tool['id'] for tool in data['tools']})
        self.assertEqual(self.request('/company/tools/call', 'POST', body, token['access_token'])[0], 400)
        body = {'id': 'music-create', 'arguments': {'caption': 'Soft piano', 'seconds': 5}}
        status, data, _ = self.request('/company/tools/call', 'POST', body, token['access_token'])
        self.assertEqual(status, 400)
        self.assertIn('destination', data['error_description'].lower())

    def test_sign_out_blocks_browser_saved_basic_credentials(self):
        saved = {'Authorization': 'Basic ' + base64.b64encode(('owner:' + self.key).encode()).decode()}
        _, info, _ = self.request('/admin/api/overview', headers=saved)
        status, _, headers = self.request('/admin/api/logout', 'POST', {}, headers={
            **saved, 'X-CSRF-Token': info['csrf']})
        self.assertEqual(status, 200)
        saved['Cookie'] = headers['Set-Cookie'].split(';')[0]
        self.assertEqual(self.request('/admin/api/overview', headers=saved)[0], 401)
        status, _, login = self.request('/admin/api/login', 'POST', {'key': self.key}, headers=saved)
        self.assertEqual(status, 200)
        saved['Cookie'] = login['Set-Cookie'].split(';')[0]
        self.assertEqual(self.request('/admin/api/overview', headers=saved)[0], 200)

    def test_owner_approval_uses_the_current_page_with_cookie_sign_in(self):
        _, _, login = self.request('/admin/api/login', 'POST', {'key': self.key})
        status, page, _ = self.request('/approve?user_code=TEST-CODE', headers={
            'Cookie': login['Set-Cookie'].split(';')[0]})
        self.assertEqual(status, 200)
        self.assertIn('/platform.js', page)

    def test_key_replacement_cancels_old_access_and_rejects_unprotected_writes(self):
        _, info, _ = self.request('/admin/api/overview', key=self.key)
        status, _, _ = self.request('/admin/api/key/rotate', 'POST', {}, key=self.key)
        self.assertEqual(status, 403)
        status, data, _ = self.request('/admin/api/key/rotate', 'POST', {}, key=self.key,
                                       headers={'X-CSRF-Token': info['csrf']})
        self.assertEqual(status, 200)
        self.assertEqual(self.request('/admin/api/overview', key=self.key)[0], 401)
        self.assertEqual(self.request('/admin/api/overview', key=data['new_key'])[0], 200)
        self.assertNotIn(data['new_key'], (self.home / 'config.json').read_text())
        self.app.CONFIG.clear()
        self.app.CONFIG.update(json.loads((self.home / 'config.json').read_text()))
        self.assertEqual(self.request('/admin/api/overview', key=data['new_key'])[0], 200)

    def test_admin_cookie_and_cross_site_protection(self):
        status, info, headers = self.request('/admin/api/login', 'POST', {'key': self.key})
        self.assertEqual(status, 200)
        cookie = headers['Set-Cookie'].split(';')[0]
        status, _, _ = self.request('/admin/api/users', headers={'Cookie': cookie})
        self.assertEqual(status, 200)
        status, _, _ = self.request('/admin/api/key/rotate', 'POST', {},
            headers={'Cookie': cookie, 'X-CSRF-Token': info['csrf'], 'Origin': 'https://another.example'})
        self.assertEqual(status, 403)

    def test_catalog_refresh_retains_prices_and_last_good_copy(self):
        from http.server import BaseHTTPRequestHandler
        catalog = {'example': {'id': 'example', 'name': 'Example Provider',
            'api': 'https://provider.example/v1', 'npm': '@ai-sdk/openai-compatible',
            'models': {'coding': {'id': 'coding', 'name': 'Coding Model',
                'limit': {'context': 64000, 'output': 8000}, 'cost': {'input': 2.0, 'output': 8.0},
                'last_updated': '2026-10-01', 'tool_call': True}}}}
        class Source(BaseHTTPRequestHandler):
            payload = catalog
            def do_GET(self):
                raw = json.dumps(self.payload).encode()
                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.send_header('Content-Length', str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)
            def log_message(self, *args):
                pass
        source = ThreadingHTTPServer(('127.0.0.1', 0), Source)
        thread = threading.Thread(target=source.serve_forever, daemon=True)
        thread.start()
        try:
            config = json.loads((self.home / 'config.json').read_text())
            config['catalog_url'] = 'http://127.0.0.1:' + str(source.server_port)
            (self.home / 'config.json').write_text(json.dumps(config))
            _, info, _ = self.request('/admin/api/overview', key=self.key)
            headers = {'X-CSRF-Token': info['csrf']}
            status, _, _ = self.request('/admin/api/catalog/refresh', 'POST', {}, self.key, headers)
            self.assertEqual(status, 200)
            self.assertEqual(self.request('/catalog/api.json')[1], catalog)
            Source.payload = {'bad': {'models': None}}
            self.assertEqual(self.request('/admin/api/catalog/refresh', 'POST', {}, self.key, headers)[0], 502)
            self.assertEqual(self.request('/catalog/api.json')[1], catalog)
            self.assertIsNotNone(self.request('/catalog/info')[1]['last_error'])
        finally:
            source.shutdown()
            source.server_close()
            thread.join()

    def test_separate_member_accounts_and_device_approval(self):
        _, owner, _ = self.request('/admin/api/overview', key=self.key)
        hs = {'X-CSRF-Token': owner['csrf']}
        status, invitation, _ = self.request('/admin/api/invites', 'POST', {'username': 'member-one'}, self.key, hs)
        self.assertEqual(status, 201)
        status, account, headers = self.request('/account/api/join', 'POST',
            {'invitation': invitation['invitation'], 'password': 'a-long-member-test-password'})
        self.assertEqual(status, 201)
        cookie = headers['Set-Cookie'].split(';')[0]
        member_hs = {'Cookie': cookie, 'X-CSRF-Token': account['csrf']}
        self.assertEqual(self.request('/admin/api/users', headers=member_hs)[0], 401)
        self.assertEqual(self.request('/account/api/join', 'POST',
            {'invitation': invitation['invitation'], 'password': 'another-long-test-password'})[0], 400)
        raw = urllib.parse.urlencode({'client_id': 'test-client'}).encode()
        req = urllib.request.Request(self.origin + '/api/oauth/device_authorization', data=raw)
        with urllib.request.urlopen(req, timeout=5) as response:
            device = json.load(response)
        status, _, _ = self.request('/account/api/approve', 'POST', {'user_code': device['user_code']}, headers=member_hs)
        self.assertEqual(status, 200)
        status, devices, _ = self.request('/account/api/devices', headers=member_hs)
        self.assertEqual(status, 200)
        self.assertEqual(len(devices['devices']), 1)
        user_id = account['user']['id']
        self.assertEqual(self.request('/admin/api/users/' + user_id + '/disable', 'POST', {}, self.key, hs)[0], 200)
        self.assertEqual(self.request('/account/api/me', headers=member_hs)[0], 401)

    def test_device_revoke_refresh_replay_and_cross_member_denial(self):
        one, one_headers = self.member('one')
        _, two_headers = self.member('two')
        device, token = self.device(one_headers)
        self.assertEqual(self.request('/me', key=token['access_token'])[0], 200)
        self.assertEqual(self.request('/account/api/revoke', 'POST', {'device_code': device['device_code']},
                                      headers=two_headers)[0], 404)
        status, refreshed = self.form('/api/oauth/token', {'client_id': 'test-client',
            'grant_type': 'refresh_token', 'refresh_token': token['refresh_token']})
        self.assertEqual(status, 200)
        self.assertEqual(self.form('/api/oauth/token', {'client_id': 'test-client',
            'grant_type': 'refresh_token', 'refresh_token': token['refresh_token']})[0], 400)
        self.assertEqual(self.request('/account/api/revoke', 'POST', {'device_code': device['device_code']},
                                      headers=one_headers)[0], 200)
        self.assertEqual(self.request('/me', key=refreshed['access_token'])[0], 401)
        self.assertEqual(self.request('/me', key=token['access_token'])[0], 401)
        self.assertEqual(self.form('/api/oauth/token', {'client_id': 'test-client',
            'grant_type': 'refresh_token', 'refresh_token': refreshed['refresh_token']})[0], 400)

    def test_device_expiry_and_single_exchange(self):
        from concurrent.futures import ThreadPoolExecutor
        _, member_headers = self.member('one')
        _, device = self.form('/api/oauth/device_authorization', {'client_id': 'test-client'})
        self.request('/account/api/approve', 'POST', {'user_code': device['user_code']}, headers=member_headers)
        grant = {'client_id': 'test-client', 'grant_type': 'urn:ietf:params:oauth:grant-type:device_code',
                 'device_code': device['device_code']}
        with ThreadPoolExecutor(max_workers=2) as pool:
            codes = list(pool.map(lambda _: self.form('/api/oauth/token', grant)[0], [1, 2]))
        self.assertEqual(sorted(codes), [200, 400])
        _, expired = self.form('/api/oauth/device_authorization', {'client_id': 'test-client'})
        self.request('/account/api/approve', 'POST', {'user_code': expired['user_code']}, headers=member_headers)
        with self.app.PLATFORM.database() as conn:
            conn.execute('UPDATE device_codes SET expires_at=0 WHERE device_code=?', (expired['device_code'],))
        grant['device_code'] = expired['device_code']
        self.assertEqual(self.form('/api/oauth/token', grant)[0], 400)

    def test_model_forwarding_usage_streaming_and_limit(self):
        from http.server import BaseHTTPRequestHandler
        calls = []
        class Provider(BaseHTTPRequestHandler):
            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                calls.append((self.path, body))
                if body.get('stream'):
                    raw = b'data: {"choices":[{"delta":{"content":"hello"}}]}\n\n'
                    raw += b'data: {"usage":{"prompt_tokens":8,"completion_tokens":2},"choices":[]}\n\ndata: [DONE]\n\n'
                    content_type = 'text/event-stream'
                else:
                    raw = json.dumps({'choices': [{'message': {'role': 'assistant', 'content': 'hello'}}],
                                      'usage': {'prompt_tokens': 8, 'completion_tokens': 2}}).encode()
                    content_type = 'application/json'
                self.send_response(200)
                self.send_header('Content-Type', content_type)
                self.send_header('Content-Length', str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)
            def log_message(self, *args):
                pass
        provider = ThreadingHTTPServer(('127.0.0.1', 0), Provider)
        thread = threading.Thread(target=provider.serve_forever, daemon=True)
        thread.start()
        try:
            member, member_headers = self.member('one')
            _, token = self.device(member_headers)
            models = {'coding': {'name': 'Coding', 'limit': {'context': 8192}, 'cost': {'input': 2, 'output': 8}}}
            with self.app.PLATFORM.database() as conn:
                conn.execute('INSERT INTO provider_settings VALUES(?,?,?,?,?,?,?,?)',
                    ('test-provider', 'Test', 'http://127.0.0.1:' + str(provider.server_port),
                     'openai', '', 1, json.dumps(models), self.app.now()))
                conn.execute('UPDATE users SET monthly_request_limit=2 WHERE id=?', (member['user']['id'],))
            body = {'model': 'test-provider/coding', 'messages': [{'role': 'user', 'content': 'hello'}]}
            self.assertEqual(self.request('/v1/chat/completions', 'POST', body)[0], 401)
            status, data, _ = self.request('/v1/chat/completions', 'POST', body, token['access_token'])
            self.assertEqual(status, 200)
            self.assertEqual(data['choices'][0]['message']['content'], 'hello')
            body['stream'] = True
            status, text, _ = self.request('/v1/chat/completions', 'POST', body, token['access_token'])
            self.assertEqual(status, 200)
            self.assertIn('[DONE]', text)
            self.assertEqual(calls[0][1]['model'], 'coding')
            self.assertTrue(calls[1][1]['stream_options']['include_usage'])
            status, usage, _ = self.request('/usages', key=token['access_token'])
            self.assertEqual(status, 200)
            self.assertEqual(usage['company_usage']['requests'], 2)
            self.assertEqual(usage['company_usage']['outcomes']['complete']['input_count'], 16)
            self.assertAlmostEqual(usage['company_usage']['outcomes']['complete']['cost_usd'], 0.000064)
            self.assertEqual(usage['usages']['limit_month_total']['used_ratio'], 1)
            self.assertEqual(self.request('/v1/chat/completions', 'POST', body, token['access_token'])[0], 429)
            self.assertEqual(len(calls), 2)
            with self.app.PLATFORM.database() as conn:
                conn.execute('UPDATE users SET monthly_request_limit=3 WHERE id=?', (member['user']['id'],))
                conn.execute('INSERT INTO provider_settings VALUES(?,?,?,?,?,?,?,?)',
                    ('company-local', 'Local models', 'http://127.0.0.1:' + str(provider.server_port),
                     'openai', '', 1, json.dumps(models), self.app.now()))
            native = {'model': 'company-local/coding', 'messages': [{'role': 'user', 'content': 'hello'}],
                      'thinking': {'type': 'disabled'}, 'max_completion_tokens': 128, 'stream': True}
            self.assertEqual(self.request('/v1/chat/completions', 'POST', native, token['access_token'])[0], 200)
            forwarded = calls[-1][1]
            self.assertNotIn('thinking', forwarded)
            self.assertNotIn('max_completion_tokens', forwarded)
            self.assertFalse(forwarded['chat_template_kwargs']['enable_thinking'])
            self.assertEqual(forwarded['max_tokens'], 128)
        finally:
            provider.shutdown()
            provider.server_close()
            thread.join()

    def test_feedback_attachment_is_saved_and_other_members_are_denied(self):
        _, one_headers = self.member('one')
        _, two_headers = self.member('two')
        _, token = self.device(one_headers)
        _, other = self.device(two_headers)
        _, feedback, _ = self.request('/feedback', 'POST', {'content': 'A test report'}, token['access_token'])
        raw = b'test attachment bytes'
        body = {'feedback_id': feedback['feedback_id'], 'file_name': 'report.txt', 'file_size': len(raw),
                'file_hash': hashlib.sha256(raw).hexdigest()}
        self.assertEqual(self.request('/feedback/upload_url', 'POST', body, other['access_token'])[0], 404)
        status, upload, _ = self.request('/feedback/upload_url', 'POST', body, token['access_token'])
        self.assertEqual(status, 200)
        part = upload['upload']['parts'][0]
        req = urllib.request.Request(part['url'], data=raw, method='PUT')
        with urllib.request.urlopen(req, timeout=5) as response:
            self.assertEqual(response.status, 200)
            etag = response.headers['ETag']
        complete = {'upload_id': upload['upload']['id'], 'parts': [{'part_number': 1, 'etag': etag}]}
        self.assertEqual(self.request('/feedback/upload_complete', 'POST', complete, other['access_token'])[0], 404)
        self.assertEqual(self.request('/feedback/upload_complete', 'POST', complete, token['access_token'])[0], 200)
        self.assertEqual((self.home / 'uploads' / (str(complete['upload_id']) + '.bin')).read_bytes(), raw)
        self.assertEqual(self.request('/admin/api/feedback', key=token['access_token'])[0], 401)
        self.assertEqual(self.request('/admin/api/feedback', key=self.key)[1]['feedback'][0]['body']['content'], 'A test report')

    def test_shared_web_tools_return_content_and_reject_unapproved_tools(self):
        import sys
        worker = self.home / 'fixture-worker.py'
        worker.write_text('import json,sys\nr=json.load(sys.stdin)\n'
            'd={"search_results":[{"title":"Found page","link":"https://example.com/page","content":"Found text"}]} if r["arguments"]["tool"]=="web_search_prime" else {"reader_result":{"content":"Read page text"}}\n'
            'if r["arguments"]["tool"]=="web_search_prime": d=d["search_results"]\n'
            'print(json.dumps({"ok":True,"data":{"content":[{"type":"text","text":json.dumps(json.dumps(d))}]}}))\n')
        config = json.loads((self.home / 'config.json').read_text())
        config['company_gateway_command'] = [sys.executable, str(worker)]
        (self.home / 'config.json').write_text(json.dumps(config))
        _, headers = self.member('reader')
        _, token = self.device(headers)
        self.assertEqual(self.request('/search', 'POST', {'text_query': 'test'})[0], 401)
        status, result, _ = self.request('/search', 'POST', {'text_query': 'test'}, token['access_token'])
        self.assertEqual(status, 200)
        self.assertEqual(result['search_results'][0]['url'], 'https://example.com/page')
        self.assertEqual(result['search_results'][0]['snippet'], 'Found text')
        status, page, response_headers = self.request('/fetch', 'POST', {'url': 'https://example.com/page'}, token['access_token'])
        self.assertEqual(status, 200)
        self.assertEqual(page, 'Read page text')
        self.assertIn('text/plain', response_headers['Content-Type'])
        self.assertEqual(self.request('/company/tools/call', 'POST', {'id': 'unapproved', 'arguments': {}}, token['access_token'])[0], 404)
        self.assertEqual(self.request('/tools', 'POST', {'method': 'unknown'}, token['access_token'])[0], 404)

    def test_local_library_keeps_speech_music_and_embeddings_out_of_chat(self):
        from http.server import BaseHTTPRequestHandler
        class Source(BaseHTTPRequestHandler):
            def do_GET(self):
                raw = json.dumps({'data': [{'id': 'chat'}, {'id': 'compare'}]}).encode()
                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.send_header('Content-Length', str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)
            def log_message(self, *args):
                pass
        source = ThreadingHTTPServer(('127.0.0.1', 0), Source)
        thread = threading.Thread(target=source.serve_forever, daemon=True)
        thread.start()
        try:
            settings = self.home / 'models.yaml'
            settings.write_text('models:\n  chat:\n    cmd: server --ctx-size 8192 --jinja\n    name: Local Chat\n  compare:\n    cmd: server --ctx-size 8192 --embedding\n    name: Local Matching\n')
            config = json.loads((self.home / 'config.json').read_text())
            config['company_model_settings'] = str(settings)
            (self.home / 'config.json').write_text(json.dumps(config))
            (self.app.DATA_DIR / 'local-library.json').write_text(json.dumps({
                'source': 'test library', 'checked_at': '2026-10-05', 'models': [
                    {'id': 'speech', 'kind': 'speech', 'display_name': 'Speech', 'status': 'Needs a speech service', 'hub_url': ''},
                    {'id': 'music', 'kind': 'music', 'display_name': 'Music', 'status': 'Needs a music service', 'hub_url': ''}]}))
            with self.app.PLATFORM.database() as conn:
                conn.execute('INSERT INTO provider_settings VALUES(?,?,?,?,?,?,?,?)',
                    ('company-local', 'Company Models', 'http://127.0.0.1:' + str(source.server_port),
                     'openai', '', 1, '{}', self.app.now()))
            _, owner, _ = self.request('/admin/api/overview', key=self.key)
            status, data, _ = self.request('/admin/api/connections/company-local/refresh', 'POST', {}, self.key,
                {'X-CSRF-Token': owner['csrf']})
            self.assertEqual(status, 200, data)
            self.assertEqual(data['models'], 4)
            models = self.request('/catalog/api.json')[1]['company-company-local']['models']
            self.assertEqual(models['company-local/compare']['kind'], 'embedding')
            self.assertEqual(models['company-local/music']['status'], 'Needs a music service')
            _, headers = self.member('local-member')
            _, token = self.device(headers)
            available = self.request('/v1/models', key=token['access_token'])[1]['data']
            self.assertEqual([model['id'] for model in available], ['company-local/chat'])
            body = {'model': 'company-local/speech', 'messages': []}
            self.assertEqual(self.request('/v1/chat/completions', 'POST', body, token['access_token'])[0], 409)
        finally:
            source.shutdown()
            source.server_close()
            thread.join()


if __name__ == '__main__':
    unittest.main()
