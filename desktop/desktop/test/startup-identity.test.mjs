import assert from 'node:assert/strict';
import test from 'node:test';
import { fetchRuntime, fetchStartupMeta } from '../src/meta.mjs';

const host = 'unix:///own/runtime';
const homeDir = '/selected/custom-state';
const selected = { cwd: '/secondary', data_dir: `${homeDir}/data/workspaces/secondary`, host, session_id: 'selected-session' };
const primary = { engine: 'go', cwd: '/primary', data_dir: `${homeDir}/data/workspaces/primary`, host, workspace_id: 'primary-id' };
const url = 'http://127.0.0.1:58627/sessions/selected-session#token=fixture-token';
const metadata = { server_version: '2.0.2', capabilities: {} };
function response(data) { return { ok: true, status: 200, json: async () => ({ code: 0, data }) }; }
function fixture({ target = selected, runtime = primary } = {}) {
 const requests = [];
 const fetchImpl = async (input, options) => {
  const endpoint = new URL(input).pathname;
  requests.push(endpoint);
  assert.equal(options.headers.Authorization, 'Bearer fixture-token');
  if (endpoint === '/api/v1/meta') return response(metadata);
  if (endpoint === '/api/v1/runtime') return response(runtime);
  assert.equal(endpoint, '/api/v1/sessions/selected-session/handoff');
  return response(target);
 };
 return { requests, fetchImpl };
}

test('startup validates the selected session when a shared adapter has another primary workspace', async () => {
 const f = fixture();
 const result = await fetchStartupMeta({ url, workspace: selected.cwd, dataDir: selected.data_dir, host, homeDir, fetchImpl: f.fetchImpl, attempts: 1 });
 assert.equal(result.ok, true, result.error);
 assert.equal(result.runtime.cwd, primary.cwd);
 assert.deepEqual(result.handoff, selected);
 assert.deepEqual(f.requests, ['/api/v1/meta', '/api/v1/runtime', '/api/v1/sessions/selected-session/handoff']);
});

test('custom MegaFloyd home accepts its own store and refuses sibling or legacy stores', async () => {
 for (const data_dir of [selected.data_dir, '/selected/custom-state-other/data/workspaces/secondary', '/selected/custom-state/.floyd/data', '/account/other/.megafloyd/data/workspaces/secondary']) {
  const runtime = { ...primary, cwd: selected.cwd, data_dir };
  const result = await fetchRuntime({ port: 1, workspace: selected.cwd, homeDir, attempts: 1, fetchImpl: async () => response(runtime) });
  assert.equal(result.ok, data_dir === selected.data_dir, data_dir);
  const f = fixture({ target: { ...selected, data_dir } });
  const startup = await fetchStartupMeta({ url, workspace: selected.cwd, homeDir, fetchImpl: f.fetchImpl, attempts: 1 });
  assert.equal(startup.ok, data_dir === selected.data_dir, data_dir);
 }
});

test('shared adapter startup still refuses substituted workspace store host and session identities', async () => {
 for (const mismatch of [{ cwd: '/other' }, { data_dir: '/other/store' }, { host: 'unix:///foreign' }, { session_id: 'other-session' }]) {
  const f = fixture({ target: { ...selected, ...mismatch } });
  const result = await fetchStartupMeta({ url, workspace: selected.cwd, dataDir: selected.data_dir, host, homeDir, fetchImpl: f.fetchImpl, attempts: 1 });
  assert.equal(result.ok, false, JSON.stringify(mismatch));
 }
 const f = fixture({ runtime: { ...primary, host: 'unix:///foreign' } });
 const result = await fetchStartupMeta({ url, workspace: selected.cwd, dataDir: selected.data_dir, host, homeDir, fetchImpl: f.fetchImpl, attempts: 1 });
 assert.equal(result.ok, false);
 assert.equal(f.requests.includes('/api/v1/sessions/selected-session/handoff'), false);
});
