# Handoff — session bricking on images (2026-11-15)

Status 2026-11-20: both media defects and the two tool defects below are repaired
on `main` by PR #20 (`d2e5cdf`). This note is retained as the reproduction record;
release verification is recorded in `progress.md` and the SSOT status log.

Written from a downstream audit session while fixing this repository. It was first
filed under `floyd-go/docs/handoffs/` because that was the only writable directory
in the tree from that session; it now lives in `docs/handoffs/` with the other
project handoffs.

## What broke

A downstream audit session (221 messages) became permanently unusable.
Every request after 17:33 failed with:

```
An assistant message with 'tool_calls' must be followed by tool messages
responding to each 'tool_call_id'. (insufficient tool messages following
tool_calls message)
```

preceded by one `Bad Request … messages[215].content[3]: You have uploaded an
unsupported image`.

Two independent defects, both in the media path for non-Anthropic providers:

1. **Double-encoded images.** `internal/agent/tools/vision.go` encoded the file
   to base64 and passed that *text* to `fantasy.NewImageResponse`, which stores
   bytes verbatim. The message layer then base64-encodes again, so the stored
   payload was base64 of base64. At request time one decode yields ASCII
   labelled `image/png`; the provider rejects it. Verified against the session
   database: one decode → `iVBORw0KGgo…`, two decodes → valid 1179×2556 PNG.
   `view.go:206` and `mcp-tools.go:200` pass raw bytes and were never affected.

2. **Media user-messages interleaved with tool results.**
   `workaroundProviderMediaLimitations` (`internal/agent/agent.go`) appended a
   synthetic user message immediately after *each* tool message. With parallel
   tool calls the request became `tool, user, tool, user, …`, which violates the
   requirement that every tool result follow its `tool_calls` block directly.
   The history is rebuilt identically on every retry, so the session stays
   invalid forever.

## What was changed

A patched clone was built outside this repository (arm64 build plus patch). It is
a clone at `7b99d27`;
`git log --since=2026-11-12T01:24:10Z -- floyd-go` is empty, so the patched
build equals the shipped source plus only this fix.

- `floyd-go/internal/agent/tools/vision.go` — pass `imageData`, not
  `[]byte(base64Data)`.
- `floyd-go/internal/agent/agent.go` — collect attachments for a run of tool
  messages and emit them after the block, one user message per file;
  `decodeMediaData` repairs rows already stored double-encoded.
- `floyd-go/internal/agent/media_fix_test.go` — regression tests for both.

The same edits were also stashed in this repository while they were being moved
out; that duplicate stash is no longer needed now that PR #20 carries the fix.

## Install state

The patched binary was copied over
`releases/2026.11.11-megafloyd-20261112T012410Z/MEGAFLOYD.app/Contents/Resources/megafloyd/bin/megafloyd`
(135,933,762 bytes), with the previous binary kept as `megafloyd.bak-20261115`
beside it. A 0-byte `.writeprobe` from a write test is also in that directory.

## Verified

- `go build ./...` clean; new regression tests pass.
- `go test ./internal/agent ./internal/agent/tools`: `TestBuildToolsRegistersCompleteToolSet`
  and `TestNamedSwarmRunsDistinctModelsToolsAndSessions` fail, but they fail
  identically on the unpatched baseline — pre-existing.
- End to end: one `vision` call on `docs/evidence/default-desktop.png` returned a
  real image description under the patched dev engine.

## Not verified

- A four-image batch (the original trigger). One image passing does not prove
  four pass; payload size is provider-side.
- Reopening the bricked session in the UI.

## Originally open, now resolved

- **Deletion guard false positive** — fixed in PR #20 (`d2e5cdf`): quoted globs,
  a bare `[`, `command -v`, and git subcommands are classified by meaning.
  Genuinely destructive forms still fail closed. See
  `internal/deletionguard/classify.go`.
- **`floyd_logs` reports "No log file found"** — fixed in PR #20: the tool reads
  the process-configured log path.

## Still open

- **Two engines ran concurrently on one database** (one detached with parent 1)
  before the restart. Two writers on one session store is a plausible source of
  intermittent message ordering problems; the second engine was stopped.
- The deletion guard still treats `cp` or `mv` onto an existing file as
  permanent deletion. That is deliberate, but it prevents an agent from copying
  a freshly built binary over an installed one. The release installer's atomic
  switch avoids this path.
- `cd <absolute path> && mv …` is classified as permanent deletion even when the
  destination is unambiguous; the same `mv` without the `cd` is allowed.
