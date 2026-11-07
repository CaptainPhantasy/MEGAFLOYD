package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/agent/ralph"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/plan"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/shell"
)

type taskCoordinator interface {
	RuntimeTasks() []agent.RuntimeTask
	RuntimeTaskControl(string, string) error
	RuntimeStartSubagent(context.Context, string, string, string, string) (agent.RuntimeTask, error)
}

func (w *AppWorkspace) RalphGoal(ctx context.Context, sid, action, objective string) (*ralph.State, error) {
	if _, err := w.GetSession(ctx, sid); err != nil {
		return nil, err
	}
	loop := ralph.New(w.Config().Options.DataDirectory)
	state, err := loop.StatusChecked()
	if err != nil {
		return nil, err
	}
	switch action {
	case "", "status":
		if state != nil && state.SessionID != sid {
			return nil, nil
		}
		return state, nil
	case "start":
		if err := w.AgentReadyErr(); err != nil {
			return nil, err
		}
		if w.AgentIsSessionBusy(sid) {
			return nil, agent.ErrSessionBusy
		}
		return loop.Start(sid, strings.TrimSpace(objective), ralph.DefaultMaxIterations, "")
	case "cancel":
		if state == nil {
			return nil, nil
		}
		if state.SessionID != sid {
			return nil, errors.New("the active goal belongs to another session")
		}
		if _, err := loop.CancelSession(sid); err != nil {
			return nil, err
		}
		w.AgentCancel(sid)
		return nil, nil
	default:
		return nil, errors.New("goal action must be status, start, or cancel")
	}
}

func (w *ClientWorkspace) RalphGoal(ctx context.Context, sid, action, objective string) (*ralph.State, error) {
	return w.client.RuntimeRalphGoal(ctx, w.workspaceID(), sid, action, objective)
}

func (w *AppWorkspace) ForkSession(ctx context.Context, sid string) (result *session.ForkResult, err error) {
	copy := func() error {
		if err := w.app.Messages.FlushAll(ctx); err != nil {
			return err
		}
		result, err = session.Fork(ctx, w.app.Sessions, sid, -1)
		return err
	}
	if c, ok := w.app.AgentCoordinator.(interface {
		WithSessionIdle(string, func() error) error
	}); ok {
		err = c.WithSessionIdle(sid, copy)
	} else if w.AgentIsSessionBusy(sid) {
		err = agent.ErrSessionBusy
	} else {
		err = copy()
	}
	return
}

func (w *ClientWorkspace) ForkSession(ctx context.Context, sid string) (*session.ForkResult, error) {
	created, err := w.client.ForkSession(ctx, w.workspaceID(), sid)
	if err != nil {
		return nil, err
	}
	return &session.ForkResult{NewSession: protoToSession(*created), CopiedCount: max(0, int(created.MessageCount)-1)}, nil
}

func (w *AppWorkspace) ReloadConfig(ctx context.Context) error {
	apply := func() error {
		if err := w.store.ReloadFromDisk(ctx); err != nil {
			return err
		}
		w.app.Permissions.SetAdditionalDirs(w.Config().Options.AdditionalDirs)
		if cfg := w.Config(); cfg.Permissions != nil {
			if svc, ok := w.app.Permissions.(permission.RuntimeAllowlist); ok {
				svc.SetAllowedTools(cfg.Permissions.AllowedTools)
			}
			if svc, ok := w.app.Permissions.(permission.ModeService); ok {
				mode := cfg.Permissions.Mode
				if mode == "" {
					mode = permission.ModeManual
				}
				if err := svc.SetMode(mode); err != nil {
					return err
				}
			}
		}
		if w.app.AgentCoordinator != nil {
			if err := w.app.AgentCoordinator.UpdateModels(ctx); err != nil {
				return fmt.Errorf("configuration reloaded but model refresh failed: %w", err)
			}
		}
		return nil
	}
	if c, ok := w.app.AgentCoordinator.(interface{ RuntimeWithIdle(func() error) error }); ok {
		return c.RuntimeWithIdle(apply)
	}
	if w.AgentIsBusy() {
		return agent.ErrSessionBusy
	}
	return apply()
}

func (w *ClientWorkspace) ReloadConfig(ctx context.Context) error {
	if err := w.client.ReloadConfig(ctx, w.workspaceID()); err != nil {
		return err
	}
	updated, err := w.client.GetWorkspace(ctx, w.workspaceID())
	if err != nil {
		return fmt.Errorf("configuration reloaded but refresh failed: %w", err)
	}
	if updated.Config != nil {
		updated.Config.SetupAgents()
		updated.Config.NormalizeOptions()
	}
	w.mu.Lock()
	w.ws = *updated
	w.mu.Unlock()
	return nil
}

func (w *AppWorkspace) Tasks(ctx context.Context) ([]agent.RuntimeTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := []agent.RuntimeTask{}
	if c, ok := w.app.AgentCoordinator.(taskCoordinator); ok {
		out = append(out, c.RuntimeTasks()...)
	}
	manager := shell.GetBackgroundShellManager()
	for _, id := range manager.List() {
		bg, ok := manager.Get(id)
		if !ok || bg.WorkspaceDir == "" || filepath.Clean(bg.WorkspaceDir) != filepath.Clean(w.WorkingDir()) {
			continue
		}
		if _, err := w.GetSession(ctx, bg.SessionID); err != nil {
			continue
		}
		stdout, stderr, done, err := bg.GetOutput()
		output := stdout + stderr
		started := bg.StartedAt
		item := agent.RuntimeTask{ID: "bash:" + id, SessionID: bg.SessionID, Kind: "bash", Description: bg.Description, Command: bg.Command, Status: "running", CreatedAt: started, StartedAt: &started, OutputBytes: len(output), RunInBackground: true}
		if len(output) > 16384 {
			output = output[len(output)-16384:]
		}
		item.OutputPreview = output
		if done {
			item.Status = "completed"
			completed := bg.CompletedAt()
			item.CompletedAt = &completed
			if err != nil {
				item.Status = "failed"
			}
		}
		if bg.Cancelled() {
			item.Status = "cancelled"
		}
		out = append(out, item)
	}
	slices.SortFunc(out, func(a, b agent.RuntimeTask) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out, nil
}

func (w *ClientWorkspace) Tasks(ctx context.Context) ([]agent.RuntimeTask, error) {
	return w.client.RuntimeTasks(ctx, w.workspaceID())
}

func (w *AppWorkspace) Task(ctx context.Context, id, action string) (agent.RuntimeTask, error) {
	tasks, err := w.Tasks(ctx)
	if err != nil {
		return agent.RuntimeTask{}, err
	}
	for _, task := range tasks {
		if task.ID != id {
			continue
		}
		if action == "" {
			return task, nil
		}
		if action != "cancel" && action != "detach" {
			return agent.RuntimeTask{}, errors.New("unknown task action")
		}
		if task.Kind == "bash" {
			if action == "cancel" {
				err = shell.GetBackgroundShellManager().Cancel(strings.TrimPrefix(id, "bash:"))
			}
		} else if c, ok := w.app.AgentCoordinator.(taskCoordinator); ok {
			err = c.RuntimeTaskControl(id, action)
		} else {
			err = ErrAgentNotInitialized
		}
		if err != nil {
			return agent.RuntimeTask{}, err
		}
		return w.Task(ctx, id, "")
	}
	return agent.RuntimeTask{}, fmt.Errorf("task %q not found in this workspace", id)
}

func (w *ClientWorkspace) Task(ctx context.Context, id, action string) (agent.RuntimeTask, error) {
	return w.client.RuntimeTask(ctx, w.workspaceID(), id, action)
}

func (w *AppWorkspace) StartAgentTask(ctx context.Context, sid, prompt, name string) (agent.RuntimeTask, error) {
	c, ok := w.app.AgentCoordinator.(taskCoordinator)
	if !ok {
		return agent.RuntimeTask{}, ErrAgentNotInitialized
	}
	return c.RuntimeStartSubagent(ctx, sid, prompt, name, "swarm: "+name)
}

func (w *ClientWorkspace) StartAgentTask(ctx context.Context, sid, prompt, name string) (agent.RuntimeTask, error) {
	return w.client.RuntimeStartAgentTask(ctx, w.workspaceID(), sid, prompt, name)
}

func (w *AppWorkspace) Plan(ctx context.Context, sid string, in *proto.RuntimePlanRequest) (proto.RuntimePlan, error) {
	if _, err := w.GetSession(ctx, sid); err != nil {
		return proto.RuntimePlan{}, err
	}
	svc := plan.NewService(w.Config().Options.DataDirectory)
	if c, ok := w.app.AgentCoordinator.(interface{ RuntimePlanService() *plan.Service }); ok {
		svc = c.RuntimePlanService()
	}
	var status plan.Status
	var err error
	if in == nil {
		status, err = svc.Status(sid)
	} else {
		switch in.Action {
		case "enter":
			status, err = svc.Enter(sid)
			if errors.Is(err, plan.ErrAlreadyActive) {
				status, err = svc.Status(sid)
			}
		case "clear":
			err = svc.Clear(sid)
		case "update":
			status, err = svc.Update(sid, in.Content)
		case "approve":
			err = svc.Exit(sid)
		case "cancel":
			err = svc.Cancel(sid)
		default:
			err = errors.New("plan action must be enter, clear, update, approve, or cancel")
		}
	}
	if err == nil {
		status, err = svc.Status(sid)
	}
	return proto.RuntimePlan{Active: status.Active, ID: status.ID, Path: status.Path, Content: status.Content}, err
}

func (w *ClientWorkspace) Plan(ctx context.Context, sid string, in *proto.RuntimePlanRequest) (proto.RuntimePlan, error) {
	return w.client.RuntimePlan(ctx, w.workspaceID(), sid, in)
}
