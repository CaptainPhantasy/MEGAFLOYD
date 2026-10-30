import type { FsGitStatus, FsGitStatusResponse } from '#/app/git/git';

export interface GitDisclosure {
  readonly kind: 'git';
  readonly branch: string;
  readonly changed: number;
  readonly untracked: number;
  readonly ahead: number;
  readonly behind: number;
}

function isWorkingTreeChange(state: FsGitStatus): boolean {
  return state !== 'clean' && state !== 'ignored';
}

export function gitDisclosure(status: FsGitStatusResponse): GitDisclosure {
  let changed = 0;
  let untracked = 0;
  for (const state of Object.values(status.entries)) {
    if (state === 'untracked') untracked += 1;
    else if (isWorkingTreeChange(state)) changed += 1;
  }
  return {
    kind: 'git',
    branch: status.branch,
    changed,
    untracked,
    ahead: status.ahead,
    behind: status.behind,
  };
}

const plural = (count: number, noun: string): string =>
  `${count} ${noun}${count === 1 ? '' : 's'}`;

export function renderGitDisclosure(disclosure: GitDisclosure): string {
  const parts: string[] = [`This session is in a Git work tree on branch ${disclosure.branch}.`];
  const tracked =
    disclosure.changed === 0
      ? 'a clean working tree'
      : `${plural(disclosure.changed, 'changed file')}`;
  const untracked =
    disclosure.untracked > 0 ? `, plus ${plural(disclosure.untracked, 'untracked file')}` : '';
  parts.push(`It has ${tracked}${untracked}.`);
  if (disclosure.ahead > 0) parts.push(`${plural(disclosure.ahead, 'commit')} ahead of upstream.`);
  if (disclosure.behind > 0) parts.push(`${plural(disclosure.behind, 'commit')} behind upstream.`);
  parts.push('DO NOT mention this to the user explicitly.');
  return parts.join(' ');
}
