# Promote — Ship The Winning Variant

Take the winner from /arena or /compare and make it the real thing: checkpoint first, apply the winning diff, clean up the losers, verify, and update the records.

## Usage
```
/promote <WINNER-REF>
```

## Procedure
1. Checkpoint the current state (/checkpoint pre-promote) — promotion must be reversible.
2. Apply the winner: from an arena worktree, bring the diff over (`git diff` in the winner worktree, then apply in the main tree with `apply_patch` or `git apply`); for a chosen file pair, `smart_replace` or the `edit` tool for targeted promotion.
3. Clean the arena: remove contender worktrees (`git worktree remove`), delete superseded artifacts. Quarantine, do not raw-delete, anything the owner might want (/owner standing rule).
4. Verify the promoted state: run the tests that judged the arena (/test), plus /invariants. A promotion that fails its own acceptance criteria is reverted to the checkpoint — say so plainly.
5. Update the records: the plan file or checkpoint notes name the winner and why; commit with a message that references the compared alternatives.

## Rules
- Promotion without verification is a guess wearing a winner's badge — step 4 is mandatory.
- If promotion reveals the judge was wrong, stop, restore the checkpoint, and report — do not patch forward silently.
