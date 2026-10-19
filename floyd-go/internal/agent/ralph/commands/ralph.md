# Ralph Loop — Start

Start a Ralph Loop in the current session. The same prompt is fed to you repeatedly after each turn completes. You see your previous work in files and git history, enabling iterative self-improvement.

## Usage
```
/ralph <PROMPT> [--max-iterations N] [--completion-promise TEXT]
```

## How It Works
1. You receive the prompt and work on the task
2. When your turn ends, the harness checks the ralph loop state
3. If not complete, the SAME PROMPT is fed back to you
4. You see your previous work in the files — iterate and improve
5. Loop ends when: completion promise is detected, max iterations reached, or `/ralph-cancel` is called

## Options
- `--max-iterations N` — Stop after N iterations (default: unlimited)
- `--completion-promise TEXT` — Stop when you output `<promise>TEXT</promise>`

## Completion Promise Rules
CRITICAL: If a completion promise is set, you may ONLY output it when the statement is completely and unequivocally TRUE. Do not output false promises to escape the loop. The loop is designed to continue until genuine completion.

To signal completion:
```
<promise>YOUR_PROMISE_TEXT</promise>
```

## Examples
```
/ralph Fix all failing tests --completion-promise "ALL TESTS PASS" --max-iterations 20
/ralph Refactor the cache layer to use LRU eviction --max-iterations 10
/ralph Add comprehensive error handling to the API layer --completion-promise "DONE"
```
