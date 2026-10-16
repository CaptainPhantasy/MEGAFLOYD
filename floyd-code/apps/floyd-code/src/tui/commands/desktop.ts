import { floydCodeOfficialInstallUrl } from '#/constant/app';
import { openUrl } from '#/utils/open-url';
import { currentFloydProfile } from '#/utils/region';

import type { SlashCommandHost } from './dispatch';

export async function handleDesktopCommand(host: SlashCommandHost): Promise<void> {
  const { siteBase } = currentFloydProfile();
  const url = siteBase.length === 0 ? floydCodeOfficialInstallUrl() : `${siteBase}/code`;
  if (url.length === 0) {
    host.showStatus(
      'Floyd Code desktop app page is not configured (no region site base or install page URL available).',
    );
    return;
  }
  host.showStatus(`${url} — opened in your browser`);
  openUrl(url);
}
