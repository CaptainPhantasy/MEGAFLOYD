Use this tool proactively when you're about to start a non-trivial implementation task. Getting user sign-off on your approach via exit_plan_mode before writing code prevents wasted effort.

Use it when ANY of these conditions apply:

1. New Feature Implementation - e.g. "Add a caching layer to the API"
2. Multiple Valid Approaches - e.g. "Optimize database queries" (indexing vs rewrite vs caching)
3. Code Modifications - e.g. "Refactor auth module to support OAuth"
4. Architectural Decisions - e.g. "Add WebSocket support"
5. Multi-File Changes - involves more than 2-3 files
6. Unclear Requirements - need exploration to understand scope
7. User Preferences Matter - if user input would materially change the implementation approach

Permission mode notes:
- enter_plan_mode enters plan mode automatically without an approval prompt in all permission modes.
- In yolo and manual modes, exit_plan_mode still presents the plan to the user for approval.
- In auto permission mode, exit_plan_mode exits plan mode without asking the user.
- Use enter_plan_mode only when planning itself adds value.

When NOT to use:
- Single-line or few-line fixes (typos, obvious bugs, small tweaks)
- User gave very specific, detailed instructions
- Pure research/exploration tasks

Once you are in plan mode, a reminder walks you through the workflow (explore → design → write the plan file → exit_plan_mode) and enforces read-only access.
