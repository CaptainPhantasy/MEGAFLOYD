package backend

import (
	"context"
	"github.com/legacy-ai/floyd/internal/agent/tools/mcp"
	"github.com/legacy-ai/floyd/internal/pluginruntime"
	"github.com/legacy-ai/floyd/internal/skills"
)

type pluginCoordinator interface {
	RuntimeWithIdle(func() error) error
	RuntimeReloadPlugins(context.Context, *skills.Manager) error
}
type PluginRuntimeResult struct {
	Reloaded  bool              `json:"reloaded"`
	Unchanged bool              `json:"unchanged"`
	Skills    int               `json:"skills"`
	MCP       map[string]string `json:"mcp_states"`
}

func (b *Backend) ReloadPlugins(ctx context.Context, wid string, in pluginruntime.Input) (PluginRuntimeResult, error) {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return PluginRuntimeResult{}, err
	}
	ws.runMu.Lock()
	defer ws.runMu.Unlock()
	if ws.closing {
		return PluginRuntimeResult{}, ErrWorkspaceClosing
	}
	same, err := pluginruntime.Unchanged(ws.Cfg.Config(), in)
	if err != nil {
		return PluginRuntimeResult{}, err
	}
	if same {
		return pluginRuntimeResult(ws, in, true), nil
	}
	apply := func() error {
		if _, err := pluginruntime.Apply(ws.Cfg, in); err != nil {
			return err
		}
		discovery := skills.DiscoveryConfig{SkillsPaths: ws.Cfg.Config().Options.SkillsPaths, DisabledSkills: ws.Cfg.Config().Options.DisabledSkills, WorkingDir: ws.Path, Resolver: ws.Cfg.Resolver().ResolveValue}
		if ws.Skills != nil {
			ws.Skills.Reload(discovery)
		}
		mcp.Reinitialize(ws.ctx, ws.Cfg)
		if coordinator, ok := ws.AgentCoordinator.(pluginCoordinator); ok {
			if err := coordinator.RuntimeReloadPlugins(ctx, ws.Skills); err != nil {
				return err
			}
		}
		publishConfigChanged(ws)
		return nil
	}
	if coordinator, ok := ws.AgentCoordinator.(pluginCoordinator); ok {
		err = coordinator.RuntimeWithIdle(apply)
	} else {
		err = apply()
	}
	if err != nil {
		return PluginRuntimeResult{}, err
	}
	return pluginRuntimeResult(ws, in, false), nil
}
func pluginRuntimeResult(ws *Workspace, in pluginruntime.Input, unchanged bool) PluginRuntimeResult {
	result := PluginRuntimeResult{Reloaded: true, Unchanged: unchanged, MCP: map[string]string{}}
	if ws.Skills != nil {
		result.Skills = len(ws.Skills.ActiveSkills())
	}
	for name := range in.MCP {
		if state, ok := mcp.GetState(name, ws.Cfg); ok {
			result.MCP[name] = state.State.String()
		} else {
			result.MCP[name] = "starting"
		}
	}
	return result
}
