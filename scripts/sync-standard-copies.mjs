// Regenerate the rule-text copies of MegaFloyd.md.
//
// MegaFloyd.md is the only authoritative behavioral document. Some tools read a
// differently named file, so AGENTS.md and claude.md carry the same rule text
// without the leading title. They must be generated from the authority rather
// than hand-maintained, or they drift — which they did.
//
// usage:
//   node scripts/sync-standard-copies.mjs          write the copies
//   node scripts/sync-standard-copies.mjs --check  verify only, exit 1 on drift
import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = join(fileURLToPath(new URL(".", import.meta.url)), "..");
const AUTHORITY = join(ROOT, "MegaFloyd.md");
const COPIES = ["AGENTS.md", "claude.md"];
const check = process.argv.includes("--check");

function ruleText(source) {
  const lines = source.replace(/\r\n/g, "\n").split("\n");
  let start = 0;
  if (lines.length > 0 && /^#\s/.test(lines[0])) {
    start = 1;
    while (start < lines.length && lines[start].trim() === "") start += 1;
  }
  let end = lines.length;
  while (end > start && lines[end - 1].trim() === "") end -= 1;
  return lines.slice(start, end).join("\n") + "\n";
}

if (!existsSync(AUTHORITY)) {
  console.error(`FAIL  ${AUTHORITY} is missing; refusing to generate copies from nothing`);
  process.exit(1);
}

const expected = ruleText(readFileSync(AUTHORITY, "utf8"));
let drifted = 0;

for (const name of COPIES) {
  const target = join(ROOT, name);
  const current = existsSync(target) ? readFileSync(target, "utf8") : null;
  const normalised = current === null ? null : ruleText(current);
  if (normalised === expected && current !== null) {
    if (!check) writeFileSync(target, expected);
    console.log(`ok    ${name} already matches the MegaFloyd.md rule text`);
    continue;
  }
  if (check) {
    console.error(`DRIFT ${name} does not match the MegaFloyd.md rule text`);
    drifted += 1;
    continue;
  }
  writeFileSync(target, expected);
  console.log(`SYNC  ${name} written from MegaFloyd.md`);
}

if (check && drifted > 0) {
  console.error(`DRIFT  ${drifted} copy/copies out of date. Run: node scripts/sync-standard-copies.mjs`);
  process.exit(1);
}
if (!check) console.log(`source ${AUTHORITY} was not modified`);
