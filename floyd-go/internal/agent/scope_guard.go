package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/jobs"
	"github.com/legacy-ai/floyd/internal/plan"
)

// scopeGuardTool enforces the active job contract's declared scope before a
// write reaches the filesystem. It mirrors planGuardTool: path-only tools are
// checked against a durable allowlist, and the call is vetoed with an explicit
// message instead of being executed. Plan mode stays authoritative: while a
// plan is active the plan guard's narrower allowlist decides first, so the
// plan file remains writable even when it sits outside the job scope.
type scopeGuardTool struct {
	inner      fantasy.AgentTool
	dataDir    string
	plans      *plan.Service
	workingDir string
}

// wrapToolsWithScopeGuard returns the tools wrapped in active-job scope
// enforcement. It returns the original slice when no data directory is
// configured. Callers install it inside the plan guard so plan mode keeps
// precedence.
func wrapToolsWithScopeGuard(
	tools []fantasy.AgentTool,
	dataDir string,
	plans *plan.Service,
	workingDir string,
) []fantasy.AgentTool {
	if strings.TrimSpace(dataDir) == "" {
		return tools
	}
	out := make([]fantasy.AgentTool, len(tools))
	for i, tool := range tools {
		out[i] = &scopeGuardTool{
			inner:      tool,
			dataDir:    dataDir,
			plans:      plans,
			workingDir: workingDir,
		}
	}
	return out
}

func (g *scopeGuardTool) Info() fantasy.ToolInfo {
	return g.inner.Info()
}

func (g *scopeGuardTool) ProviderOptions() fantasy.ProviderOptions {
	return g.inner.ProviderOptions()
}

func (g *scopeGuardTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	g.inner.SetProviderOptions(opts)
}

func (g *scopeGuardTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	switch call.Name {
	case tools.WriteToolName, tools.EditToolName, tools.MultiEditToolName:
	default:
		return g.inner.Run(ctx, call)
	}

	if g.planActive(ctx) {
		// Plan mode already restricts writes to the plan file; do not add a
		// second, conflicting allowlist underneath it.
		return g.inner.Run(ctx, call)
	}

	job, err := jobs.Active(g.dataDir)
	if err != nil {
		slog.Warn("Failed to read active job scope; allowing tool call", "tool", call.Name, "error", err)
		return g.inner.Run(ctx, call)
	}
	if job == nil {
		return g.inner.Run(ctx, call)
	}

	paths := writePaths(call)
	if len(paths) == 0 {
		return fantasy.NewTextErrorResponse(scopeMissingPathDeniedMessage(job)), nil
	}
	for _, path := range paths {
		if jobs.ScopeAllows(path, g.workingDir, job.Scope) {
			continue
		}
		return fantasy.NewTextErrorResponse(scopeWriteDeniedMessage(job, path)), nil
	}
	return g.inner.Run(ctx, call)
}

func (g *scopeGuardTool) planActive(ctx context.Context) bool {
	if g.plans == nil {
		return false
	}
	sessionID := tools.GetSessionFromContext(ctx)
	if sessionID == "" {
		return false
	}
	status, err := g.plans.Status(sessionID)
	if err != nil {
		return false
	}
	return status.Active
}

func scopeWriteDeniedMessage(job *jobs.Job, path string) string {
	return fmt.Sprintf(
		"Job %s declared scope: %s. %s is outside that scope, so the harness vetoed the write. "+
			"Revert the path or update the contract scope deliberately before retrying. "+
			"(evidence: active_job=%s scope=%s denied_path=%s)",
		job.ID, scopePaths(job.Scope), path, job.ID, strings.Join(job.Scope, ","), path,
	)
}

func scopeMissingPathDeniedMessage(job *jobs.Job) string {
	return fmt.Sprintf(
		"Job %s declared scope: %s. The tool call has no file_path, so the harness cannot prove the write is inside scope. "+
			"Pass a file path inside the declared scope. (evidence: active_job=%s scope=%s)",
		job.ID, scopePaths(job.Scope), job.ID, strings.Join(job.Scope, ","),
	)
}

func scopePaths(scope []string) string {
	if len(scope) == 0 {
		return "(none declared)"
	}
	return strings.Join(scope, ", ")
}
