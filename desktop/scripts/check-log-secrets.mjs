#!/usr/bin/env node
/**
 * End-to-end regression check (spec C3): after a smoke run, no secret value
 * and no non-loopback handoff URL may survive in the desktop log.
 */
import fs from 'node:fs';
import path from 'node:path';

const NL = String.fromCharCode(10);
const home = process.argv[2];
if (!home) {
  process.stderr.write('usage: check-log-secrets.mjs <MEGAFLOYD_HOME>' + NL);
  process.exit(2);
}

const logsDir = path.join(home, 'logs');
const files = fs.existsSync(logsDir)
  ? fs.readdirSync(logsDir).filter((name) => name.startsWith('desktop.log'))
  : [];

const checks = [];
const check = (name, fn) => {
  try {
    checks.push({ name, ok: true, detail: fn() });
  } catch (error) {
    checks.push({ name, ok: false, detail: String(error?.message ?? error) });
  }
};

check('desktop log written', () => {
  if (files.length === 0) throw new Error(`no desktop.log under ${logsDir}`);
  return `${files.length} log file(s)`;
});

const text = files
  .map((name) => fs.readFileSync(path.join(logsDir, name), 'utf8'))
  .join(NL);

check('no token values in desktop log', () => {
  const pattern = /(?:[#?&;]token=|(?:^|\s)Token:\s+)(?!<redacted>)[A-Za-z0-9._~%+\/=-]+/gim;
  const hits = text.match(pattern) ?? [];
  if (hits.length > 0) throw new Error(`${hits.length} unredacted token value(s)`);
  return 'token values redacted';
});

check('no non-loopback Web UI URL in desktop log', () => {
  const pattern = /^\s*Web UI:\s+https?:\/\/(?!127\.0\.0\.1(?::|\/|$)|localhost(?::|\/|$)|\[::1\](?::|\/|$))\S+/gim;
  const hits = text.match(pattern) ?? [];
  if (hits.length > 0) throw new Error(`${hits.length} remote handoff URL(s)`);
  return 'remote handoff URLs suppressed';
});

for (const item of checks) {
  process.stdout.write(`${item.ok ? 'PASS' : 'FAIL'}  ${item.name}  ${item.detail}${NL}`);
}
const failed = checks.filter((item) => !item.ok);
process.stdout.write(`${NL}${checks.length - failed.length}/${checks.length} checks passed${NL}`);
process.exit(failed.length === 0 ? 0 : 1);
