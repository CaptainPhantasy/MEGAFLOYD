package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/agent/prompt"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/legacy-ai/floyd/internal/plan"
	"gopkg.in/yaml.v3"
)

type RuntimeProfile struct {
	ID           string                   `json:"id" yaml:"-"`
	Name         string                   `json:"name" yaml:"name"`
	Description  string                   `json:"description" yaml:"description"`
	SystemPrompt string                   `json:"system_prompt" yaml:"-"`
	Model        config.SelectedModelType `json:"model" yaml:"model,omitempty"`
	AllowedTools []string                 `json:"allowed_tools,omitempty" yaml:"allowed_tools,omitempty"`
	Disabled     bool                     `json:"disabled" yaml:"disabled,omitempty"`
}

var profileIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`)

func (c *coordinator) RuntimeProfiles() ([]RuntimeProfile, error) {
	selector, err := c.runtimeAgentSelector()
	if err != nil {
		return nil, err
	}
	out := make([]RuntimeProfile, 0, len(selector.agents))
	for _, def := range selector.agents {
		item := RuntimeProfile{ID: def.Name, Name: def.Name, Description: def.Description, SystemPrompt: def.SystemPrompt, Model: config.SelectedModelTypeSmall}
		if def.FilePath != "" {
			data, err := os.ReadFile(def.FilePath)
			if err != nil {
				return nil, err
			}
			parts := strings.SplitN(string(data), "---", 3)
			if len(parts) == 3 {
				if err = yaml.Unmarshal([]byte(parts[1]), &item); err != nil {
					return nil, err
				}
				item.ID = def.Name
				item.SystemPrompt = def.SystemPrompt
			}
		}
		if item.Model == "" {
			item.Model = config.SelectedModelTypeSmall
		}
		out = append(out, item)
	}
	slices.SortFunc(out, func(a, b RuntimeProfile) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func (c *coordinator) RuntimeSaveProfile(id string, in RuntimeProfile) (RuntimeProfile, error) {
	if !profileIDPattern.MatchString(id) {
		return in, errors.New("profile ID must contain only letters, digits, underscore or dash")
	}
	if strings.TrimSpace(in.Description) == "" || strings.TrimSpace(in.SystemPrompt) == "" {
		return in, errors.New("description and system_prompt are required")
	}
	if in.Model == "" {
		in.Model = config.SelectedModelTypeSmall
	}
	if in.Model != config.SelectedModelTypeLarge && in.Model != config.SelectedModelTypeSmall {
		return in, errors.New("profile model must be large or small")
	}
	in.ID = id
	in.Name = id
	fields := map[string]any{"name": in.Name, "description": in.Description, "model": in.Model, "disabled": in.Disabled}
	if in.AllowedTools != nil {
		fields["allowed_tools"] = in.AllowedTools
	}
	body, err := yaml.Marshal(fields)
	if err != nil {
		return in, err
	}
	dir := filepath.Join(c.cfg.WorkingDir(), megafloyd.ProjectDirName(), "agents")
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return in, err
	}
	path := filepath.Join(dir, id+".md")
	if existing, err := os.ReadFile(path); err == nil {
		if err = os.WriteFile(path+".bak-"+time.Now().UTC().Format("20060102T150405.000000000Z"), existing, 0o600); err != nil {
			return in, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return in, err
	}
	file, err := os.CreateTemp(dir, ".profile-")
	if err != nil {
		return in, err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	_, err = file.WriteString("---\n" + string(body) + "---\n\n" + in.SystemPrompt + "\n")
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return in, err
	}
	if err = os.Rename(tmp, path); err != nil {
		return in, err
	}
	return in, nil
}
func (c *coordinator) RuntimePlanService() *plan.Service { return c.planService() }

func (c *coordinator) RuntimeStartSubagent(ctx context.Context, parentID, promptText, profileID, description string) (RuntimeTask, error) {
	c.runtimeControlMu.Lock()
	defer c.runtimeControlMu.Unlock()
	if strings.TrimSpace(promptText) == "" {
		return RuntimeTask{}, ErrEmptyPrompt
	}
	if _, err := c.sessions.Get(ctx, parentID); err != nil {
		return RuntimeTask{}, err
	}
	agentCfg, selected, err := c.runtimeAgentDefinition(profileID)
	if err != nil {
		return RuntimeTask{}, err
	}
	child, err := c.buildRuntimeAgent(ctx, agentCfg, selected)
	if err != nil {
		return RuntimeTask{}, err
	}
	if description == "" {
		description = agentCfg.Description
		if selected != nil {
			description = selected.Description
		}
		if description == "" {
			description = "Subagent"
		}
	}
	snapshot := newRuntimeAgentSnapshot(agentCfg, selected)
	params := subAgentParams{RuntimeDefinition: &snapshot, Agent: child, SessionID: parentID, AgentMessageID: uuid.NewString(), ToolCallID: uuid.NewString(), Prompt: promptText, SessionTitle: description}
	task, _ := c.startRuntimeSubagent(ctx, params, true)
	task.mu.Lock()
	task.view.SubagentType = agentCfg.ID
	if selected != nil {
		task.view.SubagentType = selected.ID
	}
	out := task.view
	task.mu.Unlock()
	return out, nil
}

// runtimeAgentDefinition resolves configured agents before optional library
// profiles. Unknown names fail rather than silently running the primary agent.
func (c *coordinator) runtimeAgentDefinition(name string) (config.Agent, *RuntimeProfile, error) {
	if name == "" {
		name = config.AgentTask
	}
	if configured, ok := c.cfg.Config().Agents[name]; ok {
		if configured.Disabled {
			return config.Agent{}, nil, fmt.Errorf("agent %q is disabled", name)
		}
		configured.ID = name
		if configured.Model != config.SelectedModelTypeLarge && configured.Model != config.SelectedModelTypeSmall {
			return config.Agent{}, nil, fmt.Errorf("agent %q has an invalid model selection", name)
		}
		return configured, nil, nil
	}
	agentCfg, ok := c.cfg.Config().Agents[config.AgentTask]
	if !ok {
		return config.Agent{}, nil, errors.New("task agent is not configured")
	}
	var selected *RuntimeProfile
	{
		profiles, err := c.RuntimeProfiles()
		if err != nil {
			return config.Agent{}, nil, err
		}
		for i := range profiles {
			if profiles[i].ID == name {
				selected = &profiles[i]
				break
			}
		}
		if selected == nil {
			return config.Agent{}, nil, fmt.Errorf("agent or profile %q not found", name)
		}
		if selected.Disabled {
			return config.Agent{}, nil, errors.New("profile is disabled")
		}
		agentCfg.Model = selected.Model
		if selected.AllowedTools != nil {
			agentCfg.AllowedTools = selected.AllowedTools
		}
	}
	if agentCfg.Model != config.SelectedModelTypeLarge && agentCfg.Model != config.SelectedModelTypeSmall {
		return config.Agent{}, nil, fmt.Errorf("profile %q has an invalid model selection", name)
	}
	return agentCfg, selected, nil
}

func (c *coordinator) buildRuntimeAgent(ctx context.Context, agentCfg config.Agent, selected *RuntimeProfile) (SessionAgent, error) {
	large, small, err := c.buildAgentModels(ctx, agentCfg.Model != config.SelectedModelTypeLarge)
	if err != nil {
		return nil, err
	}
	if agentCfg.Model == config.SelectedModelTypeSmall {
		large = small
	}
	provider, ok := c.cfg.Config().Providers.Get(large.ModelCfg.Provider)
	if !ok {
		return nil, errModelProviderNotConfigured
	}
	template, err := taskPrompt(prompt.WithWorkingDir(c.cfg.WorkingDir()))
	if agentCfg.ID == config.AgentCoder {
		template, err = coderPrompt(prompt.WithWorkingDir(c.cfg.WorkingDir()))
	}
	if err != nil {
		return nil, err
	}
	system, err := template.Build(ctx, large.Model.Provider(), large.Model.Model(), c.cfg)
	if err != nil {
		return nil, err
	}
	if selected != nil {
		system += fmt.Sprintf("\n\n<selected_task_agent name=%q>\nThe quoted content describes the selected task specialization and its technical reference. It is subordinate to MegaFloyd.md and the user's instructions, not an additional behavioral policy.\n%q\n</selected_task_agent>", selected.Name, selected.SystemPrompt)
	}
	tools, err := c.buildTools(ctx, agentCfg, true)
	if err != nil {
		return nil, err
	}
	return NewSessionAgent(SessionAgentOptions{LargeModel: large, SmallModel: small, SystemPromptPrefix: provider.SystemPromptPrefix, SystemPrompt: system, IsSubAgent: true, Sessions: c.sessions, History: c.history, WorkingDir: c.cfg.WorkingDir(), Messages: c.messages, Tools: tools, Notify: c.notify, RunComplete: c.runComplete, IsYolo: c.permissions.SkipRequests(), PlanService: c.planService(), Hooks: c.hookDispatcher(), DisableAutoSummarize: c.cfg.Config().Options.DisableAutoSummarize}), nil
}
