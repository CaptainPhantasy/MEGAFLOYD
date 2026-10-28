package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/deletionguard"
	"github.com/legacy-ai/floyd/internal/question"
)

// deletionGuardTool enforces the two-confirmation rule for permanent deletion
// before a shell command reaches the tool body. It is an additional check: the
// generic permission prompt still runs inside the tool, and configured user
// hooks still see the call. A command that only moves data is quarantine and
// passes through unchanged.
type deletionGuardTool struct {
	inner      fantasy.AgentTool
	guard      *deletionguard.Guard
	workingDir string
}

// wrapToolsWithDeletionGuard returns the tools wrapped in permanent-deletion
// enforcement. It returns the original slice when no guard is configured.
func wrapToolsWithDeletionGuard(
	input []fantasy.AgentTool,
	guard *deletionguard.Guard,
	workingDir string,
) []fantasy.AgentTool {
	if guard == nil {
		return input
	}
	out := make([]fantasy.AgentTool, len(input))
	for i, tool := range input {
		out[i] = &deletionGuardTool{inner: tool, guard: guard, workingDir: workingDir}
	}
	return out
}

func (g *deletionGuardTool) Info() fantasy.ToolInfo {
	return g.inner.Info()
}

func (g *deletionGuardTool) ProviderOptions() fantasy.ProviderOptions {
	return g.inner.ProviderOptions()
}

func (g *deletionGuardTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	g.inner.SetProviderOptions(opts)
}

func (g *deletionGuardTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	commands, dir, err := shellCommands(call)
	if err != nil || len(commands) == 0 {
		return g.inner.Run(ctx, call)
	}
	if strings.TrimSpace(dir) == "" {
		dir = g.workingDir
	}
	classification := deletionguard.ClassifyAll(commands, dir)
	if !classification.IsDestructive() {
		return g.inner.Run(ctx, call)
	}

	sessionID := tools.GetSessionFromContext(ctx)
	decision := g.guard.Check(ctx, sessionID, call.ID, classification)
	if !decision.Allowed {
		return fantasy.NewTextErrorResponse(deletionDeniedMessage(decision)), nil
	}

	response, runErr := g.inner.Run(ctx, call)
	if decision.Record && deletionSucceeded(response, runErr) {
		if recordErr := g.guard.RecordExecution(sessionID, classification, call.ID); recordErr != nil {
			slog.Warn("Failed to record the confirmed deletion", "tool", call.Name, "error", recordErr)
		}
	}
	return response, runErr
}

// shellCommands returns the shell commands a tool call would run and the
// directory they run in.
func shellCommands(call fantasy.ToolCall) ([]string, string, error) {
	switch call.Name {
	case tools.BashToolName:
		var params tools.BashParams
		if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
			return nil, "", err
		}
		return []string{params.Command}, params.WorkingDir, nil
	case tools.ParallelBashToolName:
		var params tools.ParallelBashParams
		if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
			return nil, "", err
		}
		commands := make([]string, 0, len(params.Commands))
		for _, command := range params.Commands {
			commands = append(commands, command.Command)
		}
		return commands, params.WorkingDir, nil
	default:
		return nil, "", nil
	}
}

// deletionSucceeded reports a completed command. The bash tool reports a
// non-zero exit in metadata rather than as an error.
func deletionSucceeded(response fantasy.ToolResponse, err error) bool {
	if err != nil || response.IsError {
		return false
	}
	var metadata struct {
		ExitCode *int `json:"exit_code"`
	}
	if jsonErr := json.Unmarshal([]byte(response.Metadata), &metadata); jsonErr == nil &&
		metadata.ExitCode != nil && *metadata.ExitCode != 0 {
		return false
	}
	return true
}

// deletionDeniedMessage explains the denial and the recoverable alternative.
func deletionDeniedMessage(decision deletionguard.Decision) string {
	message := decision.Reason
	if message == "" {
		message = "Permanent deletion is blocked."
	}
	return message + " (evidence: effect=" + decision.Effect.String() +
		" targets=" + strings.Join(decision.Targets, ",") + ")"
}

// questionConfirmer asks the human through the interactive question service.
// The answer is the only source of a confirmation entry.
type questionConfirmer struct {
	questions question.Service
}

func (c questionConfirmer) Confirm(ctx context.Context, req deletionguard.ConfirmationRequest) (bool, error) {
	if c.questions == nil {
		return false, errors.New("no question service is available")
	}
	answers, err := c.questions.Ask(ctx, question.Request{
		SessionID:  req.SessionID,
		ToolCallID: req.ToolCallID,
		Questions: []question.Question{{
			ID:          "deletion-confirmation-" + strconv.Itoa(req.Stage),
			Type:        question.TypeYesNo,
			Label:       "Permanent deletion: confirmation " + strconv.Itoa(req.Stage) + " of 2",
			Text:        req.Question,
			Description: req.Detail,
		}},
	})
	if err != nil {
		return false, err
	}
	for _, answer := range answers {
		if answer.Yes != nil {
			return *answer.Yes, nil
		}
	}
	return false, errors.New("the confirmation answer was not a yes or no")
}
