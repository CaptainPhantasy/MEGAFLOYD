import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';

import { afterEach, describe, expect, it, vi } from 'vitest';

import { DEFAULT_FLOYD_CODE_OAUTH_HOST } from '#/constants';
import { FLOYD_CODE_OAUTH_KEY } from '#/managed-floyd-code';
import { DEFAULT_FLOYD_CODE_BASE_URL } from '#/managed-usage';
import {
  FLOYD_REGION_MARKER_FILENAME,
  FLOYD_REGION_PROFILES,
  floydCdnContentUrl,
  floydRegionLoginHosts,
  floydRegionProfile,
  floydRegionSchema,
  resolveFloydRegion,
} from '#/region';

import { createTempWorkDir, type TempDirHandle } from './helpers';

const EMPTY_PROFILE = {
  oauthHost: '',
  baseUrl: '',
  cdnBase: '',
  siteBase: '',
  telemetryEndpoint: '',
};

describe('FLOYD_REGION_PROFILES', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it('leaves an unconfigured profile empty — no deployment is baked in', () => {
    expect(floydRegionProfile('mainland-cn', {})).toEqual(EMPTY_PROFILE);
    expect(floydRegionProfile('global', {})).toEqual(EMPTY_PROFILE);
  });

  it('keeps the mainland-cn profile aligned with the shared defaults', () => {
    vi.stubEnv('FLOYD_CODE_OAUTH_HOST', '');
    vi.stubEnv('FLOYD_OAUTH_HOST', '');
    vi.stubEnv('FLOYD_CODE_BASE_URL', '');
    expect(FLOYD_REGION_PROFILES['mainland-cn'].oauthHost).toBe(DEFAULT_FLOYD_CODE_OAUTH_HOST);
    expect(FLOYD_REGION_PROFILES['mainland-cn'].baseUrl).toBe(DEFAULT_FLOYD_CODE_BASE_URL);
  });

  it('reads every endpoint from that region own env overrides', () => {
    expect(
      floydRegionProfile('global', {
        FLOYD_CODE_GLOBAL_OAUTH_HOST: 'https://auth.example.test',
        FLOYD_CODE_GLOBAL_BASE_URL: 'https://api.example.test/coding/v1',
        FLOYD_CODE_GLOBAL_CDN_BASE: 'https://cdn.example.test/floyd-code',
        FLOYD_CODE_GLOBAL_SITE_BASE: 'https://www.example.test',
        FLOYD_CODE_GLOBAL_TELEMETRY_ENDPOINT: 'https://telemetry.example.test/v1/event',
      }),
    ).toEqual({
      oauthHost: 'https://auth.example.test',
      baseUrl: 'https://api.example.test/coding/v1',
      cdnBase: 'https://cdn.example.test/floyd-code',
      siteBase: 'https://www.example.test',
      telemetryEndpoint: 'https://telemetry.example.test/v1/event',
    });
  });

  it('reads the shared default slot from FLOYD_CODE_* and keeps regions separate', () => {
    const env = {
      FLOYD_CODE_OAUTH_HOST: 'https://auth.example.test',
      FLOYD_OAUTH_HOST: 'https://auth.alias.example.test',
      FLOYD_CODE_BASE_URL: 'https://api.example.test/coding/v1',
    };
    expect(floydRegionProfile('mainland-cn', env)).toMatchObject({
      oauthHost: 'https://auth.example.test',
      baseUrl: 'https://api.example.test/coding/v1',
    });
    expect(
      floydRegionProfile('mainland-cn', { FLOYD_OAUTH_HOST: 'https://auth.alias.example.test' })
        .oauthHost,
    ).toBe('https://auth.alias.example.test');
    expect(floydRegionProfile('global', env)).toEqual(EMPTY_PROFILE);
  });

  it('exposes the profiles through the record, re-read from env', () => {
    vi.stubEnv('FLOYD_CODE_OAUTH_HOST', '');
    vi.stubEnv('FLOYD_OAUTH_HOST', '');
    vi.stubEnv('FLOYD_CODE_GLOBAL_OAUTH_HOST', 'https://auth.example.test');
    expect(FLOYD_REGION_PROFILES['global'].oauthHost).toBe('https://auth.example.test');
    expect(FLOYD_REGION_PROFILES['mainland-cn'].oauthHost).toBe('');
  });
});

describe('resolveFloydRegion', () => {
  let workDir: TempDirHandle | undefined;

  afterEach(async () => {
    await workDir?.cleanup();
    workDir = undefined;
  });

  async function markerDir(contents?: string): Promise<string> {
    workDir = await createTempWorkDir();
    if (contents !== undefined) {
      await writeFile(join(workDir.path, FLOYD_REGION_MARKER_FILENAME), contents, 'utf-8');
    }
    return workDir.path;
  }

  it('defaults to mainland-cn when nothing points anywhere', async () => {
    expect(resolveFloydRegion({ env: {}, homeDir: await markerDir() })).toBe('mainland-cn');
  });

  it('resolves a known env oauth host, FLOYD_CODE_OAUTH_HOST first', () => {
    expect(resolveFloydRegion({ env: { FLOYD_CODE_OAUTH_HOST: 'https://auth.floyd.ai' } })).toBe(
      'global',
    );
    expect(resolveFloydRegion({ env: { FLOYD_OAUTH_HOST: 'https://auth.floyd.ai' } })).toBe(
      'global',
    );
    expect(
      resolveFloydRegion({
        env: {
          FLOYD_CODE_OAUTH_HOST: 'https://auth.floyd.com',
          FLOYD_OAUTH_HOST: 'https://auth.floyd.ai',
        },
      }),
    ).toBe('mainland-cn');
  });

  it('pins the global slot from its own env overrides, default slot first', () => {
    expect(
      resolveFloydRegion({ env: { FLOYD_CODE_GLOBAL_OAUTH_HOST: 'https://auth.example.test' } }),
    ).toBe('global');
    expect(
      resolveFloydRegion({ env: { FLOYD_GLOBAL_OAUTH_HOST: 'https://auth.example.test' } }),
    ).toBe('global');
    expect(
      resolveFloydRegion({
        env: {
          FLOYD_CODE_GLOBAL_OAUTH_HOST: 'https://auth.example.test',
          FLOYD_OAUTH_HOST: 'https://auth.internal.example.com',
        },
      }),
    ).toBe('mainland-cn');
  });

  it('treats an unknown env host as a custom environment and falls back to cn', async () => {
    // ...even when the persisted login or marker says otherwise: the custom
    // env overrides every endpoint anyway.
    expect(
      resolveFloydRegion({
        env: { FLOYD_CODE_OAUTH_HOST: 'https://auth.internal.example.com' },
        configuredOAuthHost: 'https://auth.floyd.ai',
        homeDir: await markerDir('global\n'),
      }),
    ).toBe('mainland-cn');
  });

  it('resolves the persisted login host, tolerating trailing slashes', () => {
    expect(resolveFloydRegion({ env: {}, configuredOAuthHost: 'https://auth.floyd.ai/' })).toBe(
      'global',
    );
    expect(resolveFloydRegion({ env: {}, configuredOAuthHost: 'https://auth.floyd.com' })).toBe('mainland-cn');
  });

  it('ignores an unrecognized persisted host and continues down the chain', async () => {
    expect(
      resolveFloydRegion({
        env: {},
        configuredOAuthHost: 'https://auth.legacy.example.com',
        homeDir: await markerDir('global'),
      }),
    ).toBe('global');
  });

  it('reads the install-channel marker when nothing else decides', async () => {
    expect(resolveFloydRegion({ env: {}, homeDir: await markerDir('global\n') })).toBe('global');
    expect(resolveFloydRegion({ env: {}, homeDir: await markerDir('  mainland-cn  ') })).toBe('mainland-cn');
  });

  it('ignores a malformed or missing marker', async () => {
    expect(resolveFloydRegion({ env: {}, homeDir: await markerDir('apac') })).toBe('mainland-cn');
    expect(resolveFloydRegion({ env: {}, homeDir: await markerDir('') })).toBe('mainland-cn');
  });

  it('skips the marker entirely when readMarker is false', async () => {
    expect(
      resolveFloydRegion({ env: {}, homeDir: await markerDir('global'), readMarker: false }),
    ).toBe('mainland-cn');
  });

  it('honors FLOYD_CODE_HOME when homeDir is not passed explicitly', async () => {
    const dir = await markerDir('global');
    expect(resolveFloydRegion({ env: { FLOYD_CODE_HOME: dir } })).toBe('global');
  });

  it('env beats persisted login beats marker', async () => {
    const dir = await markerDir('global');
    expect(
      resolveFloydRegion({
        env: { FLOYD_CODE_OAUTH_HOST: 'https://auth.floyd.com' },
        configuredOAuthHost: 'https://auth.floyd.ai',
        homeDir: dir,
      }),
    ).toBe('mainland-cn');
    expect(
      resolveFloydRegion({
        env: {},
        configuredOAuthHost: 'https://auth.floyd.ai',
        homeDir: dir,
      }),
    ).toBe('global');
  });

  it('treats the persisted default-slot key as explicit mainland-cn, beating the marker', async () => {
    const dir = await markerDir('global');
    expect(
      resolveFloydRegion({ env: {}, configuredOAuthKey: FLOYD_CODE_OAUTH_KEY, homeDir: dir }),
    ).toBe('mainland-cn');
  });

  it('still follows the marker when no key or host is persisted', async () => {
    expect(resolveFloydRegion({ env: {}, homeDir: await markerDir('global') })).toBe('global');
  });

  it('lets an unknown scoped key fall through to the marker', async () => {
    const dir = await markerDir('global');
    expect(
      resolveFloydRegion({
        env: {},
        configuredOAuthKey: 'oauth/floyd-code-env-0123456789abcdef',
        homeDir: dir,
      }),
    ).toBe('global');
  });

  it('resolves a recognized persisted host before consulting the key', () => {
    expect(
      resolveFloydRegion({
        env: {},
        configuredOAuthHost: 'https://auth.floyd.ai',
        configuredOAuthKey: FLOYD_CODE_OAUTH_KEY,
      }),
    ).toBe('global');
  });
});

describe('floydRegionLoginHosts', () => {
  it('returns both configured hosts for an explicit region choice', () => {
    expect(
      floydRegionLoginHosts('mainland-cn', {
        FLOYD_CODE_OAUTH_HOST: 'https://auth.example.test',
        FLOYD_CODE_BASE_URL: 'https://api.example.test/coding/v1',
      }),
    ).toEqual({
      oauthHost: 'https://auth.example.test',
      baseUrl: 'https://api.example.test/coding/v1',
    });
    expect(
      floydRegionLoginHosts('global', {
        FLOYD_CODE_GLOBAL_OAUTH_HOST: 'https://auth.example.test',
        FLOYD_CODE_GLOBAL_BASE_URL: 'https://api.example.test/coding/v1',
      }),
    ).toEqual({
      oauthHost: 'https://auth.example.test',
      baseUrl: 'https://api.example.test/coding/v1',
    });
  });

  it('returns undefined for an unconfigured profile instead of an empty host', () => {
    expect(floydRegionLoginHosts('mainland-cn', {})).toBeUndefined();
    expect(floydRegionLoginHosts('global', {})).toBeUndefined();
    // Half-configured (host without a base URL) has no login hosts to offer.
    expect(
      floydRegionLoginHosts('global', {
        FLOYD_CODE_GLOBAL_OAUTH_HOST: 'https://auth.example.test',
      }),
    ).toBeUndefined();
  });

  it('yields to env overrides', () => {
    const globalHosts = {
      FLOYD_CODE_GLOBAL_OAUTH_HOST: 'https://auth.example.test',
      FLOYD_CODE_GLOBAL_BASE_URL: 'https://api.example.test/coding/v1',
    };
    expect(
      floydRegionLoginHosts('global', {
        ...globalHosts,
        FLOYD_CODE_OAUTH_HOST: 'https://auth.x.com',
      }),
    ).toBe(undefined);
    expect(
      floydRegionLoginHosts('global', { ...globalHosts, FLOYD_OAUTH_HOST: 'https://auth.x.com' }),
    ).toBe(undefined);
    expect(
      floydRegionLoginHosts('global', {
        ...globalHosts,
        FLOYD_CODE_BASE_URL: 'https://api.x.com/coding/v1',
      }),
    ).toBe(undefined);
    expect(
      floydRegionLoginHosts('mainland-cn', {
        FLOYD_CODE_OAUTH_HOST: 'https://auth.example.test',
        FLOYD_CODE_BASE_URL: 'https://api.example.test/coding/v1',
        FLOYD_CODE_GLOBAL_OAUTH_HOST: 'https://auth.x.com',
      }),
    ).toBe(undefined);
  });
});

describe('floydRegionSchema', () => {
  it('parses valid regions and rejects others', () => {
    expect(floydRegionSchema.parse('mainland-cn')).toBe('mainland-cn');
    expect(floydRegionSchema.parse('global')).toBe('global');
    expect(floydRegionSchema.safeParse('apac').success).toBe(false);
  });
});

describe('floydCdnContentUrl', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it('returns empty until the content CDN is configured', () => {
    vi.stubEnv('FLOYD_CODE_CONTENT_CDN_BASE', '');
    expect(floydCdnContentUrl('floyd-computer-use/latest/floyd-cu-plugin.zip')).toBe('');

    vi.stubEnv('FLOYD_CODE_CONTENT_CDN_BASE', 'https://cdn.example.test/');
    expect(floydCdnContentUrl('/floyd-computer-use/latest/floyd-cu-plugin.zip')).toBe(
      'https://cdn.example.test/floyd-computer-use/latest/floyd-cu-plugin.zip',
    );
  });
});
