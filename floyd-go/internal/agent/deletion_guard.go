package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/deletionguard"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/question"
	"github.com/legacy-ai/floyd/internal/shell"
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
	sessionID := tools.GetSessionFromContext(ctx)
	commands, dir, err := shellCommands(call)
	if err != nil || len(commands) == 0 {
		response, runErr := g.inner.Run(ctx, call)
		if sessionID != "" && (call.Name == tools.JobOutputToolName || call.Name == tools.JobKillToolName) {
			if err := g.reconcileBackground(sessionID); err != nil {
				return response, errors.Join(runErr, err)
			}
		}
		return response, runErr
	}
	if strings.TrimSpace(dir) == "" {
		dir = g.workingDir
	}
	classification := deletionguard.ClassifyAll(commands, dir)
	if !classification.IsDestructive() {
		return g.inner.Run(ctx, call)
	}

	if call.Name == tools.ParallelBashToolName && overlappingDeletionCommands(commands, dir) {
		return fantasy.NewTextErrorResponse("Permanent deletion is blocked because parallel commands have overlapping targets. Run each distinct target once."), nil
	}

	// Missing manager evidence remains uncertain, including after a restart.
	if err := g.reconcileBackground(sessionID); err != nil {
		return fantasy.NewTextErrorResponse("Permanent deletion is blocked because its session record cannot be read during background reconciliation: " + err.Error()), nil
	}
	decision := g.guard.Check(ctx, sessionID, call.ID, classification)
	if !decision.Allowed {
		return fantasy.NewTextErrorResponse(deletionDeniedMessage(decision)), nil
	}

	// Keep the pre-execution effects: truncation and overwrite classification
	// can change after the tool has modified the files.
	parts := make([]deletionguard.Classification, len(commands))
	for index, command := range commands {
		parts[index] = deletionguard.Classify(command, dir)
	}
	if decision.Record {
		if err := g.guard.BeginExecution(sessionID, classification, call.ID); err != nil {
			return fantasy.NewTextErrorResponse("Permanent deletion is blocked because its execution record could not be started: " + err.Error()), nil
		}
	}
	response, runErr := g.inner.Run(ctx, call)
	if decision.Record {
		if call.Name == tools.BashToolName && runErr == nil && !response.IsError {
			var metadata tools.BashResponseMetadata
			if json.Unmarshal([]byte(response.Metadata), &metadata) == nil && metadata.Background && metadata.ShellID != "" {
				job, ok := shell.GetBackgroundShellManager().Get(metadata.ShellID)
				if ok && job.SessionID == sessionID && job.Command == commands[0] && job.WorkingDir == dir && job.WorkspaceDir == g.workingDir && job.ExecutionID != "" {
					ref, err := g.guard.BindBackground(sessionID, classification, call.ID, deletionguard.BackgroundJob{
						SessionID: sessionID, ToolCallID: call.ID, ShellID: job.ID, ExecutionID: job.ExecutionID,
						Command: job.Command, WorkingDir: job.WorkingDir, WorkspaceDir: job.WorkspaceDir, Classification: classification,
					})
					if err != nil {
						return response, fmt.Errorf("record deletion background launch: %w", err)
					}
					// Capture the actual job pointer before job_kill/cleanup removes
					// its manager entry. Only its terminal channel proves it stopped.
					go func() {
						job.Wait()
						if err := g.reconcileBackgroundJob(ref, job); err != nil {
							slog.Warn("Deletion background outcome could not be recorded", "shell_id", ref.ShellID, "error", err)
						}
					}()
					return response, nil
				}
			}
		}

		completed, finished := deletionCompletion(call.Name, commands, response, runErr)
		var successfulTargets []string
		unsuccessfulTargets := make(map[string]bool)
		succeeded := make(map[int]bool, len(completed))
		for _, index := range completed {
			succeeded[index] = true
		}
		for index, part := range parts {
			if !part.IsDestructive() {
				continue
			}
			for _, target := range part.Targets {
				if succeeded[index] {
					successfulTargets = append(successfulTargets, target)
				} else {
					unsuccessfulTargets[target] = true
				}
			}
		}
		// An overlapping failed command makes that target's outcome uncertain.
		filtered := successfulTargets[:0]
		for _, target := range successfulTargets {
			if !unsuccessfulTargets[target] {
				filtered = append(filtered, target)
			}
		}
		if err := g.guard.RecordOutcome(sessionID, classification, call.ID, deletionguard.CanonicalTargets(filtered), !finished); err != nil {
			return response, errors.Join(runErr, fmt.Errorf("record deletion outcome: %w", err))
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

// overlappingDeletionCommands prevents concurrent commands from deleting the
// same item twice, including a child covered by a recursive parent deletion.
func overlappingDeletionCommands(commands []string, dir string) bool {
	var previous []string
	for _, command := range commands {
		part := deletionguard.Classify(command, dir)
		if !part.IsDestructive() {
			continue
		}
		for _, target := range part.Targets {
			for _, other := range previous {
				if targetContains(target, other) || targetContains(other, target) {
					return true
				}
			}
		}
		previous = append(previous, part.Targets...)
	}
	return false
}

func targetContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// deletionSucceeded requires explicit completion evidence from the bash tool.
// Background launches omit exit_code; fast completed background calls include it.
func deletionSucceeded(response fantasy.ToolResponse, err error) bool {
	completed, finished := deletionCompletion(tools.BashToolName, []string{""}, response, err)
	return finished && len(completed) == 1
}

// deletionCompletion validates each tool's actual metadata and returns only
// command indexes with demonstrated success. Missing evidence is unfinished.
func deletionCompletion(name string, commands []string, response fantasy.ToolResponse, err error) ([]int, bool) {
	if err != nil || response.IsError {
		return nil, false
	}
	switch name {
	case tools.BashToolName:
		var metadata struct {
			ExitCode *int   `json:"exit_code"`
			ShellID  string `json:"shell_id"`
		}
		if json.Unmarshal([]byte(response.Metadata), &metadata) != nil || metadata.ExitCode == nil || metadata.ShellID != "" {
			return nil, false
		}
		if *metadata.ExitCode == 0 {
			return []int{0}, true
		}
		return nil, true
	case tools.ParallelBashToolName:
		var metadata struct {
			Results []struct {
				Index    *int    `json:"index"`
				Command  *string `json:"command"`
				ExitCode *int    `json:"exit_code"`
				Error    string  `json:"error"`
			} `json:"results"`
		}
		if json.Unmarshal([]byte(response.Metadata), &metadata) != nil || len(commands) == 0 || len(metadata.Results) != len(commands) {
			return nil, false
		}
		seen := make(map[int]bool, len(commands))
		var completed []int
		finished := true
		for _, result := range metadata.Results {
			if result.Index == nil || result.Command == nil || result.ExitCode == nil {
				return nil, false
			}
			index := *result.Index
			if index < 0 || index >= len(commands) || seen[index] || *result.Command != commands[index] {
				return nil, false
			}
			seen[index] = true
			if result.Error != "" {
				finished = false
			}
			if *result.ExitCode == 0 && result.Error == "" {
				completed = append(completed, index)
			}
		}
		return completed, finished
	default:
		return nil, false
	}
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
	if permission.IsNonInteractive(ctx) {
		return false, permission.ErrInteractionRequired
	}
	if c.questions == nil {
		return false, errors.New("no question service is available")
	}
	answers, err := c.questions.Ask(ctx, question.Request{
		RequireLocalInput: true,
		SessionID:         req.SessionID,
		ToolCallID:        req.ToolCallID,
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

// Background reconciliation uses manager identity and its synchronized terminal
// output, never the tool response text or caller-supplied completion metadata.
func (g *deletionGuardTool) reconcileBackground(sessionID string) error {
	if err := g.guard.RetryBackgroundOutcomes(sessionID); err != nil {
		return err
	}
	jobs, err := g.guard.PendingBackground(sessionID)
	if err != nil {
		return err
	}
	for _, ref := range jobs {
		job, ok := shell.GetBackgroundShellManager().Get(ref.ShellID)
		if !ok {
			continue
		}
		if err := g.reconcileBackgroundJob(ref, job); err != nil {
			return err
		}
	}
	return nil
}

func (g *deletionGuardTool) reconcileBackgroundJob(ref deletionguard.BackgroundJob, job *shell.BackgroundShell) error {
	if ref.SessionID != job.SessionID || ref.ShellID != job.ID || ref.ExecutionID != job.ExecutionID || ref.Command != job.Command || ref.WorkingDir != job.WorkingDir || ref.WorkspaceDir != job.WorkspaceDir {
		return nil
	}
	_, _, done, err := job.GetOutput()
	if !done {
		return nil
	}
	result := deletionguard.BackgroundResult{Job: ref, Done: true, ExitCode: shell.ExitCode(err)}
	if err != nil {
		result.Error = err.Error()
	}
	return g.guard.ReconcileBackground(ref.SessionID, result)
}
