import { afterEach, describe, expect, it, vi } from 'vitest';

import { PlatformSelectorComponent } from '#/tui/components/dialogs/platform-selector';

const ANSI = /\u001B\[[0-9;]*m/g;
const ENTER = '\r';

const PLATFORM_ENV = [
  'FLOYD_CODE_OAUTH_HOST',
  'FLOYD_OAUTH_HOST',
  'FLOYD_CODE_BASE_URL',
  'FLOYD_CODE_GLOBAL_OAUTH_HOST',
  'FLOYD_GLOBAL_OAUTH_HOST',
  'FLOYD_CODE_GLOBAL_BASE_URL',
  'FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_BASE_URL',
  'FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_CONSOLE_URL',
  'FLOYD_CODE_OPEN_PLATFORM_LEGACY_AI_BASE_URL',
  'FLOYD_CODE_OPEN_PLATFORM_LEGACY_AI_CONSOLE_URL',
];

function blankPlatformEnv(): void {
  for (const name of PLATFORM_ENV) vi.stubEnv(name, '');
}

function text(component: PlatformSelectorComponent, width = 120): string {
  return component.render(width).map((line) => line.replaceAll(ANSI, '')).join('\n');
}

afterEach(() => {
  vi.unstubAllEnvs();
});

describe('PlatformSelectorComponent', () => {
  it('offers no vendor entry when no platform is configured', () => {
    blankPlatformEnv();
    const onSelect = vi.fn();

    const selector = new PlatformSelectorComponent({ onSelect, onCancel: vi.fn() });
    const out = text(selector);

    expect(out).toContain(' Select a platform');
    expect(out).toContain(' Enter close · Esc cancel');
    expect(out).toContain(' Add your own provider with /provider, or declare one under [providers.*] in config.toml.');
    expect(out).toContain('  ❯ No login platform configured');
    expect(out).not.toMatch(/floyd\.(com|ai|cn)/);
    expect(out).not.toContain('legacy.cn');

    selector.handleInput(ENTER);

    expect(onSelect).not.toHaveBeenCalled();
  });

  it('closes the dialog instead of selecting when nothing is configured', () => {
    blankPlatformEnv();
    const onCancel = vi.fn();

    const selector = new PlatformSelectorComponent({ onSelect: vi.fn(), onCancel });
    selector.handleInput(ENTER);

    expect(onCancel).toHaveBeenCalledOnce();
  });

  it('offers a configured region labelled with its authorization host', () => {
    blankPlatformEnv();
    vi.stubEnv('FLOYD_CODE_OAUTH_HOST', 'https://auth.example.test');
    vi.stubEnv('FLOYD_CODE_BASE_URL', 'https://api.example.test/coding/v1');
    const onSelect = vi.fn();

    const selector = new PlatformSelectorComponent({ onSelect, onCancel: vi.fn() });
    const out = text(selector);

    expect(out).toContain('  ❯ Floyd Code (auth.example.test)');
    expect(out).not.toContain('No login platform configured');
    expect(out).not.toMatch(/floyd\.(com|ai|cn)/);

    selector.handleInput(ENTER);

    expect(onSelect).toHaveBeenCalledWith('floyd-code');
  });

  it('offers the global region only when that region is configured', () => {
    blankPlatformEnv();
    vi.stubEnv('FLOYD_CODE_GLOBAL_OAUTH_HOST', 'https://auth-global.example.test');
    vi.stubEnv('FLOYD_CODE_GLOBAL_BASE_URL', 'https://api-global.example.test/coding/v1');
    const onSelect = vi.fn();

    const selector = new PlatformSelectorComponent({ onSelect, onCancel: vi.fn() });
    const out = text(selector);

    expect(out).toContain('  ❯ Floyd Code (auth-global.example.test)');
    expect(out).not.toContain('Floyd Code (auth.example.test)');

    selector.handleInput(ENTER);

    expect(onSelect).toHaveBeenCalledWith('floyd-code-global');
  });

  it('offers both configured regions once each', () => {
    blankPlatformEnv();
    vi.stubEnv('FLOYD_CODE_OAUTH_HOST', 'https://auth-a.example.test');
    vi.stubEnv('FLOYD_CODE_BASE_URL', 'https://api-a.example.test/coding/v1');
    vi.stubEnv('FLOYD_CODE_GLOBAL_OAUTH_HOST', 'https://auth-b.example.test');
    vi.stubEnv('FLOYD_CODE_GLOBAL_BASE_URL', 'https://api-b.example.test/coding/v1');

    const selector = new PlatformSelectorComponent({ onSelect: vi.fn(), onCancel: vi.fn() });
    const out = text(selector);

    expect(out).toContain('  ❯ Floyd Code (auth-a.example.test)');
    expect(out).toContain('Floyd Code (auth-b.example.test)');
  });

  it('offers a region whose authorization host is configured without an API base URL', () => {
    blankPlatformEnv();
    vi.stubEnv('FLOYD_CODE_OAUTH_HOST', 'https://auth.example.test');
    const onSelect = vi.fn();

    const selector = new PlatformSelectorComponent({ onSelect, onCancel: vi.fn() });

    expect(text(selector)).toContain('Floyd Code (auth.example.test)');

    selector.handleInput(ENTER);

    expect(onSelect).toHaveBeenCalledWith('floyd-code');
  });

  it('steers to the provider path when only an API base URL is configured', () => {
    blankPlatformEnv();
    vi.stubEnv('FLOYD_CODE_BASE_URL', 'https://api.example.test/coding/v1');

    const selector = new PlatformSelectorComponent({ onSelect: vi.fn(), onCancel: vi.fn() });

    expect(text(selector)).toContain('No login platform configured');
  });

  it('offers an Open Platform only when its base URL is configured', () => {
    blankPlatformEnv();

    const unconfigured = new PlatformSelectorComponent({ onSelect: vi.fn(), onCancel: vi.fn() });
    expect(text(unconfigured)).not.toContain('Floyd Platform');

    vi.stubEnv('FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_BASE_URL', 'https://api.example.test/v1');
    const onSelect = vi.fn();
    const configured = new PlatformSelectorComponent({ onSelect, onCancel: vi.fn() });
    const out = text(configured);

    expect(out).toContain('  ❯ Floyd Platform (API key · mainland CN)');
    expect(out).toContain('https://api.example.test/v1');
    expect(out).not.toContain('Floyd Platform (API key · global)');

    configured.handleInput(ENTER);

    expect(onSelect).toHaveBeenCalledWith('legacy-cn');
  });
});
