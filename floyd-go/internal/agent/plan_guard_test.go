package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/plan"
	"github.com/stretchr/testify/require"
)

type planGuardRecordingTool struct {
	name string
	ran  *bool
}

func (t planGuardRecordingTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{Name: t.name, Description: "test tool"}
}

func (t planGuardRecordingTool) ProviderOptions() fantasy.ProviderOptions {
	return fantasy.ProviderOptions{}
}

func (t planGuardRecordingTool) SetProviderOptions(fantasy.ProviderOptions) {}

func (t planGuardRecordingTool) Run(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
	*t.ran = true
	return fantasy.NewTextResponse("ran"), nil
}

func planTestContext(sessionID string) context.Context {
	return context.WithValue(context.Background(), tools.SessionIDContextKey, sessionID)
}

func TestPlanGuardMatrix(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	plans := plan.NewService(t.TempDir())
	entered, err := plans.Enter("session-1")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(entered.Path, []byte("# Plan"), 0o644))

	sourcePath := workingDir + "/main.go"
	cases := []struct {
		name       string
		toolName   string
		input      map[string]string
		wantDenied bool
	}{
		{
			name:     "write to plan file is allowed",
			toolName: tools.WriteToolName,
			input:    map[string]string{"file_path": entered.Path, "content": "# Plan"},
		},
		{
			name:     "edit to plan file is allowed",
			toolName: tools.EditToolName,
			input:    map[string]string{"file_path": entered.Path},
		},
		{
			name:       "write to source file is denied",
			toolName:   tools.WriteToolName,
			input:      map[string]string{"file_path": sourcePath, "content": "package main"},
			wantDenied: true,
		},
		{
			name:       "edit to source file is denied",
			toolName:   tools.EditToolName,
			input:      map[string]string{"file_path": "main.go"},
			wantDenied: true,
		},
		{
			name:       "multiedit to source file is denied",
			toolName:   tools.MultiEditToolName,
			input:      map[string]string{"file_path": "main.go"},
			wantDenied: true,
		},
		{
			name:       "missing file path is denied",
			toolName:   tools.WriteToolName,
			input:      map[string]string{},
			wantDenied: true,
		},
		{
			name:       "job kill is denied",
			toolName:   tools.JobKillToolName,
			input:      map[string]string{"job_id": "job-1"},
			wantDenied: true,
		},
		{
			name:       "cron create is denied",
			toolName:   "cron_create",
			input:      map[string]string{"schedule": "* * * * *"},
			wantDenied: true,
		},
		{
			name:       "cron delete is denied",
			toolName:   "cron_delete",
			input:      map[string]string{"id": "cron-1"},
			wantDenied: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ran := false
			inner := planGuardRecordingTool{name: tc.toolName, ran: &ran}
			wrapped := wrapToolsWithPlanGuard([]fantasy.AgentTool{inner}, plans, nil, workingDir)[0]

			input, err := json.Marshal(tc.input)
			require.NoError(t, err)
			resp, err := wrapped.Run(planTestContext("session-1"), fantasy.ToolCall{
				ID:    "call-1",
				Name:  tc.toolName,
				Input: string(input),
			})
			require.NoError(t, err)

			if tc.wantDenied {
				require.True(t, resp.IsError, "expected denial, got %q", resp.Content)
				require.False(t, ran, "denied tool must not execute")
				require.Contains(t, strings.ToLower(resp.Content), "plan mode")
				if tc.toolName == tools.WriteToolName || tc.toolName == tools.EditToolName || tc.toolName == tools.MultiEditToolName {
					require.Contains(t, resp.Content, entered.Path)
				}
			} else {
				require.False(t, resp.IsError, resp.Content)
				require.True(t, ran, "allowed tool must execute")
			}
		})
	}
}

func TestPlanGuardAllowsWritesAfterExit(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	plans := plan.NewService(t.TempDir())
	_, err := plans.Enter("session-1")
	require.NoError(t, err)
	require.NoError(t, plans.Exit("session-1"))

	ran := false
	inner := planGuardRecordingTool{name: tools.WriteToolName, ran: &ran}
	wrapped := wrapToolsWithPlanGuard([]fantasy.AgentTool{inner}, plans, nil, workingDir)[0]
	resp, err := wrapped.Run(planTestContext("session-1"), fantasy.ToolCall{
		ID:    "call-1",
		Name:  tools.WriteToolName,
		Input: `{"file_path":"main.go","content":"package main"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)
	require.True(t, ran)
}

func TestPlanGuardExitApproval(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		grant       bool
		skip        bool
		wantRan     bool
		wantContain string
	}{
		{name: "yolo skips the prompt", skip: true, wantRan: true},
		{name: "approved", grant: true, wantRan: true},
		{name: "denied", grant: false, wantRan: false, wantContain: "Plan rejected by user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workingDir := t.TempDir()
			plans := plan.NewService(t.TempDir())
			entered, err := plans.Enter("session-1")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(entered.Path, []byte("# Plan\n\nDo the thing."), 0o644))

			perms := permission.NewPermissionService(workingDir, tc.skip, nil)
			if !tc.skip {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				events := perms.Subscribe(ctx)
				go func() {
					select {
					case event := <-events:
						if tc.grant {
							perms.Grant(event.Payload)
						} else {
							perms.Deny(event.Payload)
						}
					case <-ctx.Done():
					}
				}()
			}

			ran := false
			inner := planGuardRecordingTool{name: tools.ExitPlanModeToolName, ran: &ran}
			wrapped := wrapToolsWithPlanGuard([]fantasy.AgentTool{inner}, plans, perms, workingDir)[0]
			resp, err := wrapped.Run(planTestContext("session-1"), fantasy.ToolCall{
				ID:    "call-1",
				Name:  tools.ExitPlanModeToolName,
				Input: `{}`,
			})
			require.NoError(t, err)
			require.Equal(t, tc.wantRan, ran)

			if tc.wantContain != "" {
				require.True(t, resp.IsError)
				require.Contains(t, resp.Content, tc.wantContain)
				status, statusErr := plans.Status("session-1")
				require.NoError(t, statusErr)
				require.True(t, status.Active, "a rejected plan must stay active")
			}
		})
	}
}

func TestPlanGuardExitReviewDescriptionIncludesPlan(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	plans := plan.NewService(t.TempDir())
	entered, err := plans.Enter("session-1")
	require.NoError(t, err)
	planText := "# Plan\n\n1. Ship it."
	require.NoError(t, os.WriteFile(entered.Path, []byte(planText), 0o644))

	perms := permission.NewPermissionService(workingDir, false, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events := perms.Subscribe(ctx)
	requests := make(chan permission.PermissionRequest, 1)
	go func() {
		select {
		case event := <-events:
			requests <- event.Payload
			perms.Grant(event.Payload)
		case <-ctx.Done():
		}
	}()

	ran := false
	inner := planGuardRecordingTool{name: tools.ExitPlanModeToolName, ran: &ran}
	wrapped := wrapToolsWithPlanGuard([]fantasy.AgentTool{inner}, plans, perms, workingDir)[0]
	_, err = wrapped.Run(planTestContext("session-1"), fantasy.ToolCall{
		ID:    "call-1",
		Name:  tools.ExitPlanModeToolName,
		Input: `{}`,
	})
	require.NoError(t, err)

	select {
	case request := <-requests:
		require.True(t, ran)
		require.Equal(t, tools.ExitPlanModeToolName, request.ToolName)
		require.Contains(t, request.Description, planText)
		require.True(t, strings.Contains(request.Description, entered.Path))
	case <-ctx.Done():
		t.Fatal("approval request was not published")
	}
}

func TestPlanGuardEmptyPlanDoesNotPrompt(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	plans := plan.NewService(t.TempDir())
	_, err := plans.Enter("session-1")
	require.NoError(t, err)

	perms := permission.NewPermissionService(workingDir, false, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events := perms.Subscribe(ctx)

	ran := false
	inner := planGuardRecordingTool{name: tools.ExitPlanModeToolName, ran: &ran}
	wrapped := wrapToolsWithPlanGuard([]fantasy.AgentTool{inner}, plans, perms, workingDir)[0]
	_, err = wrapped.Run(planTestContext("session-1"), fantasy.ToolCall{
		ID:    "call-1",
		Name:  tools.ExitPlanModeToolName,
		Input: `{}`,
	})
	require.NoError(t, err)
	require.True(t, ran, "the tool must run so it can report the empty plan")

	select {
	case <-events:
		t.Fatal("an empty plan must not trigger an approval prompt")
	default:
	}
}

func TestPlanGuardRejectsUnprovenMutationsDespitePermissionGrants(t *testing.T) {
	plans := plan.NewService(t.TempDir())
	_, err := plans.Enter("plan")
	require.NoError(t, err)
	for _, name := range []string{"bash", "parallel_bash", "apply_patch", "agent", "mcp_fixture_mutate", "archive_query", "read_resource", "read_resources", "list_resources"} {
		t.Run(name, func(t *testing.T) {
			ran := false
			perms := permission.NewPermissionService(t.TempDir(), true, []string{name})
			tool := wrapToolsWithPlanGuard([]fantasy.AgentTool{planGuardRecordingTool{name: name, ran: &ran}}, plans, perms, t.TempDir())[0]
			resp, err := tool.Run(permission.WithNonInteractive(planTestContext("plan")), fantasy.ToolCall{Name: name, Input: `{"command":"printf plan > marker.txt","description":"fixture"}`})
			require.NoError(t, err)
			require.True(t, resp.IsError)
			require.False(t, ran)
			require.Contains(t, resp.Content, "plan mode")
		})
	}
	ran := false
	view := wrapToolsWithPlanGuard([]fantasy.AgentTool{planGuardRecordingTool{name: "view", ran: &ran}}, plans, nil, t.TempDir())[0]
	resp, err := view.Run(planTestContext("plan"), fantasy.ToolCall{Name: "view"})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.True(t, ran)
}

func TestPlanGuardAllowsRegisteredInspectionTools(t *testing.T) {
	t.Parallel()
	plans := plan.NewService(t.TempDir())
	_, err := plans.Enter("inspection-plan")
	require.NoError(t, err)
	inspectionTools := []fantasy.AgentTool{
		tools.NewArchiveQueryTool(nil, t.TempDir()),
		tools.NewReadMCPResourceTool(nil, nil),
		tools.NewListMCPResourcesTool(nil, nil),
	}
	for _, registered := range inspectionTools {
		name := registered.Info().Name
		t.Run(name, func(t *testing.T) {
			ran := false
			inner := planGuardRecordingTool{name: name, ran: &ran}
			wrapped := wrapToolsWithPlanGuard([]fantasy.AgentTool{inner}, plans, nil, t.TempDir())[0]
			response, err := wrapped.Run(planTestContext("inspection-plan"), fantasy.ToolCall{Name: name, Input: `{}`})
			require.NoError(t, err)
			require.False(t, response.IsError, response.Content)
			require.True(t, ran, "registered inspection tool must execute in plan mode")
		})
	}
}
