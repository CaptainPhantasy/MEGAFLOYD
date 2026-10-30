#!/usr/bin/env node
// Prompt-pack drift guard.
//
// docs/prompt-pack/templates is the canonical behaviour pack. floyd-go keeps a
// physical copy because its build embeds templates from inside its own module
// (go:embed cannot reach a parent directory), so the copy is derived and must
// never be edited directly.
//
// usage:
//   node scripts/verify-prompt-pack.mjs          verify all copies, exit 1 on drift
//   node scripts/verify-prompt-pack.mjs --sync   overwrite derived copies from canonical

import { createHash } from "node:crypto";
import { readdirSync, readFileSync, statSync, writeFileSync, existsSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = join(fileURLToPath(new URL(".", import.meta.url)), "..");

const CANONICAL_DIR = join(ROOT, "docs/prompt-pack/templates");
const DERIVED_DIRS = [join(ROOT, "floyd-go/internal/agent/templates")];
const ADVISORY_DIRS = [join(ROOT, "..", "floyd-merge/prompt-pack/templates")];

const ASSEMBLY_PAIRS = [
  [join(ROOT, "MegaFloyd.md"), join(ROOT, "floyd-go/internal/agent/prompt/megafloyd_standard.md")],
  [join(ROOT, "docs/prompt-pack/prompt.go.reference"), join(ROOT, "floyd-go/internal/agent/prompt/prompt.go")],
];

const sync = process.argv.includes("--sync");

function sha256(file) {
  return createHash("sha256").update(readFileSync(file)).digest("hex");
}

function listTemplates(dir) {
  if (!existsSync(dir)) return null;
  return readdirSync(dir)
    .filter((f) => statSync(join(dir, f)).isFile() && !f.includes('.bak-') && !f.includes('.bak.'))
    .sort();
}

const canonicalFiles = listTemplates(CANONICAL_DIR);
if (!canonicalFiles) {
  console.error(`FAIL  canonical pack missing: ${relative(ROOT, CANONICAL_DIR)}`);
  process.exit(1);
}

const canonical = new Map(canonicalFiles.map((f) => [f, sha256(join(CANONICAL_DIR, f))]));
let drifted = 0;
let synced = 0;

for (const dir of DERIVED_DIRS) {
  const files = listTemplates(dir);
  const label = relative(ROOT, dir);
  if (!files) {
    console.error(`FAIL  derived copy missing: ${label}`);
    drifted += 1;
    continue;
  }
  for (const name of canonicalFiles) {
    if (!files.includes(name)) {
      if (sync) {
        writeFileSync(join(dir, name), readFileSync(join(CANONICAL_DIR, name)));
        console.log(`SYNC  ${label}/${name}`);
        synced += 1;
      } else {
        console.error(`FAIL  ${label} is missing ${name}`);
        drifted += 1;
      }
      continue;
    }
    const want = canonical.get(name);
    const got = sha256(join(dir, name));
    if (want === got) continue;
    if (sync) {
      writeFileSync(join(dir, name), readFileSync(join(CANONICAL_DIR, name)));
      console.log(`SYNC  ${label}/${name}`);
      synced += 1;
    } else {
      console.error(`FAIL  ${label}/${name} differs from canonical`);
      console.error(`        canonical ${want}`);
      console.error(`        on disk   ${got}`);
      drifted += 1;
    }
  }
  for (const name of files) {
    if (!canonicalFiles.includes(name)) {
      console.error(`FAIL  ${label}/${name} does not exist in canonical`);
      drifted += 1;
    }
  }
}

for (const [canonFile, derivedFile] of ASSEMBLY_PAIRS) {
  if (!existsSync(canonFile) || !existsSync(derivedFile)) {
    console.error(`FAIL  assembly pair missing: ${relative(ROOT, canonFile)} / ${relative(ROOT, derivedFile)}`);
    drifted += 1;
    continue;
  }
  const want = sha256(canonFile);
  const got = sha256(derivedFile);
  if (want === got) continue;
  if (sync) {
    writeFileSync(derivedFile, readFileSync(canonFile));
    console.log(`SYNC  ${relative(ROOT, derivedFile)}`);
    synced += 1;
  } else {
    console.error(`FAIL  ${relative(ROOT, derivedFile)} differs from ${relative(ROOT, canonFile)}`);
    drifted += 1;
  }
}

const STANDARD_COPIES = ["AGENTS.md", "claude.md"];

function normalizedRuleText(file) {
  const lines = readFileSync(file, "utf8").replace(/\r\n/g, "\n").split("\n");
  let start = 0;
  if (lines.length > 0 && /^#\s/.test(lines[0])) {
    start = 1;
    while (start < lines.length && lines[start].trim() === "") start += 1;
  }
  let end = lines.length;
  while (end > start && lines[end - 1].trim() === "") end -= 1;
  return lines.slice(start, end).join("\n");
}

const authorityFile = join(ROOT, "MegaFloyd.md");
const authorityText = existsSync(authorityFile) ? normalizedRuleText(authorityFile) : null;
for (const name of STANDARD_COPIES) {
  const file = join(ROOT, name);
  if (!existsSync(file)) continue;
  if (authorityText === null) {
    console.error(`FAIL  ${name} exists but MegaFloyd.md does not, so drift cannot be detected`);
    drifted += 1;
    continue;
  }
  if (normalizedRuleText(file) !== authorityText) {
    console.error(`FAIL  ${name} rule text differs from MegaFloyd.md — regenerate the copy from the authoritative file`);
    drifted += 1;
  } else {
    console.log(`ok    ${name} matches the MegaFloyd.md rule text`);
  }
}

for (const dir of ADVISORY_DIRS) {
  const files = listTemplates(dir);
  if (!files) continue;
  const path = relative(ROOT, dir);
  const mismatched = canonicalFiles.filter((name) => !files.includes(name) || sha256(join(dir, name)) !== canonical.get(name));
  if (mismatched.length > 0) {
    console.log(`WARN  unversioned copy outside the repo has drifted: ${path}`);
    for (const name of mismatched) console.log(`        ${name}`);
  } else {
    console.log(`ok    unversioned copy outside the repo matches canonical: ${path}`);
  }
}

console.log("");
console.log(`canonical pack: ${relative(ROOT, CANONICAL_DIR)} (${canonicalFiles.length} files, ${canonicalFiles.join(", ")})`);
if (sync) {
  console.log(`synced ${synced} file(s) from canonical`);
  process.exit(0);
}
if (drifted > 0) {
  console.error(`DRIFT  ${drifted} problem(s). Edit the canonical pack, then re-run with --sync.`);
  process.exit(1);
}
console.log("PASS  every derived copy matches the canonical pack");
