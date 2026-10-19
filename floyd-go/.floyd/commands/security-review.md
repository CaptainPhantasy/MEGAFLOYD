# Security-Review — Threat-Model The Change

Review a change or subsystem for security weaknesses with a STRIDE-shaped pass: spoofing, tampering, repudiation, information disclosure, denial of service, elevation of privilege. Findings are file:line specific and ranked.

## Usage
```
/security-review [path-or-diff]
```

## Procedure
1. Scope: `get_active_diff` for pending work, or the given path; map trust boundaries with `project_map` (where untrusted input meets privileged code).
2. Walk the STRIDE categories over the diff. In this codebase, priority surfaces:
   - Permission paths: internal/permission, allow/deny lists, `--yolo` — never weaken checks (owner directive).
   - Config-as-code: floydrc and hooks run arbitrary shell — review any new hook or config builtin like code review.
   - Secrets: `grep` for key patterns (api[_-]?key, token, secret, bearer) in the diff; secrets arrive via env or config, never committed or logged.
   - Injection: untrusted content (fetch results, MCP data) flowing into `bash`/`eval` contexts.
3. Enumerate abuse paths with `failure_mode_enumerator` on each trust boundary; rate by blast radius (/blast-radius).
4. Report each finding: category, severity (critical/high/medium/low), file:line, the abuse path in one sentence, the smallest hardening change. No findings = say so with the scope checked.

## Rules
- Read the code; never report a category as reviewed without having looked at its surface in the diff.
- Findings block merge in this repo; fixes or explicit owner acceptance resolve them.
