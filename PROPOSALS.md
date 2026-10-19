# PROPOSALS — ALL APPROVED 2026-10-19 (now SSOT §5.4; kept for reference)

Nothing here is law. Each item enters SSOT.md only after the owner says yes.

P1. Evidence tags (VERIFIED / HYPOTHESIS / LEAD) on every state claim.
    Why: prevents unverified work being reported as done.
P2. Three-strike rule: three failed attempts on one defect = stop and report.
    Why: prevents infinite retry loops burning time.
P3. SSOT + progress updated at every wave boundary.
    Why: any session can pick the project up cold.
P4. Wave gating: build+test+lint green before the next wave starts.
    Why: catches breakage early, keeps the merge history bisectable.
P5. Per-feature worktrees for parallel agents.
    Why: no race conditions on shared checkouts.
P6. Command name lists (extracted from source, not yet owner-confirmed):
    - 43 floyd7 slash commands: /yolo /auto /permission /settings /plan
      /model /secondary-model /effort /provider /btw /help /new /sessions
      /tasks /mcp /plugins /add-dir /experiments /reload /reload-tui
      /compact /goal /init /fork /title /usage /status /feedback /undo
      /editor /theme /login /logout /export-md /export-debug-zip /copy /web
      /remote-control /desktop /exit /version /swarm
    - Superfloyd manual aliases: /ralph /ralph-status /ralph-cancel
      /selfheal /parallel-bash /workflow /project-map /symbol-index
      /transcript-export /archive-query /scratchpad /context-status /budget
      /scoreboard /doctor /prompt /ai /stash /ghost /vision /lab /resilience
    - CLI subcommands: run serve login logout models projects session stats
      logs schema update-providers ai codebase doctor exec fileops prompt
      scoreboard resilience
    Why: these are extracted from the real sources; owner confirms the list
    before it becomes the shipped surface.
