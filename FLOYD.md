# FLOYD.md — MEGAFLOYD

This folder is **MEGAFLOYD**, and it is the only project. If a session opens in this
folder, this file is what it should be working on — no asking which project first.

## What it is

One build that joins two halves that were previously separate: the shell half
(the interface, the servers, the web view, the editor app) and the behaviour half
(prompt and context assembly, identity, tool discipline, hooks, guards). The point
is to end up with a single tool rather than two unrelated ones.

## Where the plan is

- `task_plan.md` — the master plan: tracks A–F, each phase with its proof gate.
- `progress.md` — what each session actually did, with the commands and results.
- `findings.md` — defects found, with evidence.
- `docs/DECISIONS.md` — every decision, and why, including ones that reversed an earlier call.

Work the next unfinished phase in `task_plan.md`. Do not invent work outside it.

## Rules that do not need restating

1. **Verify before asserting.** No claim about the state of anything without the command
   and its output.
2. **Label every fact.** `checked` (ran it), `reasoned` (not run), `heard` (told or guessed —
   never promoted without a run).
3. **Proof gates only.** A phase closes when its gate has been run and recorded. Prose
   does not close a phase.
4. **One hypothesis, one fix, observe.** If nothing changes, retract and stop — do not loop.
5. **Never modify the two source trees.** `floyd-v6/src` and `floyd-v6/KimiSource/kimi-code`
   are read-only inputs.
6. **Isolation.** Never point the merged build at `~/.floyd7`, `~/.floyd-ff`, `~/.floyd-code`
   or `~/.floyd-10x`. The merged home folder is `~/.megafloyd`.
7. **No secrets, no credentials copied from anywhere, nothing pushed that contains them.**
8. **Plain language with the owner.** No jargon. Every reply should be readable by someone
   who has never written code.
9. **Save memory at every phase boundary** into `progress.md` and the owner's notes folder.

## Order of work, and why this order

Nothing downstream is trustworthy until a fresh copy builds and runs, so that comes first.
Then work already half-done is finished before anything new is opened. Behaviour moves into
the new shell before any new surface is opened. Packaging comes after behaviour works.
The readme and screenshots come last, taken from the real thing — never mocked up.
