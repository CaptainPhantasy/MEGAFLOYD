import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {
  MISMATCH_OVERRIDE_ENV,
  policySHA256,
  readEmbeddedPolicySHA256,
  verifyEmbeddedPolicy,
} from '../scripts/policy-check.mjs';

const NL = String.fromCharCode(10);

/** fixture writes a readable standard and a stub engine that answers `digest`. */
function fixture({ answer = null, embedPolicy = false, status = 0, stderr = '' } = {}) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'megafloyd-policy-'));
  const policy = path.join(dir, 'MegaFloyd.md');
  fs.writeFileSync(policy, 'The reviewed standard.' + NL);
  const binary = path.join(dir, 'megafloyd');
  const lines = ['#!/bin/sh'];
  if (stderr) lines.push(`printf '%s' ${JSON.stringify(stderr)} >&2`);
  const digest = embedPolicy ? policySHA256(policy) : answer;
  if (digest !== null) lines.push(`printf '%s\\n' ${JSON.stringify(digest)}`);
  lines.push(`exit ${status}`, '');
  fs.writeFileSync(binary, lines.join(NL));
  fs.chmodSync(binary, 0o755);
  return { dir, policy, binary };
}

test('a binary that embeds the readable standard passes', () => {
  const { policy, binary } = fixture({ embedPolicy: true });
  const warnings = [];
  const result = verifyEmbeddedPolicy({ binary, policy, warn: (message) => warnings.push(message) });
  assert.deepEqual(result, { expected: policySHA256(policy), embedded: policySHA256(policy), enforced: true });
  assert.equal(warnings.length, 0);
});

test('a different embedded standard fails the build and names both hashes', () => {
  const other = 'a'.repeat(64);
  const { policy, binary } = fixture({ answer: other });
  assert.throws(
    () => verifyEmbeddedPolicy({ binary, policy, warn: () => {} }),
    (error) => {
      assert.match(error.message, /embeds a different MegaFloyd standard/);
      assert.ok(error.message.includes(other), 'the embedded hash must appear');
      assert.ok(error.message.includes(policySHA256(policy)), 'the readable hash must appear');
      assert.ok(error.message.includes(`set ${MISMATCH_OVERRIDE_ENV}=1`), 'the escape hatch must be named');
      return true;
    },
  );
});

test('the escape hatch downgrades a mismatch to a loud warning', () => {
  const other = 'b'.repeat(64);
  const { policy, binary } = fixture({ answer: other });
  const warnings = [];
  const result = verifyEmbeddedPolicy({
    binary,
    policy,
    allowMismatch: true,
    warn: (message) => warnings.push(message),
  });
  assert.deepEqual(result, { expected: policySHA256(policy), embedded: other, enforced: false });
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /^WARNING: platform binary embeds a different MegaFloyd standard/);
  assert.match(warnings[0], new RegExp(`WARNING: ${MISMATCH_OVERRIDE_ENV}=1 is set`));
  assert.match(warnings[0], /will fail release verification/);
});

test('a pre-policy binary is a hard error that names it pre-policy', () => {
  const { policy, binary } = fixture({ status: 1, stderr: 'unknown command "policy"' });
  assert.throws(
    () => readEmbeddedPolicySHA256(binary),
    (error) => {
      assert.match(error.message, /is pre-policy/);
      assert.ok(error.message.includes(binary), 'the binary path must appear');
      assert.match(error.message, /unknown command "policy"/);
      return true;
    },
  );
  assert.throws(
    () => verifyEmbeddedPolicy({ binary, policy, warn: () => {} }),
    /is pre-policy/,
  );
});

test('the escape hatch also covers a pre-policy binary, loudly', () => {
  const { policy, binary } = fixture({ status: 2, stderr: 'flag provided but not defined' });
  const warnings = [];
  const result = verifyEmbeddedPolicy({
    binary,
    policy,
    allowMismatch: true,
    warn: (message) => warnings.push(message),
  });
  assert.equal(result.enforced, false);
  assert.equal(result.embedded, null);
  assert.match(warnings[0], /WARNING: platform binary is pre-policy/);
  assert.match(warnings[0], /will fail release verification/);
});

test('an unusable answer is rejected instead of compared', () => {
  const { policy, binary } = fixture({ answer: 'not-a-digest' });
  assert.throws(
    () => verifyEmbeddedPolicy({ binary, policy, warn: () => {} }),
    /unusable answer to 'prompt policy --sha256'/,
  );
});

test('the readable hash is the file the bundle ships', () => {
  const { policy } = fixture();
  assert.equal(policySHA256(policy), createHash('sha256').update(fs.readFileSync(policy)).digest('hex'));
});
