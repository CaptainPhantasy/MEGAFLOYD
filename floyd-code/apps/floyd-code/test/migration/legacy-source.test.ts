import { describe, expect, it } from 'vitest';
import { join } from 'node:path';
import { resolveLegacySourceHome, sameLegacyPath } from '#/migration/legacy-source';

const HOME = '/home/user';
const CWD = '/work/project';

describe('resolveLegacySourceHome', () => {
  it('defaults to ~/.floyd when FLOYD_SHARE_DIR is unset', () => {
    const r = resolveLegacySourceHome({}, HOME, CWD);
    expect(r).toEqual({ sourceHome: join(HOME, '.floyd'), origin: 'default' });
  });

  it('defaults to ~/.floyd when FLOYD_SHARE_DIR is empty or blank', () => {
    expect(resolveLegacySourceHome({ FLOYD_SHARE_DIR: '' }, HOME, CWD).origin).toBe('default');
    expect(resolveLegacySourceHome({ FLOYD_SHARE_DIR: '   ' }, HOME, CWD).origin).toBe('default');
  });

  it('uses an absolute FLOYD_SHARE_DIR verbatim', () => {
    const r = resolveLegacySourceHome({ FLOYD_SHARE_DIR: '/data/floyd' }, HOME, CWD);
    expect(r.sourceHome).toBe('/data/floyd');
    expect(r.origin).toBe('share-dir');
  });

  it('resolves a relative FLOYD_SHARE_DIR against the process CWD (old-CLI rule)', () => {
    const r = resolveLegacySourceHome({ FLOYD_SHARE_DIR: 'relative/floyd' }, HOME, CWD);
    expect(r.sourceHome).toBe(join(CWD, 'relative', 'floyd'));
    expect(r.origin).toBe('share-dir');
  });

  it('does not expand ~ in FLOYD_SHARE_DIR (old-CLI rule)', () => {
    const r = resolveLegacySourceHome({ FLOYD_SHARE_DIR: '~/custom' }, HOME, CWD);
    expect(r.sourceHome).toBe(join(CWD, '~/custom'));
  });

  it('resolves skills from ~/.floyd when the share dir is redirected', () => {
    const r = resolveLegacySourceHome({ FLOYD_SHARE_DIR: '/data/floyd' }, HOME, CWD);
    expect(r.skillsSourceHome).toBe(join(HOME, '.floyd'));
  });

  it('keeps a single source when FLOYD_SHARE_DIR points at ~/.floyd itself', () => {
    const r = resolveLegacySourceHome({ FLOYD_SHARE_DIR: join(HOME, '.floyd') }, HOME, CWD);
    expect(r.skillsSourceHome).toBeUndefined();
  });
});

describe('sameLegacyPath', () => {
  it('matches identical and redundant forms', () => {
    expect(sameLegacyPath('/a/b', '/a/b')).toBe(true);
    expect(sameLegacyPath('/a/b/', '/a/b')).toBe(true);
    expect(sameLegacyPath('/a/./b', '/a/b')).toBe(true);
  });

  it('rejects different paths', () => {
    expect(sameLegacyPath('/a/b', '/a/c')).toBe(false);
  });
});
