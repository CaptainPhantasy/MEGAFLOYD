# Effort — Set The Reasoning Effort Dial

Choose how hard the model thinks for upcoming work: low for mechanical edits, medium for normal coding, high for architecture and debugging. One dial, applied to the active model.

## Usage
```
/effort [low|medium|high]
```

## Procedure
1. With an argument, set the level: it maps to the `reasoning_effort` config option (enum: low, medium, high — internal/config/config.go) on the active model. Apply via the model settings in the command palette (ctrl+p → model) or the config file, then confirm.
2. Without an argument, report the current effort level and the active model (the `floyd_info` tool shows model config).
3. Pick the level for the task at hand:
   - low: file moves, renames, formatting, mechanical replacements.
   - medium: everyday feature work, tests, reviews.
   - high: architecture, security, tricky debugging, anything with cascade risk.

## Rules
- Effort changes the thinking depth, not the output style. For the speed preset (terse output plus low effort plus small model) use /fast.
- Report the change in one line: model, old level, new level.
