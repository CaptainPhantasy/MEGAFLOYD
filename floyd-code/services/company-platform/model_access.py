"""Authenticated model forwarding and measured company use."""

import calendar
import datetime
import json
import math
import socket
import urllib.error
import urllib.request

from company_platform import NoRedirect


class ModelAccess:
    def __init__(self, platform):
        self.platform = platform
        self.app = platform.app

    def month(self):
        current = datetime.datetime.now(datetime.timezone.utc)
        start = current.replace(day=1, hour=0, minute=0, second=0, microsecond=0)
        end = start.replace(year=start.year + 1, month=1) if start.month == 12 else start.replace(month=start.month + 1)
        return int(start.timestamp()), int(end.timestamp())

    def usage(self, conn, user):
        start, end = self.month()
        rows = conn.execute('SELECT outcome, COUNT(*) AS requests, SUM(input_count) AS input_count, '
                            'SUM(output_count) AS output_count, SUM(cost_usd) AS cost_usd '
                            'FROM usage_events WHERE user_id=? AND created_at>=? GROUP BY outcome',
                            (user['id'], start)).fetchall()
        outcomes = {row['outcome']: dict(row) for row in rows}
        used = sum(row['requests'] for row in rows if row['outcome'] in ['complete', 'in_progress', 'interrupted'])
        limit = user['monthly_request_limit'] or self.platform.config().get('monthly_request_limit')
        quota = {'used_ratio': min(1, used / limit), 'reset_time': self.app.iso(end)} if limit else None
        return {'usages': {'limit_month_total': quota, 'limit_month_code': quota}, 'boosterWallet': None,
                'company_usage': {'month_started': self.app.iso(start), 'resets_at': self.app.iso(end),
                                  'requests': used, 'request_limit': limit,
                                  'outcomes': outcomes, 'prices_are_estimates': True}}

    def reserve(self, user, model):
        start, _ = self.month()
        with self.platform.database() as conn:
            conn.execute('BEGIN IMMEDIATE')
            current = conn.execute('SELECT * FROM users WHERE id=? AND enabled=1', (user['id'],)).fetchone()
            if current is None:
                self.platform.fail(401, 'Your account is disabled.', 'invalid_token')
            limit = current['monthly_request_limit'] or self.platform.config().get('monthly_request_limit')
            if limit:
                used = conn.execute("SELECT COUNT(*) FROM usage_events WHERE user_id=? AND created_at>=? "
                                    "AND outcome IN ('complete','in_progress','interrupted')", (user['id'], start)).fetchone()[0]
                if used >= limit:
                    self.platform.fail(429, 'Your company request allowance has been used. Contact the owner.', 'usage_limit')
            result = conn.execute('INSERT INTO usage_events(user_id,model,created_at,outcome) VALUES(?,?,?,?)',
                                  (user['id'], model, self.app.now(), 'in_progress'))
            return result.lastrowid

    def finish(self, event, usage, model, outcome):
        if not isinstance(usage, dict):
            usage = {}
        input_count = usage.get('prompt_tokens', usage.get('input_tokens'))
        output_count = usage.get('completion_tokens', usage.get('output_tokens'))
        input_count = input_count if isinstance(input_count, int) and input_count >= 0 else None
        output_count = output_count if isinstance(output_count, int) and output_count >= 0 else None
        prices = model.get('cost') or {}
        costs = [prices.get('input'), prices.get('output')]
        cost = None
        if input_count is not None and output_count is not None and all(
                isinstance(value, (int, float)) and math.isfinite(value) and value >= 0 for value in costs):
            cost = (input_count * costs[0] + output_count * costs[1]) / 1_000_000
        with self.platform.database() as conn:
            conn.execute('UPDATE usage_events SET input_count=?,output_count=?,cost_usd=?,outcome=? WHERE id=?',
                         (input_count, output_count, cost, outcome, event))

    def relay(self, handler, path):
        with self.platform.database() as conn:
            user = self.app.require_bearer_user(conn, handler.headers)
        body = handler.read_json()
        public_model = body.get('model')
        if not isinstance(public_model, str) or '/' not in public_model:
            self.platform.fail(400, 'Choose a model from the company model list.', 'unknown_model')
        provider_id, model_id = public_model.split('/', 1)
        with self.platform.database() as conn:
            provider = conn.execute('SELECT * FROM provider_settings WHERE id=? AND enabled=1', (provider_id,)).fetchone()
        if provider is None:
            self.platform.fail(400, 'That company provider is disabled or unknown.', 'unknown_provider')
        models = json.loads(provider['models_json'])
        model = models.get(model_id)
        if not isinstance(model, dict):
            self.platform.fail(400, 'That model is not in the company model list.', 'unknown_model')
        formats = {'openai': '/chat/completions', 'openai_responses': '/responses', 'anthropic': '/messages'}
        if model.get('kind') in ['speech', 'music']:
            self.platform.fail(409, model.get('status', 'This model needs a separate service.'), 'model_service_needed')
        endpoint = '/embeddings' if model.get('kind') == 'embedding' else formats[provider['protocol']]
        if path.removeprefix('/v1') != endpoint:
            self.platform.fail(400, 'This provider needs a different request format.', 'wrong_format')
        body['model'] = model_id
        if provider_id == 'company-local' and endpoint == '/chat/completions':
            thinking = body.pop('thinking', None)
            if isinstance(thinking, dict) and thinking.get('type') in ['enabled', 'disabled']:
                body['chat_template_kwargs'] = {**(body.get('chat_template_kwargs') or {}),
                                                'enable_thinking': thinking['type'] == 'enabled'}
            if 'max_completion_tokens' in body:
                body.setdefault('max_tokens', body.pop('max_completion_tokens'))
        if endpoint == '/embeddings' and 'input' not in body:
            self.platform.fail(400, 'Enter the text to compare.', 'missing_input')
        if body.get('stream') and provider['protocol'] == 'openai':
            body['stream_options'] = {**(body.get('stream_options') or {}), 'include_usage': True}
        headers = {'Content-Type': 'application/json', 'Accept': 'text/event-stream' if body.get('stream') else 'application/json',
                   'User-Agent': 'Floyd-Code-Platform/1.0'}
        if provider['protocol'] == 'anthropic':
            headers['anthropic-version'] = '2023-06-01'
            if provider['secret']:
                headers['x-api-key'] = provider['secret']
        elif provider['secret']:
            headers['Authorization'] = 'Bearer ' + provider['secret']
        request = urllib.request.Request(provider['base_url'] + endpoint, data=json.dumps(body).encode(), headers=headers)
        event = self.reserve(user, public_model)
        usage = {}
        outcome = 'failed'
        started = False
        try:
            response = urllib.request.build_opener(NoRedirect).open(request, timeout=120)
            with response:
                content_type = response.headers.get('Content-Type', '')
                streaming = 'text/event-stream' in content_type
                if streaming:
                    complete = False
                    handler.send_response(response.status)
                    handler.send_header('Content-Type', 'text/event-stream; charset=utf-8')
                    handler.send_header('Cache-Control', 'no-store')
                    handler.send_header('X-Accel-Buffering', 'no')
                    handler.send_header('Connection', 'close')
                    handler.security_headers()
                    handler.end_headers()
                    started = True
                    handler.close_connection = True
                    pending = b''
                    while True:
                        chunk = response.read1(16384)
                        if not chunk:
                            break
                        handler.wfile.write(chunk)
                        handler.wfile.flush()
                        pending += chunk
                        if len(pending) > 4 * 1024 * 1024:
                            raise ValueError('Model sent an oversized response line.')
                        while b'\n' in pending:
                            line, pending = pending.split(b'\n', 1)
                            if line.startswith(b'data:'):
                                if line[5:].strip() == b'[DONE]':
                                    complete = True
                                    continue
                                try:
                                    data = json.loads(line[5:].strip())
                                except (ValueError, UnicodeDecodeError):
                                    continue
                                if not isinstance(data, dict):
                                    continue
                                if data.get('type') in ['message_stop', 'response.completed']:
                                    complete = True
                                if data.get('error') or data.get('type') in ['error', 'response.failed']:
                                    raise ValueError('Model reported a failed response.')
                                measured = data.get('usage') or (data.get('message') or {}).get('usage') or (data.get('response') or {}).get('usage')
                                if isinstance(measured, dict):
                                    usage.update(measured)
                    outcome = 'complete' if complete else 'interrupted'
                    return 0, None, 'sent', {}
                raw = response.read(32 * 1024 * 1024 + 1)
                if len(raw) > 32 * 1024 * 1024:
                    raise ValueError('Model response was too large.')
                payload = json.loads(raw)
                usage = payload.get('usage', {})
                outcome = 'complete'
                return response.status, payload, 'json', {}
        except (BrokenPipeError, ConnectionResetError):
            outcome = 'interrupted'
            return 0, None, 'sent', {}
        except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError, socket.timeout, ValueError):
            if started:
                outcome = 'interrupted'
                handler.wfile.write(b'data: {"error":{"message":"The company model connection was interrupted."}}\n\n')
                handler.wfile.flush()
                return 0, None, 'sent', {}
            self.platform.fail(502, 'The company model did not answer. The owner can check its connection.', 'provider_unavailable')
        finally:
            self.finish(event, usage, model, outcome)
