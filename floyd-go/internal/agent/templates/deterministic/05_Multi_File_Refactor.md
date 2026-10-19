# 05 — Multi-File Refactor

```md
Refactor for maintainability without changing behavior.

## Target
<modules/files>

## Rules
- Preserve external behavior.
- Keep edits atomic and reversible.
- No unrelated cleanup.

## Required steps
1) Snapshot current behavior and interfaces
2) Apply refactor in small commits/patches
3) Run regression checks after each cluster
4) Document invariants preserved

## Deliverables
- Refactor map (old -> new structure)
- Risk list
- Verification receipts
```