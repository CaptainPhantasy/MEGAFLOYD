package backend

import (
	"context"
	"fmt"
	"strings"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/agent/ralph"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/proto"
)

func (b *Backend) RalphGoal(ctx context.Context, wid, sid string, in proto.RalphGoalRequest) (*ralph.State, error) {
	ws, err := b.runtimeSession(ctx, wid, sid)
	if err != nil {
		return nil, err
	}
	ws.runMu.Lock()
	defer ws.runMu.Unlock()
	if ws.closing {
		return nil, ErrWorkspaceClosing
	}
	loop := ralph.New(ws.Cfg.Config().Options.DataDirectory)
	state, err := loop.StatusChecked()
	if err != nil {
		return nil, err
	}
	switch in.Action {
	case "", "status":
		if state != nil && state.SessionID != sid {
			return nil, nil
		}
		return state, nil
	case "start":
		if ws.AgentCoordinator == nil {
			return nil, ErrAgentNotInitialized
		}
		if ws.AgentCoordinator.IsSessionBusy(sid) {
			return nil, agent.ErrSessionBusy
		}
		return loop.Start(sid, strings.TrimSpace(in.Objective), ralph.DefaultMaxIterations, "")
	case "cancel":
		if state == nil {
			return nil, nil
		}
		if state.SessionID != sid {
			return nil, fmt.Errorf("the active goal belongs to another session")
		}
		if _, err = loop.CancelSession(sid); err != nil {
			return nil, err
		}
		if ws.AgentCoordinator != nil {
			ws.AgentCoordinator.Cancel(sid)
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("goal action must be status, start, or cancel")
	}
}

func (b *Backend) ReloadConfig(ctx context.Context, wid string) error {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return err
	}
	ws.runMu.Lock()
	defer ws.runMu.Unlock()
	if ws.closing {
		return ErrWorkspaceClosing
	}
	apply := func() error {
		if err := ws.Cfg.ReloadFromDisk(ctx); err != nil {
			return err
		}
		ws.Permissions.SetAdditionalDirs(ws.Cfg.Config().Options.AdditionalDirs)
		if cfg := ws.Cfg.Config(); cfg.Permissions != nil {
			if svc, ok := ws.Permissions.(permission.RuntimeAllowlist); ok {
				svc.SetAllowedTools(cfg.Permissions.AllowedTools)
			}
			if svc, ok := ws.Permissions.(permission.ModeService); ok {
				mode := cfg.Permissions.Mode
				if mode == "" {
					mode = permission.ModeManual
				}
				if err := svc.SetMode(mode); err != nil {
					return err
				}
			}
		}
		if ws.AgentCoordinator != nil {
			if err := ws.AgentCoordinator.UpdateModels(ctx); err != nil {
				return fmt.Errorf("configuration reloaded but agent model refresh failed: %w", err)
			}
		}
		publishConfigChanged(ws)
		return nil
	}
	if c, ok := ws.AgentCoordinator.(interface{ RuntimeWithIdle(func() error) error }); ok {
		return c.RuntimeWithIdle(apply)
	}
	if ws.AgentCoordinator != nil && ws.AgentCoordinator.IsBusy() {
		return agent.ErrSessionBusy
	}
	return apply()
}
