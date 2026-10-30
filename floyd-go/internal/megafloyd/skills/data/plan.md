# Plan — Design Before Building

Turn the requested goal into a checked design: inspect the project, choose the approach, and write the plan. Follow the user's request about planning and implementation.

## Usage
```
/plan <GOAL-OR-FEATURE-DESCRIPTION>
```

## Procedure
1. Read the project MegaFloyd.md using case-insensitive filename discovery and the relevant project SSOT (the `view` tool). MegaFloyd.md supplies policy; the SSOT supplies facts and user decisions.
2. Survey the terrain: `project_map` for structure, `symbol_index`/`list_symbols` for the modules involved, `grep` for prior art and existing tests.
3. Write the plan to the SSOT's recorded plans folder, or `docs/plans/<slug>.md` when none exists with the `write` tool. Sections: objective (one sentence), constraints (from the rules read in step 1), design (chosen approach + one rejected alternative, with the reason), steps (ordered, each with its verification), risks (from /blast-radius thinking).
4. Track the steps with the `todos` tool as work begins.
5. Present the plan compactly. If implementation is already authorized, continue it. If the user requested a plan for approval, present the completed plan for that approval. Ask only for remaining material decisions under MegaFloyd.md section 3.

## Rules
- A plan step without a verification command is a wish; every step names how it will be checked.
- Plans are living data: when reality contradicts the plan, update the plan file and say what changed.
