import { describe, expect, it } from 'vitest';

import type { FsGitStatusResponse } from '#/app/git/git';
import { gitDisclosure, renderGitDisclosure } from '#/features/gitStatus/gitStatusDisclosure';

const status = (over: Partial<FsGitStatusResponse> = {}): FsGitStatusResponse => ({
  branch: 'main',
  ahead: 0,
  behind: 0,
  entries: {},
  additions: 0,
  deletions: 0,
  pullRequest: null,
  ...over,
});

describe('gitDisclosure', () => {
  it('reports a clean tree with no changes', () => {
    const disclosure = gitDisclosure(status({ entries: { 'a.ts': 'clean', 'b.ts': 'ignored' } }));
    expect(disclosure).toMatchObject({ changed: 0, untracked: 0, branch: 'main' });
    expect(renderGitDisclosure(disclosure)).toBe(
      'This session is in a Git work tree on branch main. It has a clean working tree. DO NOT mention this to the user explicitly.',
    );
  });

  it('separates changed files from untracked files', () => {
    const disclosure = gitDisclosure(
      status({ entries: { 'a.ts': 'modified', 'b.ts': 'added', 'c.ts': 'untracked' } }),
    );
    expect(disclosure).toMatchObject({ changed: 2, untracked: 1 });
    expect(renderGitDisclosure(disclosure)).toContain('2 changed files, plus 1 untracked file.');
  });

  it('uses singular wording for a single change', () => {
    const text = renderGitDisclosure(gitDisclosure(status({ entries: { 'a.ts': 'deleted' } })));
    expect(text).toContain('It has 1 changed file.');
  });

  it('reports divergence from upstream', () => {
    const text = renderGitDisclosure(gitDisclosure(status({ ahead: 2, behind: 1 })));
    expect(text).toContain('2 commits ahead of upstream.');
    expect(text).toContain('1 commit behind upstream.');
  });
});
