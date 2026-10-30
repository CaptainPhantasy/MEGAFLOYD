import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fetchRuntime, fetchStartupMeta } from '../src/meta.mjs';

function fixture() {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'megafloyd-desktop-binding-')));
  const homeDir = path.join(root, 'active-home');
  const otherHome = path.join(root, 'other-home');
  const workspace = path.join(root, 'project');
  const dataDir = path.join(workspace, '.megafloyd');
  for (const directory of [homeDir, otherHome, workspace, dataDir]) fs.mkdirSync(directory, { recursive: true, mode: 0o700 });
  const host = 'unix:///binding-fixture/runtime';
  const selected = { cwd: workspace, data_dir: dataDir, host, session_id: 'selected-session' };
  const runtime = { ...selected, engine: 'go', workspace_id: 'selected-workspace' };
  const url = 'http://127.0.0.1:58627/sessions/selected-session';
  const reply = data => ({ ok: true, status: 200, json: async () => ({ code: 0, data }) });
  const writeRegistry = (record, home = homeDir) => {
    fs.mkdirSync(path.join(home, 'state'), { recursive: true, mode: 0o700 });
    fs.writeFileSync(path.join(home, 'state/workspace-data.json'), typeof record === 'string' ? record : JSON.stringify(record), { mode: 0o600 });
  };
  const fetchImpl = async input => {
    const endpoint = new URL(input).pathname;
    if (endpoint === '/api/v1/meta') return reply({ server_version: '2.0.2', capabilities: {} });
    if (endpoint === '/api/v1/runtime') return reply(runtime);
    assert.equal(endpoint, '/api/v1/sessions/selected-session/handoff');
    return reply(selected);
  };
  const probe = async (extra = {}) => [
    await fetchRuntime({ port: 58627, homeDir, workspace, host, attempts: 1, fetchImpl, ...extra }),
    await fetchStartupMeta({ url, homeDir, workspace, host, attempts: 1, fetchImpl, ...extra }),
  ];
  return { homeDir, otherHome, workspace, dataDir, host, runtime, selected, writeRegistry, probe };
}

test('desktop accepts the exact workspace store selected in its active private registry', async () => {
  const f = fixture();
  f.writeRegistry({ version: 1, workspaces: { [fs.realpathSync(f.workspace)]: f.dataDir } });
  for (const result of await f.probe()) assert.equal(result.ok, true, result.error);
  const alias = path.join(f.homeDir, 'workspace-alias');
  fs.symlinkSync(f.workspace, alias, 'dir');
  for (const result of await f.probe({ workspace: alias })) assert.equal(result.ok, true, result.error);
});

test('desktop refuses missing malformed unrelated and other-home workspace bindings', async () => {
  const records = [null, 'malformed', { version: 2, workspaces: {} }, { version: 1, workspaces: [] },
    { version: 1, workspaces: { '/unrelated/workspace': '/unrelated/store' } },
    { version: 1, workspaces: { binding: 'relative/store' } },
    { version: 1, workspaces: { binding: '/unrelated/store' } }];
  for (const record of records) {
    const f = fixture();
    if (record?.workspaces?.binding) record.workspaces = { [f.workspace]: record.workspaces.binding };
    if (record !== null) f.writeRegistry(record);
    f.writeRegistry({ version: 1, workspaces: { [f.workspace]: f.dataDir } }, f.otherHome);
    for (const result of await f.probe()) assert.equal(result.ok, false, JSON.stringify(record));
  }
});

test('registry binding keeps explicit store priority and selected host session workspace checks', async () => {
  const f = fixture();
  f.writeRegistry({ version: 1, workspaces: { [f.workspace]: f.dataDir } });
  for (const result of await f.probe({ dataDir: '/different/explicit/store' })) assert.equal(result.ok, false);
  for (const mismatch of [{ cwd: '/unrelated/workspace' }, { data_dir: '/unrelated/store' }, { host: 'unix:///other' }, { session_id: 'other-session' }]) {
    const previous = { ...f.selected };
    Object.assign(f.selected, mismatch);
    const results = await f.probe();
    assert.equal(results[1].ok, false, JSON.stringify(mismatch));
    Object.assign(f.selected, previous);
  }
});

test('registered selected workspace can start through a shared adapter with another primary workspace', async () => {
  const f = fixture();
  f.writeRegistry({ version: 1, workspaces: { [f.workspace]: f.dataDir } });
  f.runtime.cwd = '/different/primary';
  f.runtime.data_dir = '/different/primary/store';
  const results = await f.probe();
  assert.equal(results[0].ok, false);
  assert.equal(results[1].ok, true, results[1].error);
});
