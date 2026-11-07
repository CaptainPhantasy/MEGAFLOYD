package backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/goal"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/plan"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/shell"
)

type runtimeQueueCoordinator interface {
	RuntimeQueue(string) ([]agent.RuntimePrompt, *agent.RuntimePrompt, error)
	RuntimeEditQueue(string, string, string, bool) error
	RuntimeSteerQueue(string, []string) error
	RuntimeClearQueue(string) error
}
type runtimeTaskCoordinator interface {
	RuntimeTasks() []agent.RuntimeTask
	RuntimeTaskControl(string, string) error
}
type runtimeProfileCoordinator interface {
	RuntimeProfiles() ([]agent.RuntimeProfile, error)
	RuntimeSaveProfile(string, agent.RuntimeProfile) (agent.RuntimeProfile, error)
	RuntimeStartSubagent(context.Context, string, string, string, string) (agent.RuntimeTask, error)
}
type RuntimeQueue struct {
	Items  []agent.RuntimePrompt `json:"items"`
	Active *agent.RuntimePrompt  `json:"active"`
	Count  int                   `json:"count"`
}

func (b *Backend) runtimeSession(ctx context.Context, wid, sid string) (*Workspace, error) {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return nil, err
	}
	if _, err = ws.Sessions.Get(ctx, sid); err != nil {
		return nil, err
	}
	return ws, nil
}

func (b *Backend) RuntimeQueue(ctx context.Context, wid, sid string) (RuntimeQueue, error) {
	ws, err := b.runtimeSession(ctx, wid, sid)
	if err != nil {
		return RuntimeQueue{}, err
	}
	c, ok := ws.AgentCoordinator.(runtimeQueueCoordinator)
	if !ok {
		return RuntimeQueue{}, ErrAgentNotInitialized
	}
	items, active, err := c.RuntimeQueue(sid)
	return RuntimeQueue{Items: items, Active: active, Count: len(items)}, err
}

func (b *Backend) RuntimeEditQueue(ctx context.Context, wid, sid, id, prompt string, remove bool) error {
	ws, err := b.runtimeSession(ctx, wid, sid)
	if err != nil {
		return err
	}
	c, ok := ws.AgentCoordinator.(runtimeQueueCoordinator)
	if !ok {
		return ErrAgentNotInitialized
	}
	return c.RuntimeEditQueue(sid, id, prompt, remove)
}

func (b *Backend) RuntimeSteerQueue(ctx context.Context, wid, sid string, ids []string) error {
	ws, err := b.runtimeSession(ctx, wid, sid)
	if err != nil {
		return err
	}
	c, ok := ws.AgentCoordinator.(runtimeQueueCoordinator)
	if !ok {
		return ErrAgentNotInitialized
	}
	return c.RuntimeSteerQueue(sid, ids)
}

func (b *Backend) RuntimePlan(ctx context.Context, wid, sid string, in *proto.RuntimePlanRequest) (proto.RuntimePlan, error) {
	ws, err := b.runtimeSession(ctx, wid, sid)
	if err != nil {
		return proto.RuntimePlan{}, err
	}
	svc := plan.NewService(ws.Cfg.Config().Options.DataDirectory)
	if c, ok := ws.AgentCoordinator.(interface{ RuntimePlanService() *plan.Service }); ok {
		svc = c.RuntimePlanService()
	}
	var status plan.Status
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

func (b *Backend) RuntimeSwarm(ctx context.Context, wid, sid string, in *proto.RuntimeSwarmRequest) (agent.SwarmState, error) {
	ws, err := b.runtimeSession(ctx, wid, sid)
	if err != nil {
		return agent.SwarmState{}, err
	}
	if in == nil {
		return agent.RuntimeSwarm(ctx, ws.Sessions, sid)
	}
	state, err := agent.SetRuntimeSwarm(ctx, ws.Sessions, sid, in.Active, in.MaxParallel)
	if err == nil {
		if c, ok := ws.AgentCoordinator.(interface{ RuntimeSwarmChanged() }); ok {
			c.RuntimeSwarmChanged()
		}
	}
	return state, err
}

func (b *Backend) RuntimeGoal(ctx context.Context, wid, sid string, in *goal.Input) (*goal.State, error) {
	ws, err := b.runtimeSession(ctx, wid, sid)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return goal.Get(ctx, ws.Sessions, sid)
	}
	value, err := goal.Configure(ctx, ws.Sessions, sid, *in)
	if err != nil {
		return nil, err
	}
	if in.Control == "pause" || in.Control == "cancel" {
		if err = b.CancelSession(wid, sid); err != nil {
			return nil, err
		}
	}
	if !in.DeferStart && (strings.TrimSpace(in.Objective) != "" || in.Control == "resume") {
		err = b.SendMessage(wid, proto.AgentMessage{SessionID: sid, Prompt: "Work toward the active goal: " + value.Objective, RunID: uuid.NewString()})
		if err != nil {
			_, storeErr := goal.Configure(context.WithoutCancel(ctx), ws.Sessions, sid, goal.Input{Control: "blocked", Reason: "could not start goal: " + err.Error()})
			return nil, errors.Join(err, storeErr)
		}
	}
	return value, nil
}

func backgroundTask(bg *shell.BackgroundShell) agent.RuntimeTask {
	stdout, stderr, done, err := bg.GetOutput()
	output := stdout + stderr
	started := bg.StartedAt
	t := agent.RuntimeTask{ID: "bash:" + bg.ID, SessionID: bg.SessionID, Kind: "bash", Description: bg.Description, Command: bg.Command, Status: "running", CreatedAt: started, StartedAt: &started, OutputBytes: len(output), RunInBackground: true}
	if len(output) > 16384 {
		output = output[len(output)-16384:]
	}
	t.OutputPreview = output
	if done {
		t.Status = "completed"
		completed := bg.CompletedAt()
		t.CompletedAt = &completed
		if err != nil {
			t.Status = "failed"
		}
	}
	if bg.Cancelled() {
		t.Status = "cancelled"
	}
	return t
}

func (b *Backend) RuntimeTasks(ctx context.Context, wid string) ([]agent.RuntimeTask, error) {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return nil, err
	}
	out := []agent.RuntimeTask{}
	if c, ok := ws.AgentCoordinator.(runtimeTaskCoordinator); ok {
		out = append(out, c.RuntimeTasks()...)
	}
	manager := shell.GetBackgroundShellManager()
	for _, id := range manager.List() {
		bg, ok := manager.Get(id)
		if !ok || bg.WorkspaceDir == "" || filepath.Clean(bg.WorkspaceDir) != filepath.Clean(ws.Path) {
			continue
		}
		if _, err = ws.Sessions.Get(ctx, bg.SessionID); err != nil {
			continue
		}
		out = append(out, backgroundTask(bg))
	}
	slices.SortFunc(out, func(a, b agent.RuntimeTask) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out, nil
}

func (b *Backend) RuntimeTask(ctx context.Context, wid, id, action string) (agent.RuntimeTask, error) {
	items, err := b.RuntimeTasks(ctx, wid)
	if err != nil {
		return agent.RuntimeTask{}, err
	}
	var found *agent.RuntimeTask
	for i := range items {
		if items[i].ID == id {
			found = &items[i]
			break
		}
	}
	if found == nil {
		return agent.RuntimeTask{}, fmt.Errorf("task %q not found in this workspace", id)
	}
	if action != "" {
		ws, _ := b.GetWorkspace(wid)
		switch found.Kind {
		case "bash":
			if action == "cancel" {
				err = shell.GetBackgroundShellManager().Cancel(strings.TrimPrefix(id, "bash:"))
			} else if action != "detach" {
				err = errors.New("unknown task action")
			}
		case "subagent":
			c, ok := ws.AgentCoordinator.(runtimeTaskCoordinator)
			if !ok {
				return agent.RuntimeTask{}, ErrAgentNotInitialized
			}
			err = c.RuntimeTaskControl(id, action)
		}
		if err != nil {
			return agent.RuntimeTask{}, err
		}
		return b.RuntimeTask(ctx, wid, id, "")
	}
	return *found, nil
}

func (b *Backend) RuntimeProfiles(wid string) ([]agent.RuntimeProfile, error) {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return nil, err
	}
	c, ok := ws.AgentCoordinator.(runtimeProfileCoordinator)
	if !ok {
		return nil, ErrAgentNotInitialized
	}
	return c.RuntimeProfiles()
}

func (b *Backend) RuntimeSaveProfile(wid, id string, in agent.RuntimeProfile) (agent.RuntimeProfile, error) {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return in, err
	}
	c, ok := ws.AgentCoordinator.(runtimeProfileCoordinator)
	if !ok {
		return in, ErrAgentNotInitialized
	}
	return c.RuntimeSaveProfile(id, in)
}

func (b *Backend) RuntimeStartSubagent(ctx context.Context, wid, sid string, in proto.RuntimeSubagentRequest) (agent.RuntimeTask, error) {
	ws, err := b.runtimeSession(ctx, wid, sid)
	if err != nil {
		return agent.RuntimeTask{}, err
	}
	ws.runMu.Lock()
	defer ws.runMu.Unlock()
	if ws.closing {
		return agent.RuntimeTask{}, ErrWorkspaceClosing
	}
	c, ok := ws.AgentCoordinator.(runtimeProfileCoordinator)
	if !ok {
		return agent.RuntimeTask{}, ErrAgentNotInitialized
	}
	runCtx := ws.ctx
	if in.NonInteractive {
		runCtx = permission.WithNonInteractive(runCtx)
	}
	return c.RuntimeStartSubagent(runCtx, sid, in.Prompt, in.Profile, in.Description)
}

func (b *Backend) RuntimeSubagents(ctx context.Context, wid, sid string) ([]proto.Session, error) {
	ws, err := b.runtimeSession(ctx, wid, sid)
	if err != nil {
		return nil, err
	}
	svc, ok := ws.Sessions.(interface {
		ListChildren(context.Context, string) ([]session.Session, error)
	})
	if !ok {
		return nil, errors.New("child sessions are unavailable")
	}
	children, err := svc.ListChildren(ctx, sid)
	if err != nil {
		return nil, err
	}
	out := []proto.Session{}
	for _, s := range children {
		if !ws.Sessions.IsAgentToolSession(s.ID) {
			continue
		}
		out = append(out, proto.Session{ID: s.ID, Title: s.Title, ParentSessionID: s.ParentSessionID, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt})
	}
	return out, nil
}

func (b *Backend) RuntimeClearQueue(ctx context.Context, wid, sid string) error {
	ws, err := b.runtimeSession(ctx, wid, sid)
	if err != nil {
		return err
	}
	c, ok := ws.AgentCoordinator.(runtimeQueueCoordinator)
	if !ok {
		return ErrAgentNotInitialized
	}
	return c.RuntimeClearQueue(sid)
}
