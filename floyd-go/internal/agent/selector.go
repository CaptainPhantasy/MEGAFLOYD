package agent

import (
	"path/filepath"
	"strings"

	"github.com/legacy-ai/floyd/internal/agents"
)

// AgentSelector provides automatic agent selection based on task type
type AgentSelector struct {
	agents          []agents.AgentDefinition
	agentsByTrigger map[string]*agents.AgentDefinition
	agentsByTag     map[string][]*agents.AgentDefinition
}

// NewAgentSelector creates a new agent selector with loaded agent definitions
func NewAgentSelector(agentsDir string) (*AgentSelector, error) {
	defs, err := agents.LoadAgents(agentsDir)
	if err != nil {
		return nil, err
	}

	as := &AgentSelector{
		agents:          defs,
		agentsByTrigger: make(map[string]*agents.AgentDefinition),
		agentsByTag:     make(map[string][]*agents.AgentDefinition),
	}

	// Index by trigger
	for i := range defs {
		if defs[i].Trigger != "" {
			as.agentsByTrigger[strings.ToLower(defs[i].Trigger)] = &defs[i]
		}
		// Index by tags
		for _, tag := range defs[i].Tags {
			as.agentsByTag[strings.ToLower(tag)] = append(as.agentsByTag[tag], &defs[i])
		}
	}

	return as, nil
}

// TaskClassification represents the type of task
type TaskClassification struct {
	Type           string  // "code", "review", "audit", "test", "docs", "deploy", etc.
	Confidence     float64 // 0-1
	SuggestedAgent string  // Agent name if a specific match is found
}

// ClassifyTask analyzes the user prompt to determine task type and suggested agent
func (as *AgentSelector) ClassifyTask(prompt string) TaskClassification {
	lower := strings.ToLower(prompt)

	// Check for explicit trigger keywords first
	for trigger, agent := range as.agentsByTrigger {
		if strings.Contains(lower, trigger) {
			return TaskClassification{
				Type:           trigger,
				Confidence:     0.9,
				SuggestedAgent: agent.Name,
			}
		}
	}

	// Classify by keyword patterns
	classifiers := []struct {
		keywords []string
		taskType string
		tags     []string
	}{
		{
			keywords: []string{"review", "audit", "check", "inspect", "analyze code"},
			taskType: "review",
			tags:     []string{"review", "code", "audit"},
		},
		{
			keywords: []string{"release", "deploy", "publish", "ship", "version"},
			taskType: "release",
			tags:     []string{"release", "deploy", "audit"},
		},
		{
			keywords: []string{"test", "spec", "coverage", "unit test", "integration test"},
			taskType: "test",
			tags:     []string{"test", "testing", "quality"},
		},
		{
			keywords: []string{"document", "docs", "readme", "comment", "docstring"},
			taskType: "docs",
			tags:     []string{"docs", "documentation", "writing"},
		},
		{
			keywords: []string{"refactor", "clean up", "optimize", "improve"},
			taskType: "refactor",
			tags:     []string{"code", "refactor", "quality"},
		},
		{
			keywords: []string{"debug", "fix", "error", "bug", "issue"},
			taskType: "debug",
			tags:     []string{"debug", "code", "troubleshoot"},
		},
		{
			keywords: []string{"security", "vulnerability", "auth", "permission"},
			taskType: "security",
			tags:     []string{"security", "audit", "review"},
		},
		{
			keywords: []string{"performance", "slow", "latency", "optimize speed"},
			taskType: "performance",
			tags:     []string{"performance", "optimize"},
		},
		{
			keywords: []string{"api", "endpoint", "http", "rest", "graphql"},
			taskType: "api",
			tags:     []string{"api", "backend", "server"},
		},
		{
			keywords: []string{"ui", "frontend", "component", "view", "page"},
			taskType: "ui",
			tags:     []string{"ui", "frontend", "component"},
		},
	}

	// Find best match
	var bestMatch struct {
		taskType string
		score    int
	}

	for _, classifier := range classifiers {
		score := 0
		for _, kw := range classifier.keywords {
			if strings.Contains(lower, kw) {
				score++
			}
		}
		if score > bestMatch.score {
			bestMatch.taskType = classifier.taskType
			bestMatch.score = score
		}
	}

	// Default to code
	if bestMatch.taskType == "" {
		bestMatch.taskType = "code"
	}

	// Find agent by tag match
	var suggestedAgent string
	if bestMatch.taskType != "code" {
		for _, tag := range getClassifierTags(bestMatch.taskType) {
			if agents, ok := as.agentsByTag[tag]; ok && len(agents) > 0 {
				suggestedAgent = agents[0].Name
				break
			}
		}
	}

	confidence := 0.5
	if bestMatch.score > 0 {
		confidence = 0.7
	}
	if bestMatch.score > 1 {
		confidence = 0.85
	}
	if suggestedAgent != "" {
		confidence += 0.1
	}

	return TaskClassification{
		Type:           bestMatch.taskType,
		Confidence:     confidence,
		SuggestedAgent: suggestedAgent,
	}
}

// GetAgentByName returns an agent definition by name
func (as *AgentSelector) GetAgentByName(name string) *agents.AgentDefinition {
	for i := range as.agents {
		if strings.EqualFold(as.agents[i].Name, name) {
			return &as.agents[i]
		}
	}
	return nil
}

// ListAgents returns all available agent names and descriptions
func (as *AgentSelector) ListAgents() []string {
	var names []string
	for _, agent := range as.agents {
		names = append(names, agent.Name)
	}
	return names
}

// GetAgentsDir returns the default agents directory
func GetAgentsDir() string {
	// Default to internal/agents relative to the Floyd source
	// In production, this could be configurable via config
	return filepath.Join(".", "internal", "agents")
}

func getClassifierTags(taskType string) []string {
	tags := map[string][]string{
		"review":      {"review", "code", "audit"},
		"release":     {"release", "audit", "deploy"},
		"test":        {"test", "testing", "quality"},
		"docs":        {"docs", "documentation", "writing"},
		"refactor":    {"code", "refactor"},
		"debug":       {"debug", "troubleshoot"},
		"security":    {"security", "audit"},
		"performance": {"performance", "optimize"},
		"api":         {"api", "backend"},
		"ui":          {"ui", "frontend"},
	}
	if t, ok := tags[taskType]; ok {
		return t
	}
	return []string{"code"}
}
