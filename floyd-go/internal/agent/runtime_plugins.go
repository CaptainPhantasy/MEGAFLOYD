package agent

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/legacy-ai/floyd/internal/agent/prompt"
	"github.com/legacy-ai/floyd/internal/agents"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/pluginruntime"
	"github.com/legacy-ai/floyd/internal/skills"
)

func (c *coordinator) RuntimePluginPrompt(ctx context.Context, sessionID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	snapshot, err := pluginruntime.Read(c.cfg.Config())
	if err != nil {
		return "", err
	}
	var out []string
	for _, item := range snapshot.SystemPrompts {
		if strings.TrimSpace(item.Content) != "" {
			out = append(out, fmt.Sprintf("<plugin_guidance name=%q>\n%s\n</plugin_guidance>", item.PluginID, item.Content))
		}
	}
	return strings.Join(out, "\n\n"), nil
}
func (c *coordinator) runtimeAgentSelector() (*AgentSelector, error) {
	own, err := loadWorkspaceAgentSelector(c.cfg.WorkingDir())
	if err != nil {
		return nil, err
	}
	snapshot, err := pluginruntime.Read(c.cfg.Config())
	if err != nil {
		return nil, err
	}
	defs := slices.Clone(own.agents)
	for _, root := range snapshot.AgentRoots {
		loaded, err := agents.LoadAgents(root)
		if err != nil {
			return nil, err
		}
		for _, definition := range loaded {
			existing := -1
			for i := range defs {
				if strings.EqualFold(defs[i].Name, definition.Name) {
					existing = i
					break
				}
			}
			if existing < 0 {
				defs = append(defs, definition)
			} else if defs[existing].FilePath == "" {
				defs[existing] = definition
			}
		}
	}
	return selectorForDefinitions(defs), nil
}

// RuntimeWithIdle holds accepted/dispatch transitions while a workspace-wide
// control refreshes the installed palette. It includes detached child agents.
func (c *coordinator) RuntimeWithIdle(operation func() error) error {
	list := []SessionAgent{c.currentAgent}
	store := c.runtimeTasks()
	store.Lock()
	for _, task := range store.tasks {
		list = append(list, task.agent)
	}
	store.Unlock()
	seen := map[*sessionAgent]bool{}
	var releases []func()
	defer func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}()
	for _, entry := range list {
		a, ok := entry.(*sessionAgent)
		if !ok {
			if entry != nil && entry.IsBusy() {
				return ErrSessionBusy
			}
			continue
		}
		if seen[a] {
			continue
		}
		seen[a] = true
		a.dispatchMuCreate.Lock()
		releases = append(releases, a.dispatchMuCreate.Unlock)
		var ids []string
		locks := map[string]*sync.Mutex{}
		for id, lock := range a.dispatchMu.Seq2() {
			ids = append(ids, id)
			locks[id] = lock
		}
		slices.Sort(ids)
		for _, id := range ids {
			lock := locks[id]
			lock.Lock()
			releases = append(releases, lock.Unlock)
		}
		a.acceptedMu.Lock()
		releases = append(releases, a.acceptedMu.Unlock)
		for count := range a.acceptedRuns.Seq() {
			if count > 0 {
				return ErrSessionBusy
			}
		}
		if a.IsBusy() {
			return ErrSessionBusy
		}
		for queue := range a.messageQueue.Seq() {
			if len(queue) > 0 {
				return ErrSessionBusy
			}
		}
	}
	if c.runtimeChildrenBusy() {
		return ErrSessionBusy
	}
	return operation()
}

// RuntimeReloadPlugins refreshes the existing engine without recreating models
// or discarding sessions. The caller holds RuntimeWithIdle.
func (c *coordinator) RuntimeReloadPlugins(ctx context.Context, manager *skills.Manager) error {
	if err := c.readyWg.Wait(); err != nil {
		return err
	}
	c.toolRefreshMu.Lock()
	defer c.toolRefreshMu.Unlock()
	if manager != nil {
		c.allSkills = manager.AllSkills()
		c.activeSkills = manager.ActiveSkills()
	} else {
		c.allSkills, c.activeSkills = discoverSkills(c.cfg)
	}
	c.skillTracker.Reload(c.activeSkills)
	selector, err := c.runtimeAgentSelector()
	if err != nil {
		return err
	}
	c.selector = selector
	agentCfg, ok := c.cfg.Config().Agents[config.AgentCoder]
	if !ok {
		return errCoderAgentNotConfigured
	}
	template, err := coderPrompt(prompt.WithWorkingDir(c.cfg.WorkingDir()))
	if err != nil {
		return err
	}
	model := c.currentAgent.Model()
	full, err := template.Build(ctx, model.Model.Provider(), model.Model.Model(), c.cfg)
	if err != nil {
		return err
	}
	cacheable := prompt.BuildCacheablePrompts(ctx, full, prompt.PromptDataForDynamic(ctx, c.cfg.WorkingDir(), *c.cfg.Config()))
	agentTools, err := c.buildTools(ctx, agentCfg, false)
	if err != nil {
		return err
	}
	c.currentAgent.SetSystemPrompt(cacheable.StaticPrompt)
	c.currentAgent.SetTools(agentTools)
	if a, ok := c.currentAgent.(*sessionAgent); ok {
		a.SetDynamicContext(cacheable.DynamicContext)
		a.sessionStartRunner = c.eventHookRunner(hooks.EventSessionStart, false)
		a.stopRunner = c.eventHookRunner(hooks.EventStop, false)
	}
	return nil
}
