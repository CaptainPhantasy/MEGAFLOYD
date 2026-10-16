# Prompt pack — canonical location, interface, and checksums

**Status:** Current — verified 2026-10-16 against `floyd-go` at the merged commit and against the
Go source of truth (`floyd-v6/src`, HEAD `57ba00a8`).
**Canonical pack:** `docs/prompt-pack/templates/`
**Derived copy:** `floyd-go/internal/agent/templates/` (must never be edited directly)

Every fact below was read out of the Go code, not restated from `PLAN.md`.

---

## 1. Why there are two copies, and which one to edit

The Go program embeds its templates from *inside* its own module. `//go:embed` cannot reach a parent
directory, so the Go tree must keep a physical copy:

```
floyd-go/internal/agent/prompts.go:14   //go:embed templates/coder.md.tpl
floyd-go/internal/agent/prompts.go:17   //go:embed templates/task.md.tpl
floyd-go/internal/agent/prompts.go:20   //go:embed templates/initialize.md.tpl
floyd-go/internal/agent/agent_tool.go:15        //go:embed templates/agent_tool.md
floyd-go/internal/agent/agent.go:65             //go:embed templates/title.md
floyd-go/internal/agent/agent.go:68             //go:embed templates/summary.md
floyd-go/internal/agent/agentic_fetch_tool.go:19  //go:embed templates/agentic_fetch.md
floyd-go/internal/agent/agentic_fetch_tool.go:50  //go:embed templates/agentic_fetch_prompt.md.tpl
```

So the copy is derived, not duplicated by accident. The rule is:

- **Edit `docs/prompt-pack/templates/` only.**
- Run `node scripts/verify-prompt-pack.mjs --sync` to push the canonical files into the Go tree.
- Run `node scripts/verify-prompt-pack.mjs` (no flag) to prove every copy matches. It exits non-zero
  on drift, so it can be wired into a pre-commit hook or CI.

The guard also checks the unversioned third copy at `../floyd-merge/prompt-pack/templates/` and warns
if it has drifted. That copy is outside git and is not authoritative.

## 2. The real template interface

The Go assembler passes one struct into every template —
`floyd-go/internal/agent/prompt/prompt.go:31`:

```go
type PromptDat struct {
    Provider           string
    Model              string
    Config             config.Config
    WorkingDir         string
    IsGitRepo          bool
    Platform           string
    Date               string
    GitStatus          string
    ContextFiles       []ContextFile
    GlobalContextFiles []ContextFile
    AvailSkillXML      string
    AgentName          string
}

type ContextFile struct {
    Path    string
    Content string
}
```

Plus two nested reads: `.Config.LSP` (len used in `coder.md.tpl`) and `.Config.Options.InitializeAs`
(used in `initialize.md.tpl`).

> **Correction to `PLAN.md`.** The P1 variable list in the plan — `CWD`, `ENV`, `GIT_STATUS`,
> `GIT_BRANCH`, `RECENT_COMMITS`, `FLOYD_MD`, `SUPERCACHE`, `SKILLS`, `MCP_PROMPTS`, `IDENTITY` —
> does not exist. None of those names appears anywhere in the Go code. `GIT_BRANCH`, `RECENT_COMMITS`,
> `FLOYD_MD`, `SUPERCACHE`, `MCP_PROMPTS` and `IDENTITY` have no implementation at all, and the four
> real names that are close (`WorkingDir`, `GitStatus`, `AvailSkillXML`, `AgentName`) are spelled
> differently. P2 must be written against the struct above.

## 3. Placeholders each template actually uses

Extracted by scanning the files for `{{` … `}}`.

| Template | Placeholders | Lines | Consumed by | Overridable |
|---|---|---|---|---|
| `coder.md.tpl` | `AgentName`, `Date`, `Platform`, `WorkingDir`, `GitStatus`, `IsGitRepo`, `ContextFiles` (`.Path`, `.Content`), `GlobalContextFiles`, `AvailSkillXML`, `Config.LSP` | 434 | `prompts.go:14` → `NewPrompt("coder", …)` | yes |
| `task.md.tpl` | `AgentName`, `Date`, `Platform`, `WorkingDir`, `IsGitRepo` | 15 | `prompts.go:17` → `NewPrompt("task", …)` | yes |
| `initialize.md.tpl` | `Config.Options.InitializeAs` | 29 | `prompts.go:20` → `NewPrompt("initialize", …)` | yes |
| `agentic_fetch_prompt.md.tpl` | `AgentName`, `Date`, `Platform`, `WorkingDir` | 76 | `agentic_fetch_tool.go:50` → `NewPrompt("agentic_fetch", …)` | no |
| `summary.md` | none | 48 | `agent.go:68` | no |
| `title.md` | none | 11 | `agent.go:65` | no |
| `agent_tool.md` | none | 1 | `agent_tool.go:15` | no |
| `agentic_fetch.md` | none | 1 | `agentic_fetch_tool.go:19` | no |

## 4. The override path (a plug point `PLAN.md` never mentions)

`floyd-go/internal/agent/prompts.go:23`:

```go
func promptTemplate(name string, embedded []byte) string {
    overridePath := filepath.Join(filepath.Dir(config.GlobalConfig()), "prompts", name+".md.tpl")
    if b, err := os.ReadFile(overridePath); err == nil {
        slog.Info("Using external prompt template", "name", name, "path", overridePath)
        return string(b)
    }
    return string(embedded)
}
```

A file at `<global config dir>/prompts/<name>.md.tpl` replaces the embedded template at runtime. This
is why prompt overrides are user-visible behaviour, and why the pack could never be edited in place
without a decision.

Two things follow, and both are findings rather than plans:

- Only three templates are routed through `promptTemplate()`: `coder`, `task`, `initialize`. The other
  five (`agent_tool`, `agentic_fetch`, `agentic_fetch_prompt`, `title`, `summary`) are embedded and
  **cannot** be overridden from disk. The override surface is narrower than the pack.
- The override path depends on `config.GlobalConfig()`, so the pack's directory layout in the merged
  build has to match that call, not the other way round.

## 5. Checksums

`sha256` of the canonical files at the time of writing. `node scripts/verify-prompt-pack.mjs` is the
authority; if this table and the guard disagree, the guard wins.

| File | sha256 |
|---|---|
| `agent_tool.md` | `9188c56e38be2ee8c392d9f94655bb2c4b8bfa85e27f3aa5ce2807cb6a5280b5` |
| `agentic_fetch.md` | `ecd21b6368cb9a16a786a8c2252f088bcca736564fcdfcf3312e0ac079a6583a` |
| `agentic_fetch_prompt.md.tpl` | `f26d06cb4e02906772e870e06dcaab495f0ef47b134dbda79b1fde99065eb53e` |
| `coder.md.tpl` | `d674c2dd729625f34531650bc61aeea2b009ffe1c8e3a4e17bc8855143e42c07` |
| `initialize.md.tpl` | `f435ad8b7e128e525a4772f9f435ad0005a79c5d7d25c767135741d68348905e` |
| `summary.md` | `58a9daa183a36c734214deffaa2ea30cd3e5f8d224ac7c95fe6fba2dff5544ad` |
| `task.md.tpl` | `983a1d940495cc8a15bd3174b68b93fdea973ca630f939caa44579c40d8d75a8` |
| `title.md` | `edb4d5fcc117b415651cac2585624ae6de956c1087823bbfc305dc85d849a465` |

Assembly logic, pinned to the Go source:

| File | sha256 |
|---|---|
| `prompt.go.reference` | `7322e5a2fd8ab7db63a0fb0827c130dd000783242bf2a6b2e9b08f9c672efe62` |
| `floyd-go/internal/agent/prompt/prompt.go` | `7322e5a2fd8ab7db63a0fb0827c130dd000783242bf2a6b2e9b08f9c672efe62` |

## 6. Provenance

| Item | Value |
|---|---|
| Source of truth | `/Users/floyd/floyd-v6/src` at HEAD `57ba00a8` |
| Web source | `/Users/floyd/floyd-v6/KimiSource/kimi-code` (Floyd Code v2.0.2) |
| Merged into | `/Users/floyd/MEGAFLOYD` @ `e33af3f` |
| Verification | `diff -rq` of every file against the source — templates and `prompt.go` are byte-identical |
| Guard | `scripts/verify-prompt-pack.mjs` — PASS on 2026-10-16 |
