import { gzipSync } from 'node:zlib';

import { describe, expect, it, vi } from 'vitest';

const PAYLOAD = '# Protocol Kernel v1.0\n\nInjected by the Go chassis.\n';

function encode(text: string): string {
  return `gz1:${gzipSync(Buffer.from(text, 'utf8')).toString('base64')}`;
}

async function loadModule() {
  vi.resetModules();
  return import('#/app/agentProfileCatalog/profile-shared');
}

describe('inheritedRoleAdditional', () => {
  it('decodes the payload the Go chassis sends', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const mod = await loadModule();
    expect(mod.inheritedRoleAdditional()).toBe(PAYLOAD.trim());
    vi.unstubAllEnvs();
  });

  it('ignores an absent, malformed, or unknown-version value', async () => {
    for (const value of ['', 'garbage', 'gz1:not-base64!!', 'v2:whatever']) {
      vi.stubEnv('FLOYD_ROLE_ADDITIONAL', value);
      const mod = await loadModule();
      expect(mod.inheritedRoleAdditional()).toBe('');
    }
    vi.unstubAllEnvs();
  });

  it('applies the inherited layers to the rendered system prompt', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const mod = await loadModule();
    const rendered = mod.renderSystemPromptResult('', { cwd: '/tmp' }, { skillActive: false });
    expect(rendered.text).toContain('Protocol Kernel v1.0');
    expect(rendered.text).toContain('Injected by the Go chassis.');
    vi.unstubAllEnvs();
  });

  it('keeps an explicit role when one is supplied', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const mod = await loadModule();
    const rendered = mod.renderSystemPromptResult('explicit role', { cwd: '/tmp' }, { skillActive: false });
    expect(rendered.text).toContain('explicit role');
    expect(rendered.text).not.toContain('Protocol Kernel v1.0');
    vi.unstubAllEnvs();
  });
});
