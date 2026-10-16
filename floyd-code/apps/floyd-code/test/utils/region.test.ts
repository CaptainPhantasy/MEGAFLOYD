import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import {
  currentFloydProfile,
  currentFloydRegion,
  refreshFloydRegion,
  regionForBareLogin,
} from '#/utils/region';

const originalEnv = { ...process.env };

let home: string;

beforeEach(() => {
  home = mkdtempSync(join(tmpdir(), 'floyd-region-test-'));
  process.env['FLOYD_CODE_HOME'] = home;
  delete process.env['FLOYD_CODE_OAUTH_HOST'];
  delete process.env['FLOYD_OAUTH_HOST'];
  delete process.env['FLOYD_CODE_BASE_URL'];
  delete process.env['FLOYD_CODE_REGION_MARKER'];
  refreshFloydRegion();
});

afterEach(() => {
  process.env = { ...originalEnv };
  refreshFloydRegion();
  rmSync(home, { recursive: true, force: true });
});

describe('currentFloydRegion', () => {
  it('follows the install-channel marker before the first login', () => {
    writeFileSync(join(home, 'region'), 'global\n');
    expect(refreshFloydRegion()).toBe('global');
    expect(currentFloydRegion()).toBe('global');
  });

  it('ignores the marker when FLOYD_CODE_REGION_MARKER=off (embedded server)', () => {
    writeFileSync(join(home, 'region'), 'global\n');
    process.env['FLOYD_CODE_REGION_MARKER'] = 'off';
    expect(refreshFloydRegion()).toBe('mainland-cn');
  });

  it('leaves every endpoint of the resolved profile unconfigured', () => {
    expect(refreshFloydRegion()).toBe('mainland-cn');
    const profile = currentFloydProfile();
    expect(profile.oauthHost).toBe('');
    expect(profile.baseUrl).toBe('');
    expect(profile.telemetryEndpoint).toBe('');
  });

  it('still honors a persisted global login when the marker is opted out', () => {
    writeFileSync(join(home, 'region'), 'global\n');
    writeFileSync(
      join(home, 'config.toml'),
      [
        '[providers."managed:floyd-code"]',
        'type = "floyd"',
        '',
        '[providers."managed:floyd-code".oauth]',
        'storage = "file"',
        'key = "oauth/floyd-code-env-0123456789abcdef"',
        'oauthHost = "https://auth.floyd.ai"',
        '',
      ].join('\n'),
    );
    process.env['FLOYD_CODE_REGION_MARKER'] = 'off';
    expect(refreshFloydRegion()).toBe('global');
  });
});

describe('regionForBareLogin', () => {
  it('follows the resolved region for a fresh install (no persisted ref)', () => {
    expect(regionForBareLogin(undefined)).toBe('mainland-cn');
    writeFileSync(join(home, 'region'), 'global\n');
    refreshFloydRegion();
    expect(regionForBareLogin(undefined)).toBe('global');
  });

  it('re-pins mainland-cn for the default slot', () => {
    expect(regionForBareLogin({ key: 'oauth/floyd-code' })).toBe('mainland-cn');
  });

  it('keeps the configured environment for a scoped slot without a persisted host', () => {
    expect(regionForBareLogin({ key: 'oauth/floyd-code-env-0123456789abcdef' })).toBeUndefined();
  });

  it('keeps the persisted environment for a global login', () => {
    expect(
      regionForBareLogin({
        key: 'oauth/floyd-code-env-0123456789abcdef',
        oauthHost: 'https://auth.floyd.ai',
      }),
    ).toBeUndefined();
  });
});
