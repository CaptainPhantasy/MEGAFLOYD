# B1 — Context assembly in Floyd Code (design, verified 2026-10-16)

Goal (PLAN.md P2): fill the prompt pack at session start in `agent-core-v2`.
Gate: `pnpm --filter @legacy-ai/agent-core-v2 typecheck` + `vitest run` pass, and the assembled
prompt is dumped and diffed against the Go output.

## Verified seam (file:line, this tree)

| Stage | Location |
|---|---|
| Prompt context builder | `agent/profile/profileService.ts:805` `buildSystemPromptContext()` |
| Base context | `agent/profile/context.ts:33` `prepareSystemPromptContext()` |
| Context type | `app/agentProfileCatalog/agentProfileCatalog.ts:12` `AgentProfileContext` |
| Variables bag | `app/agentProfileCatalog/profile-shared.ts:136` `systemPromptVars()` |
| Template | `app/agentProfileCatalog/system.md` (`${var}` syntax) |
| Renderer | `_base/utils/render-prompt.ts:3` `renderPrompt()` (`${name}` only) |

`SystemPromptContext` (`agent/profile/profile.ts:42`) extends `AgentProfileContext`.

## Verified gap vs Go `PromptDat`

| Go `PromptDat` | TS today | Verdict |
|---|---|---|
| `WorkingDir` | `cwd` var | present |
| `Platform` | `os` var (`osKind`) | present |
| `AvailSkillXML` | `skills` var | present |
| `AgentName` | `product_name` (identity) | present |
| `ContextFiles`/`GlobalContextFiles` | `agents_md` var | present (different shape) |
| `Provider`, `Model` | model alias handled elsewhere | out of prompt-var scope |
| **`Date`** | `features/dateChange/dateChangeService.ts` (reminder) | **present, by a different mechanism — NOT a gap** |
| **`IsGitRepo`** | — | **GAP** |
| **`GitStatus`** | `app/git/gitService.ts:31` `status(cwd)` exists, but nothing injects it | **GAP** |

**Corrected assumption:** the first read of this task ("add `date`/`git_status` vars") was wrong.
`system.md` states the date is disclosed via reminders; adding a `date` var would duplicate that.
The real gap is **git branch/status disclosure**, which Go does in-prompt and Floyd Code does not
do at all.

## Decision to make (B1-D1)

Where git disclosure lives, to match Go without fighting the TS architecture:
- **(recommended)** a session-start reminder carrying branch + short status — mirrors the existing
  `dateChange` pattern, keeps `system.md` stable, and reuses `IGitService.status`.
- alternative: a `${git_status}` prompt var filled in `buildSystemPromptContext`. Cheaper, but
  diverges from the reminder-based convention the TS side already set for date.

Record the choice in `docs/DECISIONS.md` before coding.

## First code step (after B1-D1)

1. Read `features/dateChange/dateChangeService.ts` as the pattern to mirror.
2. Add git disclosure using `IGitService.status(sessionContext.cwd)` (branch + porcelain summary),
   only when the dir is a git work tree (`findGitWorkTree`, already imported in `context.ts`).
3. Vitest under `src/human/test/**` asserting: non-git dir → no disclosure; clean repo → branch
   only; dirty repo → branch + counts.
4. Gate: package `typecheck` + `vitest run`.
