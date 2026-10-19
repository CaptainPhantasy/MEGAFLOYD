# Deterministic Prompt Framework (FLOYD)

This folder is a visible, human-readable prompt pack for running real tasks with benchmark-level discipline.

## What this pack contains

- 15 high-value templates (planning, execution, debugging, verification, handoff)
- deterministic output contracts (proof over prose)
- explicit completion gates to prevent "planned but skipped"

## How to use

1. Open the template file matching your task.
2. Copy/paste into a new session as your first instruction.
3. Fill placeholders (`<like_this>`).
4. Require the model to follow the completion gate exactly.

## Mode selector (fast)

- **A. Debug**: errors, crashes, regressions
- **B. Orchestration**: multi-file implementation/refactor
- **C. Exploration**: ideas, options, tradeoffs
- **D. Analysis**: logs, exports, traces

## Anti-skip contract (recommended add-on)

Paste this block under any template:

```md
## Anti-Skip Contract
- Do not mark a section complete without evidence.
- If blocked, return exactly: blocker, impact, one next diagnostic.
- Final output MUST include a completeness matrix (requested item -> status -> evidence).
- If any item lacks evidence, final status = INCOMPLETE.
```

## File index

1. `01_Deterministic_Task_Kickoff.md`
2. `02_Complex_Implementation_Orchestration.md`
3. `03_Failure_Driven_Debugging.md`
4. `04_Regression_Bug_Hunt.md`
5. `05_Multi_File_Refactor.md`
6. `06_Test_First_Fix.md`
7. `07_Stability_Hardening.md`
8. `08_Release_Readiness_Audit.md`
9. `09_Commit_Preparation_and_Intent_Check.md`
10. `10_PR_Summary_and_TestPlan.md`
11. `11_Context_Window_Protection.md`
12. `12_Incident_Response_Production.md`
13. `13_Architecture_Decision_Record.md`
14. `14_Safe_Exploration_Tradeoff.md`
15. `15_Handoff_Continuity.md`
16. `16_10X_Execution_Proof_Enforcer.md` (**10X multiplier**)
17. `17_10X_Reality_Benchmark_Parity.md` (**10X multiplier**)
18. `18_State_Drift_Detector.md`
19. `19_Zero_Loss_Context_Handoff.md`
20. `20_Surgical_Change_MinRisk.md`

## Why these 5 were added

- **16 (10X):** Prevents “planned but skipped” by enforcing proof per requested item.
- **17 (10X):** Converts benchmark-grade behavior into real-world execution discipline.
- **18:** Isolates environment/build/config drift that causes inconsistent behavior.
- **19:** Prevents context-loss damage during session transitions.
- **20:** Enforces smallest-safe-diff changes in fragile code paths.

---

Created for high-reliability real-world execution, not demo behavior.
