import { mkdtemp, readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { createFloydHarness } from '@legacy-ai/floyd-code-sdk';

import { smokeIdentityFromEnv } from './runtime-smoke-helpers';

async function main(): Promise<void> {
  const homeDir = await mkdtemp(join(tmpdir(), 'floyd-harness-config-home-'));
  const harness = createFloydHarness({ homeDir, identity: smokeIdentityFromEnv() });

  const initial = await harness.getConfig();
  if (Object.keys(initial.providers).length > 0) {
    throw new Error('expected empty providers for a fresh config home');
  }

  await harness.setConfig({
    defaultModel: 'floyd-code/floyd-for-coding',
    thinking: { enabled: true },
    defaultPermissionMode: 'manual',
    defaultPlanMode: false,
    providers: {
      'managed:floyd-code': {
        type: 'floyd',
        baseUrl: 'https://api.floyd.com/coding/v1',
        apiKey: '',
        oauth: { storage: 'file', key: 'oauth/floyd-code' },
      },
    },
    models: {
      'floyd-code/floyd-for-coding': {
        provider: 'managed:floyd-code',
        model: 'floyd-for-coding',
        maxContextSize: 262144,
        capabilities: ['image_in', 'thinking', 'video_in'],
        displayName: 'Floyd for Coding',
      },
    },
    loopControl: {
      maxRetriesPerStep: 3,
      maxRalphIterations: 0,
      reservedContextSize: 50000,
      compactionTriggerRatio: 0.85,
    },
    services: {
      legacySearch: {
        baseUrl: 'https://api.floyd.com/coding/v1/search',
        apiKey: '',
        oauth: { storage: 'file', key: 'oauth/floyd-code' },
      },
      legacyFetch: {
        baseUrl: 'https://api.floyd.com/coding/v1/fetch',
        apiKey: '',
        oauth: { storage: 'file', key: 'oauth/floyd-code' },
      },
    },
  });

  const configPath = join(homeDir, 'config.toml');
  const text = await readFile(configPath, 'utf-8');
  for (const expected of [
    'default_model = "floyd-code/floyd-for-coding"',
    'default_permission_mode = "manual"',
    '[providers."managed:floyd-code"]',
    '[providers."managed:floyd-code".oauth]',
    '[models."floyd-code/floyd-for-coding"]',
    '[services.legacy_search]',
  ]) {
    if (!text.includes(expected)) {
      throw new Error(`missing ${expected} in written config`);
    }
  }

  const reloaded = await harness.getConfig({ reload: true });
  if (reloaded.defaultModel !== 'floyd-code/floyd-for-coding') {
    throw new Error('reloaded config did not preserve defaultModel');
  }
  if (reloaded.providers['managed:floyd-code']?.oauth?.key !== 'oauth/floyd-code') {
    throw new Error('reloaded config did not preserve provider oauth');
  }

  process.stdout.write(`config: ${configPath}\n`);
  process.stdout.write('ok\n');
}

try {
  await main();
} catch (error: unknown) {
  console.error(error);
  process.exitCode = 1;
}
