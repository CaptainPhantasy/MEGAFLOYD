# Plan — Design Before Building

Turn a goal into a checked design before any code changes: read the terrain, choose the approach, write the plan, get sign-off, then implement against it.

## Usage
```
/plan <GOAL-OR-FEATURE-DESCRIPTION>
```

## Procedure
1. Read the governing context first: AGENTS.md, MEGAFLOYD.md, and any SSOT section touching the goal (the `view` tool). The plan obeys standing rules.
2. Survey the terrain: `project_map` for structure, `symbol_index`/`list_symbols` for the modules involved, `grep` for prior art and existing tests.
3. Write the plan to `.harness/plans/<slug>.md` with the `write` tool. Sections: objective (one sentence), constraints (from the rules read in step 1), design (chosen approach + one rejected alternative, with the reason), steps (ordered, each with its verification), risks (from /blast-radius thinking).
4. Track the steps with the `todos` tool as work begins.
5. Present the plan compactly and get the owner's go-ahead before mutating anything — planning is the cheap phase; changes without a plan are how rewrites happen.

## Rules
- A plan step without a verification command is a wish; every step names how it will be checked.
- Plans are living data: when reality contradicts the plan, update the plan file and say what changed.
