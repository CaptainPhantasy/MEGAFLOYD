#!/usr/bin/env node
/**
 * Sign a built MegaFloyd engine with the persistent local identity and refresh
 * the build receipt and SHA256SUMS to match the signed bytes.
 *
 * The Go build emits an unsigned binary. The desktop builder refuses to embed
 * it unless it carries the `ai.legacy.megafloyd.engine` identifier and the
 * persistent certificate root, and the release packager checks the embedded
 * binary against the build receipt. Run this between `build-megafloyd.sh` and
 * `desktop/desktop/scripts/build.mjs`.
 *
 * Usage:
 *   node scripts/sign-megafloyd-engine.mjs <binary> [build-receipt.json]
 *
 * Run it once per build, before the desktop stage. Signing again after the
 * binary has been embedded changes the signature hash and breaks the packager's
 * receipt check.
 *
 * The receipt defaults to `<binary>/../../build-receipt.json`, which is the
 * layout written by `build-megafloyd.sh`.
 */
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { signCode } from '../desktop/desktop/scripts/stable-sign.mjs';

const ENGINE_IDENTIFIER = 'ai.legacy.megafloyd.engine';
const binary = process.argv[2];
if (!binary) {
  console.error('usage: node scripts/sign-megafloyd-engine.mjs <binary> [build-receipt.json]');
  process.exit(2);
}

const binaryPath = path.resolve(binary);
if (!fs.existsSync(binaryPath)) {
  console.error(`error: binary not found: ${binaryPath}`);
  process.exit(2);
}

const receiptPath = path.resolve(
  process.argv[3] ?? path.join(path.dirname(binaryPath), '..', 'build-receipt.json'),
);
if (!fs.existsSync(receiptPath)) {
  console.error(`error: build receipt not found: ${receiptPath}`);
  process.exit(2);
}

signCode(binaryPath, ENGINE_IDENTIFIER);

// Fail closed before the receipt is rewritten. If signing silently did nothing,
// the receipt must not attest a signature that is not there.
execFileSync(
  'codesign',
  ['--verify', '--strict', '-R', `=identifier "${ENGINE_IDENTIFIER}"`, binaryPath],
  { stdio: ['ignore', 'pipe', 'pipe'] },
);

const sha256 = createHash('sha256').update(fs.readFileSync(binaryPath)).digest('hex');
const receipt = JSON.parse(fs.readFileSync(receiptPath, 'utf8'));
const relativeBinary = path.relative(path.dirname(receiptPath), binaryPath).split(path.sep).join('/');
const artifact = (receipt.artifacts ?? []).find(item => item.path === relativeBinary);
if (!artifact) {
  console.error(`error: receipt has no artifact named ${relativeBinary}`);
  process.exit(1);
}
artifact.sha256 = sha256;
fs.writeFileSync(receiptPath, JSON.stringify(receipt) + '\n');

const sumsPath = path.join(path.dirname(receiptPath), 'SHA256SUMS');
if (fs.existsSync(sumsPath)) {
  const lines = fs.readFileSync(sumsPath, 'utf8').split('\n');
  const suffix = `  ${relativeBinary}`;
  const updated = lines.map(line => (line.endsWith(suffix) ? sha256 + suffix : line));
  fs.writeFileSync(sumsPath, updated.join('\n'));
}

console.log(JSON.stringify({
  binary: binaryPath,
  identifier: ENGINE_IDENTIFIER,
  sha256,
  build_receipt: receiptPath,
}, null, 2));
