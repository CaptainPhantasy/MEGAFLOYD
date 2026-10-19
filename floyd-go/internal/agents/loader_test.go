package agents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAgentFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		wantName    string
		wantDesc    string
		wantTrigger string
		wantPrompt  string
		wantErr     bool
	}{
		{
			name: "valid agent with all fields",
			content: `---
name: Code Reviewer
description: Expert code reviewer
trigger: review
---

# Code Reviewer Persona

You are an expert code reviewer.
`,
			wantName:    "Code Reviewer",
			wantDesc:    "Expert code reviewer",
			wantTrigger: "review",
			wantPrompt:  "# Code Reviewer Persona\n\nYou are an expert code reviewer.",
		},
		{
			name: "valid agent minimal",
			content: `---
name: Simple Agent
description: A minimal agent
---
# Simple Agent
`,
			wantName:   "Simple Agent",
			wantDesc:   "A minimal agent",
			wantPrompt: "# Simple Agent",
		},
		{
			name:    "missing frontmatter",
			content: "# No Frontmatter\nJust content",
			wantErr: true,
		},
		{
			name: "unclosed frontmatter",
			content: `---
name: Broken
description: Missing closing delimiter
# Content`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "agent.md")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o644))

			got, err := ParseAgentFile(path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantName, got.Name)
			require.Equal(t, tt.wantDesc, got.Description)
			require.Equal(t, tt.wantTrigger, got.Trigger)
			require.Equal(t, tt.wantPrompt, got.SystemPrompt)
		})
	}
}

func TestAgentDefinition_Validate(t *testing.T) {
	tests := []struct {
		name    string
		agent   AgentDefinition
		wantErr bool
	}{
		{
			name: "valid agent",
			agent: AgentDefinition{
				Name:        "Test",
				Description: "Test description",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			agent: AgentDefinition{
				Description: "Has description",
			},
			wantErr: true,
		},
		{
			name: "missing description",
			agent: AgentDefinition{
				Name: "Has name",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.agent.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLoadAgents(t *testing.T) {
	t.Run("load multiple agents", func(t *testing.T) {
		dir := t.TempDir()

		agent1 := `---
name: Agent One
description: First agent
---
Content 1`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "agent1.md"), []byte(agent1), 0o644))

		agent2 := `---
name: Agent Two
description: Second agent
---
Content 2`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "agent2.md"), []byte(agent2), 0o644))

		// Invalid agent (missing description) - should be skipped
		invalid := `---
name: Incomplete
---
Content`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "invalid.md"), []byte(invalid), 0o644))

		agents, err := LoadAgents(dir)
		require.NoError(t, err)
		require.Len(t, agents, 2)
	})

	t.Run("non-existent directory", func(t *testing.T) {
		agents, err := LoadAgents("/non/existent/directory")
		require.NoError(t, err)
		require.Empty(t, agents)
	})
}
