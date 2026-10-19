# Loop — Self-Correction Loop

Run the same task repeatedly, improving on each pass by seeing your previous work in files and git history, until a stated completion promise is genuinely true. This skill starts a Ralph Loop.

## Usage
```
/loop <PROMPT> [--max-iterations N] [--completion-promise TEXT]
```

## Procedure
1. Start the loop with the ralph command: `/ralph <PROMPT> --max-iterations N --completion-promise TEXT` (installed by ralph into `.floyd/commands/`; see internal/agent/ralph/).
2. Work the task for this iteration. Leave evidence in files and commits — the next iteration reads those, not your memory.
3. Check state between passes with `/ralph-status`; stop early with `/ralph-cancel`.
4. The loop ends when: the completion promise is genuinely and verifiably true, max iterations are reached, or `/ralph-cancel` is called.

## Rules
- The completion promise may only be output when it is completely and unequivocally TRUE. Never output a false promise to escape the loop.
- Each iteration must change something real; repeating an unchanged failing approach is a three-strike stop-and-report (SSOT P2).
