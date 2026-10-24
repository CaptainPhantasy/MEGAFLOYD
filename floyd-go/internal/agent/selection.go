package agent

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/legacy-ai/floyd/internal/agents"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/legacy-ai/floyd/internal/message"
)

// loadWorkspaceAgentSelector loads configured roles and installed fallbacks.
func loadWorkspaceAgentSelector(workingDir string) (*AgentSelector, error) {
	defs := builtinTaskRoles()
	paths := append(config.GlobalAgentsDirs(), filepath.Join(workingDir, megafloyd.ProjectDirName(), "agents"))
	if !megafloyd.IsActive() {
		paths = append(paths, filepath.Join(workingDir, ".agents", "agents"))
	}
	for _, dir := range paths {
		loaded, err := agents.LoadAgents(dir)
		if err != nil {
			return nil, err
		}
		for _, definition := range loaded {
			found := false
			for i := range defs {
				if strings.EqualFold(defs[i].Name, definition.Name) {
					defs[i] = definition
					found = true
					break
				}
			}
			if !found {
				defs = append(defs, definition)
			}
		}
	}
	return selectorForDefinitions(defs), nil
}

func builtinTaskRoles() []agents.AgentDefinition {
	roles := []struct{ name, instruction string }{
		{"review", "Review the requested changes for concrete defects. Explain each finding with a location and observable consequence."},
		{"test", "Verify the requested behavior with the smallest relevant check. Report the command and observed result."},
		{"docs", "Write or update the requested documentation to match actual behavior, using clear examples."},
		{"debug", "Trace the reported failure to its cause, make a scoped fix, and verify the original failure."},
		{"security", "Inspect the requested security boundary, preserve existing access controls, and support findings with evidence."},
		{"performance", "Identify the measured bottleneck, change only the requested path, and compare the relevant result."},
		{"release", "Prepare the requested release or deployment and verify prerequisites. Publish only within the user's authorization."},
		{"api", "Implement the requested API behavior while preserving caller contracts and existing permissions."},
		{"ui", "Implement the requested interface behavior using the existing design and verify the user interaction."},
		{"refactor", "Refactor the requested code while preserving behavior and checking the affected contract."},
	}
	defs := make([]agents.AgentDefinition, 0, len(roles))
	for _, role := range roles {
		defs = append(defs, agents.AgentDefinition{Name: role.name, Description: role.instruction, Tags: []string{role.name}, SystemPrompt: role.instruction})
	}
	return defs
}

func selectorForDefinitions(defs []agents.AgentDefinition) *AgentSelector {
	as := &AgentSelector{agents: defs, agentsByTrigger: make(map[string]*agents.AgentDefinition), agentsByTag: make(map[string][]*agents.AgentDefinition)}
	for i := range as.agents {
		def := &as.agents[i]
		if def.Trigger != "" {
			as.agentsByTrigger[strings.ToLower(def.Trigger)] = def
		}
		for _, tag := range def.Tags {
			tag = strings.ToLower(tag)
			as.agentsByTag[tag] = append(as.agentsByTag[tag], def)
		}
	}
	return as
}

func (as *AgentSelector) roleForPrompt(prompt string, history []message.Message) (*agents.AgentDefinition, bool) {
	explicit := func(text string) *agents.AgentDefinition {
		for i := range as.agents {
			def := &as.agents[i]
			if strings.HasPrefix(text, "# "+def.Name+"\n\n"+def.SystemPrompt) {
				return def
			}
		}
		return nil
	}
	if def := explicit(prompt); def != nil {
		return def, true
	}
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == message.User {
			if def := explicit(history[i].Content().Text); def != nil {
				return def, true
			}
		}
	}
	classification := as.ClassifyTask(prompt)
	return as.GetAgentByName(classification.SuggestedAgent), false
}

func (c *coordinator) selectedRole(ctx context.Context, sessionID, prompt string) (string, error) {
	if c.selector == nil {
		return "", nil
	}
	history, err := c.messages.List(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("load session for agent selection: %w", err)
	}
	definition, explicit := c.selector.roleForPrompt(prompt, history)
	if definition == nil {
		return "", nil
	}
	slog.Info("Selected task agent", "session_id", sessionID, "agent", definition.Name, "explicit", explicit)
	return fmt.Sprintf("<selected_task_agent name=%q>\nApply this task-specific guidance within the user's instructions and existing permissions.\n%s\n</selected_task_agent>", definition.Name, definition.SystemPrompt), nil
}
