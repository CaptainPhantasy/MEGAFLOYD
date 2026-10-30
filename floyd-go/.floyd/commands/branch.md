# Branch — Start A Parallel Session Branch

Open a second line of work next to the current one: a new session branch sharing the current conversation history, optionally with its own git branch or worktree. Work continues independently in both.

## Aliases
`/fork` — same skill, same job. Branch and fork are synonyms here (SSOT §3E combining rule). Not to be confused with /rewind, which abandons the recent messages rather than keeping both lines.

## Usage
```
/branch <NAME>
/fork <NAME>
```

## Procedure
1. Create the code isolation if needed: `git branch <name>` for a light split, or a git worktree (`git worktree add ../<repo>-<name> -b <name>`) when both branches must be checked out at once.
2. Fork the current session at the latest message via the session fork mechanism (internal/session/fork.go; the TUI built-in fork action). The branch session inherits full history.
3. Name the new session so it is findable: rename via `megafloyd session rename <id> <name>` or the built-in rename action.
4. Report both lines of work: original session id + branch session id, and which git branch/worktree each uses.
5. Keep working in the branch for the new line; the original session is untouched.

## Rules
- Branch when both lines stay alive. Use /checkpoint instead when you only want a return point.
