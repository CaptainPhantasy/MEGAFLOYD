import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { FloydRegionProfile } from '@legacy-ai/floyd-code-oauth';

import { handleDesktopCommand } from '#/tui/commands/desktop';
import type { SlashCommandHost } from '#/tui/commands/dispatch';

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

describe('handleDesktopCommand', () => {
  beforeEach(() => {
    mocks.currentFloydProfile.mockReturnValue(profile('https://example.com'));
    mocks.officialInstallUrl.mockReturnValue(OFFICIAL_PAGE_URL);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.clearAllMocks();
  });

  it('shows the region-derived desktop app page URL and opens it in the browser', async () => {
    const host = { showStatus: vi.fn() } as unknown as SlashCommandHost;

    await handleDesktopCommand(host);

    expect(host.showStatus).toHaveBeenCalledWith(expect.stringContaining('https://example.com/code'));
    expect(mocks.openUrl).toHaveBeenCalledWith('https://example.com/code');
  });

  it('falls back to the official install page when the region has no site base', async () => {
    mocks.currentFloydProfile.mockReturnValue(profile(''));
    const host = { showStatus: vi.fn() } as unknown as SlashCommandHost;

    await handleDesktopCommand(host);

    expect(host.showStatus).toHaveBeenCalledWith(expect.stringContaining(OFFICIAL_PAGE_URL));
    expect(mocks.openUrl).toHaveBeenCalledWith(OFFICIAL_PAGE_URL);
  });

  it('shows a clear not-configured message and opens nothing when no page URL is available', async () => {
    mocks.currentFloydProfile.mockReturnValue(profile(''));
    mocks.officialInstallUrl.mockReturnValue('');
    const host = { showStatus: vi.fn() } as unknown as SlashCommandHost;

    await handleDesktopCommand(host);

    expect(host.showStatus).toHaveBeenCalledWith(NOT_CONFIGURED);
    expect(mocks.openUrl).not.toHaveBeenCalled();
  });
});
