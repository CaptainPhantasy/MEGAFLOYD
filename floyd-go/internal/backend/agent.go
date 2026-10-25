package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/agent/notify"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/pubsub"
	"github.com/legacy-ai/floyd/internal/shell"
)

// SendMessage validates and accepts a prompt for the workspace's agent,
// then dispatches the run on a goroutine bound to the workspace context
// and returns immediately. It does not wait for the LLM turn to
// complete: the run's lifetime is owned by the workspace, not by the
// caller. Errors from the dispatched run reach observers through the
// agent event channels (a notify.TypeAgentError notification), not
// through this return value.
//
// SendMessage returns synchronously when the request cannot be accepted:
// ErrWorkspaceNotFound if the workspace is missing, ErrAgentNotInitialized
// if its coordinator is nil, the structural validation errors from
// agent.ValidateCall (ErrEmptyPrompt, ErrSessionMissing) when the prompt
// or session is missing, and ErrWorkspaceClosing if the workspace is
// being torn down.
func (b *Backend) SendMessage(workspaceID string, msg proto.AgentMessage) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if err := agent.ValidateCall(agent.SessionAgentCall{
		SessionID:   msg.SessionID,
		Prompt:      msg.Prompt,
		Attachments: proto.AttachmentsToMessage(msg.Attachments),
	}); err != nil {
		return err
	}

	ws.runMu.Lock()
	if ws.closing {
		ws.runMu.Unlock()
		return ErrWorkspaceClosing
	}
	if ws.AgentCoordinator == nil {
		ws.runMu.Unlock()
		return ErrAgentNotInitialized
	}
	accept := ws.AgentCoordinator.BeginAccepted(msg.SessionID)
	ws.runWG.Add(1)
	ws.runMu.Unlock()

	go b.runAgent(ws, msg, accept)
	return nil
}

// runAgent executes an accepted agent run for the workspace. It owns the
// accept reservation (releasing it on return) and the runWG ticket added
// by SendMessage. The run is bound to the workspace context so its
// lifetime is independent of any client's HTTP request.
//
// On a non-cancel error it surfaces the failure to observers via a
// notify.TypeAgentError notification (lossy, best-effort). That alone is
// not a reliable terminal signal: the agent-event fan-in uses lossy
// subscribers, so a `floyd run` caller blocking on its RunID could hang
// if the event is dropped. To guarantee termination, when msg.RunID is
// non-empty and the coordinator did not already publish the run's
// authoritative terminal RunComplete (e.g. the error was returned before
// sessionAgent.Run executed, such as a readyWg or UpdateModels failure),
// runAgent emits an errored RunComplete on the must-deliver
// runCompletions broker so the waiter observes a deterministic terminal
// event. context.Canceled is expected (sessionAgent.Run already
// publishes the cancelled terminal marker) and produces no error
// terminal event.
//
// When msg.RunID is non-empty it is attached to the context via
// agent.WithRunID so the coordinator can stamp the terminal
// notify.RunComplete event with that correlator. A run-complete marker
// is also attached so the coordinator can report whether it published
// the terminal event, letting runAgent avoid a duplicate fallback.
func (b *Backend) runAgent(ws *Workspace, msg proto.AgentMessage, accept *agent.AcceptedRun) {
	defer ws.runWG.Done()
	defer accept.Close()

	ctx := ws.ctx
	if msg.RunID != "" {
		ctx = agent.WithRunID(ctx, msg.RunID)
	}
	ctx = agent.WithRunCompleteMarker(ctx)

	_, err := ws.AgentCoordinator.RunAccepted(ctx, accept, msg.SessionID, msg.Prompt, proto.AttachmentsToMessage(msg.Attachments)...)
	if err == nil || errors.Is(err, context.Canceled) {
		// A clean completion (err == nil, no cancel raced the context)
		// may trigger the heartbeat auto-continue so the session does
		// not sit idle between turns.
		if err == nil && ctx.Err() == nil {
			b.maybeHeartbeat(ws, msg)
		}
		return
	}

	ws.AgentNotifications().Publish(pubsub.CreatedEvent, notify.Notification{
		SessionID: msg.SessionID,
		RunID:     msg.RunID,
		Type:      notify.TypeAgentError,
		Message:   err.Error(),
	})

	// Reliable terminal fallback. Only needed when a RunID waiter
	// exists and the coordinator has not already emitted the run's
	// terminal RunComplete; otherwise this would be a duplicate.
	if msg.RunID == "" || agent.RunCompletePublished(ctx) {
		return
	}
	if rc := ws.RunCompletions(); rc != nil {
		rc.PublishMustDeliver(ctx, pubsub.UpdatedEvent, notify.RunComplete{
			SessionID: msg.SessionID,
			RunID:     msg.RunID,
			Error:     err.Error(),
		})
	}
}

// maybeHeartbeat schedules the synthetic continuation prompt for a
// session whose run just completed cleanly, keeping the agent working
// instead of idling between turns.
//
// Guards: heartbeat disabled in config → no-op; workspace closing →
// no-op; consecutive-continuation cap reached → reset and stop. A real
// user message (msg.Heartbeat == false) resets the counter.
func (b *Backend) maybeHeartbeat(ws *Workspace, msg proto.AgentMessage) {
	if ws.Cfg == nil || ws.Cfg.Config() == nil {
		return
	}
	hb := ws.Cfg.Config().Heartbeat
	if hb == nil || !hb.Enabled {
		return
	}

	ws.heartbeatMu.Lock()
	if ws.heartbeatCounts == nil {
		ws.heartbeatCounts = make(map[string]int)
	}
	if !msg.Heartbeat {
		ws.heartbeatCounts[msg.SessionID] = 0
	}
	if ws.heartbeatCounts[msg.SessionID] >= hb.EffectiveMaxConsecutive() {
		ws.heartbeatCounts[msg.SessionID] = 0
		ws.heartbeatMu.Unlock()
		slog.Warn("Heartbeat cap reached; stopping auto-continue", "session", msg.SessionID, "max", hb.EffectiveMaxConsecutive())
		return
	}
	ws.heartbeatCounts[msg.SessionID]++
	ws.heartbeatMu.Unlock()

	delay := hb.Cooldown()
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ws.ctx.Done():
			return
		case <-timer.C:
		}

		ws.runMu.Lock()
		closing := ws.closing
		ws.runMu.Unlock()
		if closing {
			return
		}

		// SendMessage re-validates and dispatches through the normal
		// path; the Heartbeat marker keeps the consecutive counter
		// counting and lets clients label the synthetic prompt.
		if err := b.SendMessage(ws.ID, proto.AgentMessage{
			SessionID: msg.SessionID,
			Prompt:    hb.EffectivePrompt(),
			Heartbeat: true,
		}); err != nil {
			slog.Warn("Heartbeat continuation failed to dispatch", "session", msg.SessionID, "error", err)
		}
	}()
}

// GetAgentInfo returns the agent's model and busy status.
func (b *Backend) GetAgentInfo(workspaceID string) (proto.AgentInfo, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return proto.AgentInfo{}, err
	}

	ws.runMu.Lock()
	defer ws.runMu.Unlock()
	var agentInfo proto.AgentInfo
	if ws.AgentCoordinator != nil {
		m := ws.AgentCoordinator.Model()
		agentInfo = proto.AgentInfo{
			Model:    m.CatwalkCfg,
			ModelCfg: m.ModelCfg,
			IsBusy:   ws.AgentCoordinator.IsBusy(),
			IsReady:  true,
		}
	}
	return agentInfo, nil
}

// InitAgent initializes the workspace's coder agent once. Reattaching clients
// share the installed coordinator, including its active runs and pending tools.
func (b *Backend) InitAgent(ctx context.Context, workspaceID string, interactive bool) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	ws.runMu.Lock()
	defer ws.runMu.Unlock()
	if ws.closing {
		return ErrWorkspaceClosing
	}
	if ws.AgentCoordinator != nil {
		return nil
	}
	if interactive {
		return ws.InitCoderAgent(ctx)
	}
	return ws.InitCoderAgentNonInteractive(ctx)
}

// UpdateAgent reloads the agent model configuration.
func (b *Backend) UpdateAgent(ctx context.Context, workspaceID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	return ws.UpdateAgentModel(ctx)
}

// CancelSession cancels an ongoing agent operation for the given
// session.
func (b *Backend) CancelSession(workspaceID, sessionID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if ws.AgentCoordinator != nil {
		ws.AgentCoordinator.Cancel(sessionID)
	}
	return nil
}

// SummarizeSession triggers a session summarization.
func (b *Backend) SummarizeSession(ctx context.Context, workspaceID, sessionID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if ws.AgentCoordinator == nil {
		return ErrAgentNotInitialized
	}

	return ws.AgentCoordinator.Summarize(ctx, sessionID)
}

// QueuedPrompts returns the number of queued prompts for the session.
func (b *Backend) QueuedPrompts(workspaceID, sessionID string) (int, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return 0, err
	}

	if ws.AgentCoordinator == nil {
		return 0, nil
	}

	return ws.AgentCoordinator.QueuedPrompts(sessionID), nil
}

// ClearQueue clears the prompt queue for the session.
func (b *Backend) ClearQueue(workspaceID, sessionID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if ws.AgentCoordinator != nil {
		ws.AgentCoordinator.ClearQueue(sessionID)
	}
	return nil
}

// QueuedPromptsList returns the list of queued prompt strings for a
// session.
func (b *Backend) QueuedPromptsList(workspaceID, sessionID string) ([]string, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}

	if ws.AgentCoordinator == nil {
		return nil, nil
	}

	return ws.AgentCoordinator.QueuedPromptsList(sessionID), nil
}

// GetDefaultSmallModel returns the default small model for a provider.
func (b *Backend) GetDefaultSmallModel(workspaceID, providerID string) (config.SelectedModel, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return config.SelectedModel{}, err
	}

	return ws.GetDefaultSmallModel(providerID), nil
}

// RunShellCommand runs a shell command in the workspace directory and
// persists the command + output as a user message in the session.
func (b *Backend) RunShellCommand(ctx context.Context, workspaceID string, req proto.ShellCommandRequest) (proto.ShellCommandResponse, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return proto.ShellCommandResponse{}, err
	}

	ws.runMu.Lock()
	if ws.closing {
		ws.runMu.Unlock()
		return proto.ShellCommandResponse{}, ErrWorkspaceClosing
	}
	ws.runWG.Add(1)
	ws.runMu.Unlock()
	defer ws.runWG.Done()
	if ws.ctx != nil {
		runCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(ws.ctx, cancel)
		defer stop()
		defer cancel()
		ctx = runCtx
	}

	var persist shell.PersistFunc
	if req.SessionID != "" {
		persist = func(cmd, output string, exitCode int) error {
			return shell.PersistOutput(ctx, ws.Messages, req.SessionID, cmd, output, exitCode)
		}
	}

	run := func(runCtx context.Context, command string) (shell.CaptureResult, error) {
		return shell.RunAndPersist(runCtx, shell.RunOptions{Command: command, Cwd: ws.Path, Env: append(os.Environ(), ws.Env...), TermWidth: req.TermWidth}, persist)
	}
	var result shell.CaptureResult
	if ws.Hooks == nil {
		result, err = run(ctx, req.Command)
	} else {
		input, _ := json.Marshal(map[string]string{"command": req.Command})
		call := fantasy.ToolCall{ID: uuid.NewString(), Name: "bash", Input: string(input)}
		hookCtx := hookruntime.WithRun(ctx, uuid.NewString())
		executed := false
		response, runErr := ws.Hooks.ExecuteTool(hookCtx, req.SessionID, call, func(innerCtx context.Context, actual fantasy.ToolCall) (fantasy.ToolResponse, error) {
			var params struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal([]byte(actual.Input), &params); err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("invalid rewritten shell input: %w", err)
			}
			executed = true
			result, err = run(innerCtx, params.Command)
			metadata, _ := json.Marshal(map[string]int{"exit_code": result.ExitCode})
			return fantasy.ToolResponse{Content: result.Output, IsError: result.ExitCode != 0, Metadata: string(metadata)}, err
		})
		err = runErr
		if err == nil && !executed {
			return proto.ShellCommandResponse{}, errors.New(response.Content)
		}
		if err == nil && response.IsError && result.ExitCode == 0 {
			result.ExitCode = 1
		}
		result.Output = response.Content
	}
	if err != nil {
		return proto.ShellCommandResponse{}, err
	}
	return proto.ShellCommandResponse{Output: result.Output, ExitCode: result.ExitCode}, nil
}
