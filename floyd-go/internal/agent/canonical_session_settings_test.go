package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

type canonicalSettingsModel struct {
	finishStreamModel
	calls []fantasy.Call
}

func (m *canonicalSettingsModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.calls = append(m.calls, call)
	return m.finishStreamModel.Stream(ctx, call)
}

func TestCanonicalSettingsStayWithAcceptedQueuedRun(t *testing.T) {
	t.Setenv("MEGAFLOYD_ACTIVE", "1")
	t.Setenv("MEGAFLOYD_HOME", t.TempDir())
	env := testEnv(t)
	sess, err := env.sessions.Create(t.Context(), "settings integration")
	require.NoError(t, err)
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Existing session"}}})
	require.NoError(t, err)
	createTool := func(name string) fantasy.AgentTool {
		return fantasy.NewAgentTool(name, "test tool", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextResponse("unused"), nil
		})
	}
	palette := []fantasy.AgentTool{createTool("alpha"), createTool("mcp_native"), &installedMCPTool{AgentTool: createTool("mcp_gateway_read"), server: "gateway"}, &installedMCPTool{AgentTool: createTool("mcp_other_read"), server: "other"}}
	model := &canonicalSettingsModel{finishStreamModel: finishStreamModel{text: "done"}}
	sa := testSessionAgent(env, model, model, "BASE_PROMPT", palette...).(*sessionAgent)
	cfg := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{DataDirectory: filepath.Join(env.workingDir, ".megafloyd")}}, env.workingDir)
	c := &coordinator{cfg: cfg, sessions: env.sessions, currentAgent: sa}
	profileDir := filepath.Join(env.workingDir, ".megafloyd", "agents")
	require.NoError(t, os.MkdirAll(profileDir, 0700))
	profilePath := filepath.Join(profileDir, "canonical-profile.md")
	profile := func(prompt string) {
		require.NoError(t, os.WriteFile(profilePath, []byte("---\nname: canonical-profile\ndescription: Canonical settings test\nallowed_tools: [alpha, mcp_native, mcp_gateway_read, mcp_other_read]\n---\n\n"+prompt+"\n"), 0600))
	}
	profile("PROFILE_BEFORE")
	mutator := env.sessions.(session.MetadataMutator)
	_, err = mutator.MutateMetadata(t.Context(), sess.ID, func(metadata map[string]any) error {
		metadata["agent_config"] = map[string]any{"system_prompt": "SETTINGS_BEFORE", "profile": "canonical-profile", "tools": []string{"alpha", "mcp_native", "mcp_gateway_read", "mcp_other_read"}, "mcp_servers": []string{"gateway"}, "disabled_tools": []string{"alpha"}}
		return nil
	})
	require.NoError(t, err)
	accepted := c.BeginAccepted(sess.ID)
	require.NotNil(t, accepted)
	require.NoError(t, accepted.settingsErr)
	profile("PROFILE_AFTER")
	_, err = mutator.MutateMetadata(t.Context(), sess.ID, func(metadata map[string]any) error {
		metadata["agent_config"] = map[string]any{"system_prompt": "SETTINGS_AFTER", "profile": "canonical-profile", "tools": []string{}}
		return nil
	})
	require.NoError(t, err)
	snapshot, err := c.sessionSettingsSnapshot(t.Context(), accepted, sess.ID)
	require.NoError(t, err)
	sa.activeRequests.Set(sess.ID, &activeCancel{cancel: func() {}})
	_, err = sa.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, RunID: "accepted-settings", Prompt: "queued first settings", Accepted: accepted, RunSettings: snapshot})
	require.NoError(t, err)
	require.Empty(t, model.calls)
	sa.activeRequests.Del(sess.ID)
	fresh, err := c.captureSessionRunSettings(t.Context(), sess.ID)
	require.NoError(t, err)
	_, err = sa.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, RunID: "fresh-settings", Prompt: "current settings", RunSettings: fresh})
	require.NoError(t, err)
	require.Len(t, model.calls, 2)
	require.Empty(t, model.calls[0].Tools, "an explicit empty tool selection disables tools")
	require.Contains(t, intelligenceCallText(model.calls[0]), "SETTINGS_AFTER")
	require.Contains(t, intelligenceCallText(model.calls[0]), "PROFILE_AFTER")
	require.NotContains(t, intelligenceCallText(model.calls[0]), "SETTINGS_BEFORE")
	queued := model.calls[1]
	require.Contains(t, intelligenceCallText(queued), "BASE_PROMPT")
	require.Contains(t, intelligenceCallText(queued), "SETTINGS_BEFORE")
	require.Contains(t, intelligenceCallText(queued), "PROFILE_BEFORE")
	require.NotContains(t, intelligenceCallText(queued), "PROFILE_AFTER")
	var names []string
	for _, tool := range queued.Tools {
		names = append(names, tool.GetName())
	}
	require.Equal(t, []string{"mcp_native", "mcp_gateway_read"}, names, "disabled native tools and unselected MCP servers must be removed from the actual model request")
	noMCP := []string{}
	_, err = sa.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, Prompt: "native tools only", RunSettings: SessionRunSettings{MCPServers: &noMCP}})
	require.NoError(t, err)
	names = nil
	for _, tool := range model.calls[2].Tools {
		names = append(names, tool.GetName())
	}
	require.Equal(t, []string{"alpha", "mcp_native"}, names, "native mcp_ names must not be mistaken for MCP server tools")
	require.Len(t, sa.tools.Copy(), 4, "per-call filtering must preserve the installed shared palette")
	require.Same(t, model, sa.Model().Model, "session settings must not replace the configured model")
}
