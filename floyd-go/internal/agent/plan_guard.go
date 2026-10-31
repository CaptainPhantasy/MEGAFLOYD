package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/filepathext"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/plan"
)

// planGuardTool enforces plan mode before delegating to the inner tool.
// It mirrors the reference harness: while a plan is active, write tools
// may touch only the plan file, background-task termination and cron
// mutation are denied, and exiting plan mode needs approval unless
// permission prompts are disabled (yolo/auto).
type planGuardTool struct {
	inner       fantasy.AgentTool
	plans       *plan.Service
	permissions permission.Service
	workingDir  string
}

// wrapToolsWithPlanGuard returns the tools wrapped in plan-mode
// enforcement. It returns the original slice when no plan service is
// configured.
func wrapToolsWithPlanGuard(
	tools []fantasy.AgentTool,
	plans *plan.Service,
	permissions permission.Service,
	workingDir string,
) []fantasy.AgentTool {
	if plans == nil {
		return tools
	}
	out := make([]fantasy.AgentTool, len(tools))
	for i, tool := range tools {
		out[i] = &planGuardTool{
			inner:       tool,
			plans:       plans,
			permissions: permissions,
			workingDir:  workingDir,
		}
	}
	return out
}

func (g *planGuardTool) Info() fantasy.ToolInfo {
	return g.inner.Info()
}

func (g *planGuardTool) ProviderOptions() fantasy.ProviderOptions {
	return g.inner.ProviderOptions()
}

func (g *planGuardTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	g.inner.SetProviderOptions(opts)
}

func (g *planGuardTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	sessionID := tools.GetSessionFromContext(ctx)
	if sessionID == "" {
		return g.inner.Run(ctx, call)
	}

	status, err := g.plans.Status(sessionID)
	if err != nil {
		slog.Warn("Failed to read plan state; blocking tool call", "tool", call.Name, "error", err)
		return fantasy.NewTextErrorResponse("Plan mode state cannot be verified: " + err.Error()), nil
	}
	if !status.Active {
		return g.inner.Run(ctx, call)
	}

	if call.Name == tools.ExitPlanModeToolName {
		// Do not ask the user to approve a plan that does not exist.
		// The tool returns the empty-plan error instead.
		if strings.TrimSpace(status.Content) == "" {
			return g.inner.Run(ctx, call)
		}
		if g.permissions != nil && !g.permissions.SkipRequests() {
			approved, err := g.permissions.Request(ctx, permission.CreatePermissionRequest{
				SessionID:   sessionID,
				ToolCallID:  call.ID,
				ToolName:    tools.ExitPlanModeToolName,
				Action:      "exit_plan_mode",
				Path:        status.Path,
				Description: planReviewDescription(status),
				Params:      map[string]string{"plan_path": status.Path},
			})
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !approved {
				resp := fantasy.NewTextErrorResponse("Plan rejected by user. Plan mode remains active.")
				resp.StopTurn = true
				return resp, nil
			}
		}
		return g.inner.Run(ctx, call)
	}

	switch call.Name {
	case tools.WriteToolName, tools.EditToolName, tools.MultiEditToolName:
		if !writesOnlyPlanFile(call, status.Path, g.workingDir) {
			return fantasy.NewTextErrorResponse(planModeWriteDeniedMessage(status.Path)), nil
		}
	case tools.RunSkillToolName:
		return fantasy.NewTextErrorResponse("Executable skills are unavailable in plan mode. Call exit_plan_mode before running a skill."), nil
	case tools.JobKillToolName:
		return fantasy.NewTextErrorResponse(
			"job_kill is not available in plan mode. Call exit_plan_mode to exit plan mode before stopping a background task.",
		), nil
	case "cron_create", "cron_delete":
		return fantasy.NewTextErrorResponse(
			call.Name + " is not available in plan mode because it would mutate scheduled work that runs after plan exit. Call exit_plan_mode first.",
		), nil
	default:
		// A generic permission grant does not prove a tool is read-only. Shell,
		// delegated agents and MCP tools can mutate through many indirect paths.
		// Planning uses the known inspection tools and the current plan file.
		if !planInspectionTool(call.Name) {
			return fantasy.NewTextErrorResponse(call.Name + " is unavailable in plan mode because its read-only behavior is not guaranteed. Use the inspection tools or exit plan mode first."), nil
		}
	}

	return g.inner.Run(ctx, call)
}

func planInspectionTool(name string) bool {
	switch name {
	case "view", "ls", "glob", "grep", "lsp_references", "lsp_definition", "lsp_symbols", "lsp_diagnostics", "lsp_call_hierarchy", "sourcegraph", "fetch", "web_fetch", "web_search", "agentic_fetch", "floyd_info", "floyd_logs", "hook_explain", "hook_trace", "archive_query", "tool_search", "search_tools", "question", "enter_plan_mode", "job_output", "todo_write", "todos", "read_resource", "read_resources", "list_resources":
		return true
	default:
		return false
	}
}

func planReviewDescription(status plan.Status) string {
	if status.Path == "" {
		return "Review and approve the plan, then exit plan mode."
	}
	return "Review and approve the plan at " + status.Path + ":\n\n" + status.Content
}

func planModeWriteDeniedMessage(planFilePath string) string {
	if planFilePath == "" {
		planFilePath = "(no plan file selected yet)"
	}
	return "Plan mode is active. You may only write to the current plan file: " + planFilePath + ". " +
		"Call exit_plan_mode to exit plan mode before editing other files."
}

func writesOnlyPlanFile(call fantasy.ToolCall, planFilePath, workingDir string) bool {
	paths := writePaths(call)
	if len(paths) == 0 || planFilePath == "" {
		return false
	}
	for _, path := range paths {
		if !samePath(path, planFilePath, workingDir) {
			return false
		}
	}
	return true
}

func writePaths(call fantasy.ToolCall) []string {
	var params struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return nil
	}
	if strings.TrimSpace(params.FilePath) == "" {
		return nil
	}
	return []string{params.FilePath}
}

func samePath(one, two, workingDir string) bool {
	return normalizePath(one, workingDir) == normalizePath(two, workingDir)
}

func normalizePath(path, workingDir string) string {
	path = filepathext.SmartJoin(workingDir, path)
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}
