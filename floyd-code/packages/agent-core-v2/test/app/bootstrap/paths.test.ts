import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { afterEach, describe, expect, it } from 'vitest';

import { ensureFloydHome, resolveConfigPath, resolveFloydHome } from '#/app/bootstrap/bootstrap';

describe('bootstrap path helpers', () => {
  describe('resolveFloydHome', () => {
    it('uses explicit homeDir when provided', () => {
      expect(resolveFloydHome('/tmp/floyd')).toBe('/tmp/floyd');
    });

    it('falls back to FLOYD_CODE_HOME env', () => {
      const prev = process.env['FLOYD_CODE_HOME'];
      process.env['FLOYD_CODE_HOME'] = '/env/floyd';
      try {
        expect(resolveFloydHome()).toBe('/env/floyd');
      } finally {
        if (prev === undefined) delete process.env['FLOYD_CODE_HOME'];
        else process.env['FLOYD_CODE_HOME'] = prev;
      }
    });
  });

  describe('resolveConfigPath', () => {
    it('uses explicit configPath when provided', () => {
      expect(resolveConfigPath({ configPath: '/x/config.toml' })).toBe('/x/config.toml');
    });

    it('joins homeDir with config.toml', () => {
      expect(resolveConfigPath({ homeDir: '/tmp/floyd' })).toBe('/tmp/floyd/config.toml');
    });
  });

  describe('ensureFloydHome', () => {
    let dir: string | undefined;
    afterEach(() => {
      if (dir) rmSync(dir, { recursive: true, force: true });
    });

    it('creates the directory with 0700 permissions', () => {
      dir = join(mkdtempSync(join(tmpdir(), 'floyd-home-')), 'nested');
      ensureFloydHome(dir);
      expect(existsSync(dir)).toBe(true);
    });
  });
});
