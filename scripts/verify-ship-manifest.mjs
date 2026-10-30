// Verify ship-manifest.json: every declared path exists, every pinned sha256 matches the
// bytes on disk, and every referenced release manifest records the approved policy standard.
// A declared-but-absent artifact, a stale pin, or a release whose embedded policy is not the
// checked-out MegaFloyd.md must fail the build rather than ship an unapproved package.
//
// --checkout-only skips the checks whose inputs are not in git (release artifacts under
// dist/, releases/ and bin/ are ignored) and reports every skipped check by name, so CI can
// verify the approved policy pin without pretending to verify artifacts it cannot see.
import { readFileSync, existsSync, statSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const POLICY_PATH = 'MegaFloyd.md';
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const manifest = JSON.parse(readFileSync(resolve(root, 'ship-manifest.json'), 'utf8'));
const checkoutOnly = process.argv.includes('--checkout-only');

function sha256(file) {
  return createHash('sha256').update(readFileSync(file)).digest('hex');
}

const problems = [];
const deferred = [];

const policyFile = resolve(root, POLICY_PATH);
const policySha = existsSync(policyFile) ? sha256(policyFile) : null;
if (!policySha) {
  problems.push(`${POLICY_PATH}: approved policy standard is absent, so no release can be verified against it`);
}

function releaseManifestProblems(path, file) {
  let release;
  try {
    release = JSON.parse(readFileSync(file, 'utf8'));
  } catch (error) {
    return [`${path}: release manifest is not readable JSON (${error.message})`];
  }
  const policy = release.components?.policy;
  if (!policy || typeof policy.sha256 !== 'string') {
    return [
      `${path}: PRE-POLICY release manifest — components.policy is absent, so the policy this release embeds cannot be verified against the approved standard. Rebuild the release.`,
    ];
  }
  if (policySha && policy.sha256 !== policySha) {
    return [`${path}: components.policy.sha256 ${policy.sha256} does not match ${POLICY_PATH} ${policySha}`];
  }
  return [];
}

if (!Array.isArray(manifest.ship_set) || manifest.ship_set.length === 0) {
  problems.push('ship_set must be a non-empty array');
}

const seen = new Set();
for (const entry of manifest.ship_set || []) {
  if (typeof entry.path !== 'string' || !entry.path) { problems.push('entry without a path'); continue; }
  if (!entry.kind) problems.push(`${entry.path}: missing kind`);
  if (seen.has(entry.path)) problems.push(`${entry.path}: duplicate entry`);
  seen.add(entry.path);

  const file = resolve(root, entry.path);
  if (!existsSync(file)) {
    if (checkoutOnly) {
      deferred.push(`${entry.path}: absent from this checkout, so its bytes were not checked`);
      continue;
    }
    problems.push(`${entry.path}: declared but absent`);
    continue;
  }

  if (entry.sha256) {
    if (!statSync(file).isFile()) {
      problems.push(`${entry.path}: pins a sha256 but is not a regular file`);
    } else {
      const actual = sha256(file);
      if (actual !== entry.sha256) {
        problems.push(`${entry.path}: pinned sha256 mismatch — ship-manifest declares ${entry.sha256}, on disk ${actual}`);
      }
    }
  }

  if (entry.kind === 'release-manifest') {
    if (checkoutOnly) {
      deferred.push(`${entry.path}: release-manifest policy cross-check requires the release tree, not the git tree`);
    } else {
      problems.push(...releaseManifestProblems(entry.path, file));
    }
  }
}

if (problems.length) {
  console.error('ship manifest FAILED:');
  for (const problem of problems) console.error('  - ' + problem);
  process.exit(1);
}
console.log(`ship manifest OK (${manifest.ship_set.length} entries) release ${manifest.release_id}`);
console.log(`  policy standard ${POLICY_PATH} ${policySha}`);
if (checkoutOnly) {
  console.log(`  checkout-only: ${deferred.length} check(s) deferred, each named below`);
  for (const item of deferred) console.log('    - ' + item);
}
