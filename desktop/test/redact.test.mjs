import assert from 'node:assert/strict';
import test from 'node:test';

import { REDACTED, SUPPRESSED_URL, redactSecrets, sanitizeState } from '../src/redact.mjs';

test('redacts a tokenised loopback URL fragment', () => {
  const line = 'Local:  http://127.0.0.1:58627/sessions/#token=test-token-one';
  assert.equal(redactSecrets(line), `Local:  http://127.0.0.1:58627/sessions/#token=${REDACTED}`);
});

test('redacts token query parameters and leaves other parameters intact', () => {
  const line = 'see https://example.test/path?mode=1&token=test-token-two&next=2';
  assert.equal(redactSecrets(line), `see https://example.test/path?mode=1&token=${REDACTED}&next=2`);
});

test('redacts every token occurrence on a line', () => {
  const line = '#token=test-token-three and ?token=test-token-four';
  assert.equal(redactSecrets(line), `#token=${REDACTED} and ?token=${REDACTED}`);
});

test('redacts the engine Token: banner', () => {
  const line = 'Token:    test-token-five';
  assert.equal(redactSecrets(line), `Token:    ${REDACTED}`);
});

test('suppresses a non-loopback Web UI URL', () => {
  const line = 'Web UI: https://example-tailnet.ts.net/sessions/#token=test-token-six';
  assert.equal(redactSecrets(line), `Web UI: ${SUPPRESSED_URL}`);
});

test('keeps a loopback Web UI URL', () => {
  const line = 'Web UI: http://127.0.0.1:58627/sessions/#token=test-token-seven';
  assert.equal(redactSecrets(line), `Web UI: http://127.0.0.1:58627/sessions/#token=${REDACTED}`);
});

test('keeps localhost Web UI URLs', () => {
  const line = 'Web UI: http://localhost:58627/sessions/';
  assert.equal(redactSecrets(line), line);
});

test('leaves unrelated engine output untouched', () => {
  const line = 'Server: started on 127.0.0.1:58627 (loopback)';
  assert.equal(redactSecrets(line), line);
});

test('sanitizeState strips the token from the renderer copy only', () => {
  const state = {
    status: 'ready',
    url: 'http://127.0.0.1:58627/sessions/#token=test-token-eight',
    port: 58627,
  };
  const safe = sanitizeState(state);
  assert.equal(safe.url, `http://127.0.0.1:58627/sessions/#token=${REDACTED}`);
  assert.equal(safe.port, 58627);
  assert.equal(state.url, 'http://127.0.0.1:58627/sessions/#token=test-token-eight');
});
