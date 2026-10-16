# HANDOFF — MEGAFLOYD

Written 2026-10-16 for whoever picks this up next, human or agent. Everything below was
checked on this machine today. Short on purpose.

## What it is

One tool built by taking the best parts of the builds already on this machine. New folder,
new GitHub home, so nothing that already works can be broken by it.

- Folder: `/Users/floyd/MEGAFLOYD`
- GitHub: `github.com/CaptainPhantasy/MEGAFLOYD` (branch `main`, level with this machine)
- The plan: `task_plan.md` in that folder. Findings: `findings.md`. Session log: `progress.md`.
  Decisions: `docs/DECISIONS.md`.

## What is verified done

- The whole thing installs and builds from a clean copy: install, full build, the app reports
  `2.0.2`, and the web-assets check passes. Recorded in `progress.md` with the commands.
- A start-up file `FLOYD.md` sits in the project root, so a session opened there knows what the
  project is and what the house rules are without being told.
- `floyd-code` (the interface half, 2.0.2) and `floyd-go` (the behaviour half) are copied in.

## What is NOT done — the actual work

- None of the behaviour half is inside the interface half yet. No reading of `FLOYD.md`, no
  reading of the memory folder, no command guards, no identity. The interface does not yet
  behave like the thing that was reliable.
- Nothing is packaged: no launcher, no home folder of its own.
- No readme, no screenshots, and the web interface is still one layout rather than the three
  separate ones asked for (desktop, iPad, iPhone 16 Pro).

## Correction worth knowing before starting

An earlier note claimed the September 10 backup build (`floyd-v6.backup-20260910`, reports
`v6.0.0`) was already the combined build. It is not: it does not carry the ralph loop, the
workflows or the plugins. There is **no ready-made merged build** on this machine.

## The builds on this machine, and what each is

- `f7` — 2.0.2, the interface app (same line as `floyd-code`).
- `floyd-v6` — v5.3.0, what is in use. This line carries the ralph loop, the built-in
  workflows, the plugins, the embedded terminal, the path checks.
- `floyd-v6.backup-20260910` — v6.0.0. Missing the above machinery.
- `superfloyd-real` — v5.6.3, a separate production line.
- `floyd-v6-staging` — v0.92.1, the line matching the public repo.
- `floyd.bak.20260509` — v0.58.1, a May build.
- Older product folders still present: `.floyd7`, `.floyd-code`, `.floyd-10x`, `.floyd-fork`,
  `.floyd-superfloyd`, `.floyd-ff.fresh-20260826`, `hivemind-build`.

## The next step, in order

1. Compare those builds on what a user can actually see and use — commands, menus, wording —
   not on internal labels. Two of the builds are compiled without those labels, so comparing
   by labels gives false blanks (this mistake was already made once today).
2. From that comparison, decide what gets carried over from which build.
3. Carry the behaviour half into the interface half: reading `FLOYD.md` and the memory folder
   at start-up, then the command guards, then identity.
4. Package it, then write the readme and take real screenshots.

## Traps already paid for — do not repeat

- The plan once listed file paths that do not exist; five of nine were wrong. Check a path
  exists before working from it.
- Claims in these files marked as done may be wrong. Re-run the check before trusting it.
- Copying a new version of a program over the old file in place makes the machine refuse to
  run it. Remove the old file first, then copy.
- Do not modify the two source trees this was built from. Read only.
- Truncated checks are a trap: an early check of mine used a wrong root folder and I reported
  two items as broken that were fine.

## Evidence

- Full session record: `/Users/floyd/.floyd/exports/session-export-2026-10-16-115402.md`
- The read-out report on this project (text and audio) was published to the reports service
  on 2026-10-16: `megafloyd-merge-status-2026-10-16`.
