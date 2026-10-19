#!/usr/bin/env node
/**
 * Upstream watch for the pinned Kimi desktop base.
 *
 * The Kimi desktop app is closed source and has no official GitHub release
 * feed. This script therefore reports *signals*, never decides anything:
 *
 *   1. The version pinned in vendor/ (from the app's Info.plist).
 *   2. The newest release tag of a community mirror that republishes Kimi
 *      Code builds (useful only as a "upstream is active" hint — it is a
 *      different product from the Kimi desktop).
 *   3. An optional official download page (KIMI_DOWNLOAD_URL), scanned for a
 *      kimi_<version>.dmg style filename.
 *
 * Exit codes:
 *   0  no newer signal found
 *   10 a newer signal was found; a human must decide whether to adopt it
 *   2  the check itself could not run
 *
 * Nothing is downloaded, installed, or executed.
 */
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..', '..');
const VENDOR_APP = path.join(ROOT, 'vendor', 'Kimi-3.2.17-arm64.app');
const MIRROR_REPO = 'huangzhixin0420/kimicode-desktop';
const DOWNLOAD_URL = process.env.KIMI_DOWNLOAD_URL ?? '';

function pinnedVersion() {
  try {
    return execFileSync('plutil', [
      '-extract', 'CFBundleShortVersionString', 'raw', '-o', '-',
      path.join(VENDOR_APP, 'Contents', 'Info.plist'),
    ], { encoding: 'utf8' }).trim();
  } catch {
    return null;
  }
}

function versionParts(v) {
  return String(v).replace(/^v/i, '').split(/[^0-9]+/).filter(Boolean).map(Number);
}

function isNewer(candidate, pinned) {
  const a = versionParts(candidate);
  const b = versionParts(pinned);
  for (let i = 0; i < Math.max(a.length, b.length); i += 1) {
    const x = a[i] ?? 0;
    const y = b[i] ?? 0;
    if (x !== y) return x > y;
  }
  return false;
}

async function mirrorTag() {
  try {
    const res = await fetch(`https://api.github.com/repos/${MIRROR_REPO}/releases/latest`, {
      headers: { accept: 'application/vnd.github+json', 'user-agent': 'megafloyd-upstream-watch' },
    });
    if (!res.ok) return { error: `HTTP ${res.status}` };
    const data = await res.json();
    return { tag: data.tag_name ?? null, published: data.published_at ?? null };
  } catch (error) {
    return { error: error.message };
  }
}

async function downloadPageVersion() {
  if (!DOWNLOAD_URL) return { skipped: true };
  try {
    const res = await fetch(DOWNLOAD_URL, { headers: { 'user-agent': 'megafloyd-upstream-watch' } });
    if (!res.ok) return { error: `HTTP ${res.status}` };
    const html = await res.text();
    const match = html.match(/kimi[_-](\d+\.\d+(?:\.\d+)?)/i);
    return { version: match ? match[1] : null };
  } catch (error) {
    return { error: error.message };
  }
}

const pinned = pinnedVersion();
if (!pinned) {
  console.error('UPSTREAM  cannot read the pinned vendor version');
  process.exit(2);
}

const [mirror, page] = await Promise.all([mirrorTag(), downloadPageVersion()]);

console.log(`pinned vendor version      ${pinned}`);
console.log(`mirror ${MIRROR_REPO}  ${mirror.error ? `error: ${mirror.error}` : mirror.tag ?? 'no releases'}`);
if (page.skipped) {
  console.log('official download page     skipped (set KIMI_DOWNLOAD_URL to enable)');
} else if (page.error) {
  console.log(`official download page     error: ${page.error}`);
} else {
  console.log(`official download page     ${page.version ?? 'no version found in page'}`);
}

const candidates = [];
// The mirror republishes Kimi Code with date-based tags; it is a different
// product and must never be compared against the desktop version. It is
// reported above as an activity hint only.
if (page.version && isNewer(page.version, pinned)) candidates.push(`download page ${page.version}`);

if (candidates.length > 0) {
  console.log('\nnewer signal(s) found — a human decides whether to adopt:');
  for (const candidate of candidates) console.log(`  - ${candidate}`);
  console.log('\nAdoption is manual: see docs/UPSTREAM.md.');
  process.exit(10);
}

console.log('\nno newer signal found. Upstream may still have shipped; check the official download page manually.');
process.exit(0);
