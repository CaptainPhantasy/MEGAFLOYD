# Fast — Speed Mode

A one-shot preset for cheap, quick turns: small model, low reasoning effort, terse output, no ceremony. Sets everything at once; /effort changes only the thinking dial.

## Usage
```
/fast
```

## Procedure
1. Switch to the small/secondary model via the model picker (ctrl+p → model; the config's small-model slot — internal/config resolves `large` and `small` model roles).
2. Set `reasoning_effort` to `low` (see /effort).
3. Adopt terse-output protocol for the session: answer first, shortest correct form, no restating of the task, no summaries of unchanged state.
4. Prefer `parallel_bash` for independent commands — one call, all results.
5. Report activation in one line. To leave speed mode: /effort medium (or high) and switch the model back.

## Rules
- Fast never means skipping verification: a claimed result still needs the command run and its output shown — just without prose around it.
- Do not use fast mode for destructive operations, security review, or architecture decisions; say so and stay on the normal path.
