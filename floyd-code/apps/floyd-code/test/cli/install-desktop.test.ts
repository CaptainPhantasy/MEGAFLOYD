import { Command } from 'commander';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { FloydRegionProfile } from '@legacy-ai/floyd-code-oauth';

import { registerInstallDesktopCommand } from '#/cli/sub/install-desktop';

const OFFICIAL_PAGE_URL = 'https://app.example.test/install';
const NOT_CONFIGURED =
  'Floyd Code desktop app page is not configured (no region site base or install page URL available).';

const mocks = vi.hoisted(() => ({
  openUrl: vi.fn(),
  currentFloydProfile: vi.fn(),
  officialInstallUrl: vi.fn(),
}));

vi.mock('#/utils/open-url', async (importOriginal) => {
  const actual = await importOriginal<typeof import('#/utils/open-url')>();
  return { ...actual, openUrl: mocks.openUrl };
});

vi.mock('#/utils/region', async (importOriginal) => {
  const actual = await importOriginal<typeof import('#/utils/region')>();
  return { ...actual, currentFloydProfile: mocks.currentFloydProfile };
});

vi.mock('#/constant/app', async (importOriginal) => {
  const actual = await importOriginal<typeof import('#/constant/app')>();
  return { ...actual, floydCodeOfficialInstallUrl: mocks.officialInstallUrl };
});

function profile(siteBase: string): FloydRegionProfile {
  return { siteBase } as unknown as FloydRegionProfile;
}

async function run(command: string): Promise<void> {
  const program = new Command('floyd');
  registerInstallDesktopCommand(program);
  await program.parseAsync(['node', 'floyd', command]);
}

describe('floyd install-desktop', () => {
  beforeEach(() => {
    mocks.currentFloydProfile.mockReturnValue(profile('https://example.com'));
    mocks.officialInstallUrl.mockReturnValue(OFFICIAL_PAGE_URL);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.clearAllMocks();
  });

  it('prints the region-derived desktop app page URL and opens it in the browser', async () => {
    const write = vi.spyOn(process.stdout, 'write').mockImplementation(() => true);

    await run('install-desktop');

    expect(write).toHaveBeenCalledWith('https://example.com/code\n');
    expect(mocks.openUrl).toHaveBeenCalledWith('https://example.com/code');
  });

  it('keeps install-app working as a hidden alias', async () => {
    const write = vi.spyOn(process.stdout, 'write').mockImplementation(() => true);

    await run('install-app');

    expect(write).toHaveBeenCalledWith('https://example.com/code\n');
    expect(mocks.openUrl).toHaveBeenCalledWith('https://example.com/code');
  });

  it('falls back to the official install page when the region has no site base', async () => {
    mocks.currentFloydProfile.mockReturnValue(profile(''));
    const write = vi.spyOn(process.stdout, 'write').mockImplementation(() => true);

    await run('install-desktop');

    expect(write).toHaveBeenCalledWith(`${OFFICIAL_PAGE_URL}\n`);
    expect(mocks.openUrl).toHaveBeenCalledWith(OFFICIAL_PAGE_URL);
  });

  it('prints a clear not-configured message and opens nothing when no page URL is available', async () => {
    mocks.currentFloydProfile.mockReturnValue(profile(''));
    mocks.officialInstallUrl.mockReturnValue('');
    const write = vi.spyOn(process.stdout, 'write').mockImplementation(() => true);

    await run('install-desktop');

    expect(write).toHaveBeenCalledWith(`${NOT_CONFIGURED}\n`);
    expect(mocks.openUrl).not.toHaveBeenCalled();
  });
});
