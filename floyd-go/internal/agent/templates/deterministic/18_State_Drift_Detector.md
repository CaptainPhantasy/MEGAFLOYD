# 18 — State Drift Detector

```md
Use this when behavior differs between environments/branches/builds.

## Compare targets
- Target A: <env/branch/build>
- Target B: <env/branch/build>

## Drift checklist
1) Binary/version source
2) Config values (model/context/flags)
3) Runtime dependencies/providers
4) Key behavior checkpoints
5) Error signatures

## Output requirements
- Drift table: category -> A -> B -> impact
- Proven drifts only (with evidence)
- Unknowns listed separately

## Gate
No root-cause claim unless a drift is linked to symptom by evidence.
```
