/**
 * Proof that the platform binary the desktop bundle embeds carries the same
 * MegaFloyd standard the build copies beside it.
 *
 * The bundle ships a readable MegaFloyd.md and a Go engine that enforces the
 * standard compiled into it. Copying the file and hashing the copy proves
 * nothing about the engine, so the build asks the binary itself with
 * `prompt policy --sha256` and compares that answer with the readable bytes.
 */
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import fs from 'node:fs';

/** Set to `1` to downgrade a failure described here into a loud warning. */
export const MISMATCH_OVERRIDE_ENV = 'MEGAFLOYD_ALLOW_POLICY_MISMATCH';

/** policySHA256 hashes the readable standard the bundle ships. */
export function policySHA256(policy) {
  return createHash('sha256').update(fs.readFileSync(policy)).digest('hex');
}

/**
 * readEmbeddedPolicySHA256 asks the binary which standard it enforces.
 * A binary built before the policy subcommand existed throws, and the message
 * must say so plainly: it is not a transient failure.
 */
export function readEmbeddedPolicySHA256(binary) {
  let output;
  try {
    output = execFileSync(binary, ['prompt', 'policy', '--sha256'], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
    });
  } catch (error) {
    const detail = String(error?.stderr ?? error?.message ?? '').trim();
    throw new Error(
      `platform binary is pre-policy: it does not answer 'prompt policy --sha256', so nothing can prove which MegaFloyd standard it embeds: ${binary}` +
        `${detail ? `\n  ${detail}` : ''}`,
    );
  }
  const digest = output.trim();
  if (!/^[0-9a-f]{64}$/.test(digest)) {
    throw new Error(
      `platform binary returned an unusable answer to 'prompt policy --sha256' (${JSON.stringify(digest)}): ${binary}`,
    );
  }
  return digest;
}

/**
 * verifyEmbeddedPolicy fails unless the binary embeds exactly the standard at
 * `policy`.
 *
 * `allowMismatch` downgrades both outcomes — a different embedded standard and
 * a binary that is too old to answer — into a warning, because a developer may
 * legitimately build against an older engine while the readable standard has
 * already moved ahead. Such a bundle cannot pass release verification, which
 * re-checks the same equality against the installed binary.
 */
export function verifyEmbeddedPolicy({
  binary,
  policy,
  allowMismatch = false,
  warn = (message) => process.stderr.write(message),
}) {
  const expected = policySHA256(policy);
  let embedded;
  try {
    embedded = readEmbeddedPolicySHA256(binary);
  } catch (error) {
    if (!allowMismatch) throw error;
    warn(
      `WARNING: ${error.message}\n` +
        `WARNING: ${MISMATCH_OVERRIDE_ENV}=1 is set; building anyway. This bundle will fail release verification.\n`,
    );
    return { expected, embedded: null, enforced: false };
  }
  if (embedded === expected) return { expected, embedded, enforced: true };

  const message =
    `platform binary embeds a different MegaFloyd standard than the readable ${policy}` +
    `\n  binary:   ${embedded}\n  readable: ${expected}`;
  if (!allowMismatch) {
    throw new Error(
      `${message}\n  Rebuild the platform binary from floyd-go, or set ${MISMATCH_OVERRIDE_ENV}=1 to build anyway.`,
    );
  }
  warn(
    `WARNING: ${message}\n` +
      `WARNING: ${MISMATCH_OVERRIDE_ENV}=1 is set; building anyway. This bundle will fail release verification.\n`,
  );
  return { expected, embedded, enforced: false };
}
