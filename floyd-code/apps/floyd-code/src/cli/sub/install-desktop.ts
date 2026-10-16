import type { Command } from 'commander';

import { floydCodeOfficialInstallUrl } from '#/constant/app';
import { openUrl } from '#/utils/open-url';
import { currentFloydProfile } from '#/utils/region';

function desktopAppPageUrl(): string {
  const { siteBase } = currentFloydProfile();
  return siteBase.length === 0 ? floydCodeOfficialInstallUrl() : `${siteBase}/code`;
}

function openDesktopAppPage(): void {
  const url = desktopAppPageUrl();
  if (url.length === 0) {
    process.stdout.write(
      'Floyd Code desktop app page is not configured (no region site base or install page URL available).\n',
    );
    return;
  }
  process.stdout.write(`${url}\n`);
  openUrl(url);
}

export function registerInstallDesktopCommand(program: Command): void {
  program
    .command('install-desktop')
    .description('Print the Floyd Code desktop app page and open it in your browser.')
    .action(openDesktopAppPage);

  program.command('install-app', { hidden: true }).action(openDesktopAppPage);
}
