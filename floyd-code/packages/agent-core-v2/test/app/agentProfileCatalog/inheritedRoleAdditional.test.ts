import { gzipSync } from 'node:zlib';

import { describe, expect, it, vi } from 'vitest';

import { DEFAULT_AGENT_PROFILE_NAME } from '#/app/agentProfileCatalog/agentProfileCatalog';
import { TASK_AGENT_ROLE_PREFIX } from '#/app/agentProfileCatalog/profile-shared';

const SENTINEL = 'INHERITED-STANDARD-SENTINEL-6f4b1c9e';

const PAYLOAD =
  '# Protocol Kernel v1.0\n\n' +
  'Injected by the Go chassis.\n\n' +
  `${SENTINEL}\n\n` +
  'Every agent profile receives this standard.';

function encode(text: string): string {
  return `gz1:${gzipSync(Buffer.from(text, 'utf8')).toString('base64')}`;
}

async function loadModule() {
  vi.resetModules();
  return import('#/app/agentProfileCatalog/profile-shared');
}

async function loadRegisteredProfiles() {
  vi.resetModules();
  await import('#/session/agentLifecycle/profile/profiles');
  await import('#/features/plan/profile/plan');
  const [{ getAgentProfileContributions }, { TOWER_WORKER_PROFILE_DEF }] = await Promise.all([
    import('#/app/agentProfileCatalog/contribution'),
    import('#/features/tower/workerProfile'),
  ]);
  return [...getAgentProfileContributions(), TOWER_WORKER_PROFILE_DEF];
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
});

describe('inherited standard composition', () => {
  it('keeps the default agent role slot exactly the inherited standard', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const mod = await loadModule();
    const rendered = mod.renderSystemPromptResult('', { cwd: '/tmp' }, { skillActive: false });

    const parts = rendered.text.split(mod.inheritedRoleAdditional());
    expect(parts).toHaveLength(2);
    expect(parts[1]?.startsWith('\n\n# Communicating with the user')).toBe(true);
    vi.unstubAllEnvs();
  });

  it('places the inherited standard ahead of an explicit role text', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const mod = await loadModule();
    const rendered = mod.renderSystemPromptResult(
      'explicit role',
      { cwd: '/tmp' },
      { skillActive: false },
    );

    expect(rendered.text).toContain(`${PAYLOAD}\n\nexplicit role`);
    expect(rendered.text.split(SENTINEL)).toHaveLength(2);
    vi.unstubAllEnvs();
  });

  it('keeps an explicit role text untouched when no standard is inherited', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', '');
    const mod = await loadModule();
    const rendered = mod.renderSystemPromptResult(
      'explicit role',
      { cwd: '/tmp' },
      { skillActive: false },
    );

    expect(rendered.text).toContain('\n\nexplicit role\n\n# Communicating with the user');
    vi.unstubAllEnvs();
  });
});

describe('agents_md de-duplication', () => {
  it('suppresses project instructions that repeat the inherited standard', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const mod = await loadModule();
    const vars = mod.systemPromptVars({ agentsMd: `\n\n${PAYLOAD}\n` }, { skillActive: false });

    expect(vars['agents_md']).toBe('');
    vi.unstubAllEnvs();
  });

  it('keeps project instructions the standard does not contain', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const mod = await loadModule();
    const vars = mod.systemPromptVars(
      { agentsMd: '# Repository guide\n\nUse pnpm.\n' },
      { skillActive: false },
    );

    expect(vars['agents_md']).toBe('# Repository guide\n\nUse pnpm.\n');
    vi.unstubAllEnvs();
  });

  it('renders one copy of the standard when the project instructions repeat it', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const mod = await loadModule();
    const rendered = mod.renderSystemPromptResult(
      '',
      { agentsMd: `${PAYLOAD}\n`, cwd: '/tmp' },
      { skillActive: false },
    );

    expect(rendered.text.split(SENTINEL)).toHaveLength(2);
    vi.unstubAllEnvs();
  });

  it('renders unrelated project instructions in full', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const mod = await loadModule();
    const rendered = mod.renderSystemPromptResult(
      '',
      { agentsMd: '# Repository guide\n\nUse pnpm.\n', cwd: '/tmp' },
      { skillActive: false },
    );

    expect(rendered.text).toContain('```````\n# Repository guide\n\nUse pnpm.\n\n```````');
    expect(rendered.text.split(SENTINEL)).toHaveLength(2);
    vi.unstubAllEnvs();
  });
});

describe('registered agent profiles', () => {
  it('gives every profile exactly one copy of the inherited standard', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const profiles = await loadRegisteredProfiles();
    expect(profiles.length).toBeGreaterThan(0);

    for (const profile of profiles) {
      const text = profile.renderSystemPrompt({ cwd: '/tmp' }).text;
      expect(text.split(SENTINEL), `profile "${profile.name}" inherits the standard`).toHaveLength(
        2,
      );
    }
    vi.unstubAllEnvs();
  });

  it('keeps the default profile free of its own role text', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const profiles = await loadRegisteredProfiles();
    const agent = profiles.find((profile) => profile.name === DEFAULT_AGENT_PROFILE_NAME);
    expect(agent).toBeDefined();

    const text = agent!.renderSystemPrompt({ cwd: '/tmp' }).text;
    expect(text).not.toContain(TASK_AGENT_ROLE_PREFIX);
    vi.unstubAllEnvs();
  });

  it('keeps a profile own role text alongside the inherited standard', async () => {
    vi.stubEnv('FLOYD_ROLE_ADDITIONAL', encode(PAYLOAD));
    const profiles = await loadRegisteredProfiles();
    const coder = profiles.find((profile) => profile.name === 'coder');
    expect(coder).toBeDefined();

    const text = coder!.renderSystemPrompt({ cwd: '/tmp' }).text;
    expect(text).toContain(SENTINEL);
    expect(text).toContain(TASK_AGENT_ROLE_PREFIX);
    expect(text.split(SENTINEL)).toHaveLength(2);
    vi.unstubAllEnvs();
  });
});
