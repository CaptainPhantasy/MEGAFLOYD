package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/stretchr/testify/require"
)

type intelligenceModel struct {
	finishStreamModel
	calls       []fantasy.Call
	generateErr error
	streamErr   error
	deadline    time.Time
}

func (m *intelligenceModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.calls = append(m.calls, call)
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	return m.finishStreamModel.Stream(ctx, call)
}
func (m *intelligenceModel) Generate(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
	m.calls = append(m.calls, call)
	m.deadline, _ = ctx.Deadline()
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	return m.finishStreamModel.Generate(ctx, call)
}
func intelligenceCallText(call fantasy.Call) string {
	var out strings.Builder
	for _, msg := range call.Prompt {
		for _, part := range msg.Content {
			if text, ok := part.(fantasy.TextPart); ok {
				out.WriteString(text.Text + "\n")
			}
		}
	}
	return out.String()
}

func TestCanonicalAgentSelectionReachesModel(t *testing.T) {
	agentsDir := t.TempDir()
	t.Setenv("MEGAFLOYD_ACTIVE", "1")
	t.Setenv("MEGAFLOYD_AGENTS_DIR", agentsDir)
	t.Setenv("FLOYD_AGENTS_DIR", t.TempDir())
	require.NoError(t, os.WriteFile(filepath.Join(agentsDir, "review.md"), []byte("---\nname: review\ndescription: Canonical reviewer\ntags: [Review]\n---\nCANONICAL_REVIEW_INSTRUCTION"), 0o600))
	selector, err := loadWorkspaceAgentSelector(t.TempDir())
	require.NoError(t, err)
	env := testEnv(t)
	sess, err := env.sessions.Create(t.Context(), "canonical selection")
	require.NoError(t, err)
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Existing session"}}})
	require.NoError(t, err)
	coordinator := &coordinator{messages: env.messages, selector: selector}
	role, err := coordinator.selectedRole(t.Context(), sess.ID, "review this change")
	require.NoError(t, err)
	require.Contains(t, role, "CANONICAL_REVIEW_INSTRUCTION")
	model := &intelligenceModel{finishStreamModel: finishStreamModel{text: "done"}}
	a := testSessionAgent(env, model, model, "base prompt")
	_, err = a.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, Prompt: "review this change", SystemPromptAddition: role})
	require.NoError(t, err)
	require.Contains(t, intelligenceCallText(model.calls[0]), "CANONICAL_REVIEW_INSTRUCTION")
	_, err = a.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, Prompt: "hello"})
	require.NoError(t, err)
	for _, msg := range model.calls[len(model.calls)-1].Prompt {
		if msg.Role == fantasy.MessageRoleSystem {
			for _, part := range msg.Content {
				if text, ok := part.(fantasy.TextPart); ok {
					require.NotContains(t, text.Text, "CANONICAL_REVIEW_INSTRUCTION")
				}
			}
		}
	}
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "# review\n\nCANONICAL_REVIEW_INSTRUCTION"}}})
	require.NoError(t, err)
	explicit, err := coordinator.selectedRole(t.Context(), sess.ID, "write documentation")
	require.NoError(t, err)
	require.Contains(t, explicit, "CANONICAL_REVIEW_INSTRUCTION", "explicit session role takes precedence over automatic matching")
}

func TestCanonicalTieredSummaryUsesCompressedTranscript(t *testing.T) {
	env := testEnv(t)
	sess, err := env.sessions.Create(t.Context(), "canonical summary")
	require.NoError(t, err)
	requirement := "You must preserve this requirement verbatim: " + strings.Repeat("critical detail ", 80) + "KEEP_THE_END"
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: requirement}}})
	require.NoError(t, err)
	verbose := "BEGIN_OUTPUT" + strings.Repeat("0123456789", 600) + "END_OUTPUT"
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.Tool, Parts: []message.ContentPart{message.ToolResult{Name: "bash", ToolCallID: "call", Content: verbose}}})
	require.NoError(t, err)
	model := &intelligenceModel{finishStreamModel: finishStreamModel{text: "Compacted conversation."}}
	a := testSessionAgent(env, model, model, "base").(*sessionAgent)
	imageModel := a.largeModel.Get()
	imageModel.CatwalkCfg.SupportsImages = true
	a.largeModel.Set(imageModel)
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.BinaryContent{Path: "diagram.png", MIMEType: "image/png", Data: []byte("image-sentinel")}}})
	require.NoError(t, err)
	require.NoError(t, a.Summarize(t.Context(), sess.ID, nil, nil))
	require.Len(t, model.calls, 1)
	retainedImage := false
	for _, msg := range model.calls[0].Prompt {
		for _, part := range msg.Content {
			if file, ok := part.(fantasy.FilePart); ok && string(file.Data) == "image-sentinel" {
				retainedImage = true
			}
		}
	}
	require.True(t, retainedImage, "tiered compression retains existing media support")
	input := intelligenceCallText(model.calls[0])
	require.Contains(t, input, "SEMANTIC TIERED COMPRESSION")
	require.Contains(t, input, requirement)
	require.Contains(t, input, "BEGIN_OUTPUT")
	require.Contains(t, input, "END_OUTPUT")
	require.NotContains(t, input, verbose)
	stored, err := env.sessions.Get(t.Context(), sess.ID)
	require.NoError(t, err)
	summary, err := env.messages.Get(t.Context(), stored.SummaryMessageID)
	require.NoError(t, err)
	require.Contains(t, summary.Content().Text, requirement)
	require.NoError(t, a.Summarize(t.Context(), sess.ID, nil, nil))
	stored, err = env.sessions.Get(t.Context(), sess.ID)
	require.NoError(t, err)
	summary, err = env.messages.Get(t.Context(), stored.SummaryMessageID)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(summary.Content().Text, requirement), "repeated compaction keeps the exact requirement once")
}

func TestCanonicalSuggestionOneBoundedModelCall(t *testing.T) {
	model := &intelligenceModel{finishStreamModel: finishStreamModel{text: "Add the missing regression test"}}
	msgs := []message.Message{{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "The fix is implemented."}}}}
	result, _, err := generatePromptSuggestion(t.Context(), Model{Model: model}, msgs, "Add", nil)
	require.NoError(t, err)
	require.Equal(t, "Add the missing regression test", result)
	require.Len(t, model.calls, 1)
	require.Empty(t, model.calls[0].Tools)
	require.EqualValues(t, 256, *model.calls[0].MaxOutputTokens)
	require.WithinDuration(t, time.Now().Add(15*time.Second), model.deadline, 2*time.Second)
	model.calls = nil
	model.generateErr = errors.New("provider unavailable")
	_, _, err = generatePromptSuggestion(t.Context(), Model{Model: model}, msgs, "", nil)
	require.Error(t, err)
	require.Len(t, model.calls, 1, "a suggestion never retries or falls back to another provider")
}

func TestCanonicalPersistentHealthGuardsExecutedRuns(t *testing.T) {
	t.Setenv("FLOYD_RUNTIME_PROFILE", "megafloyd")
	t.Setenv("SUPERFLOYD_DEGRADATION_CONTROLS", "on")
	env := testEnv(t)
	sess, err := env.sessions.Create(t.Context(), "canonical resilience")
	require.NoError(t, err)
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Existing turn"}}})
	require.NoError(t, err)
	model := &intelligenceModel{streamErr: errors.New("canonical provider failure")}
	dataDir := t.TempDir()
	a := testSessionAgent(env, model, model, "base").(*sessionAgent)
	a.dataDirectory = dataDir

	a.activeRequests.Set(sess.ID, &activeCancel{cancel: func() {}})
	_, err = a.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, Prompt: "queued turn"})
	require.NoError(t, err)
	require.Empty(t, model.calls)
	files, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	require.Empty(t, files, "accepting a queued turn does not record an executed run")
	a.ClearQueue(sess.ID)
	a.activeRequests.Del(sess.ID)

	for range 2 {
		_, err = a.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, Prompt: "same task"})
		require.Error(t, err)
	}
	require.Len(t, model.calls, 2)
	restarted := testSessionAgent(env, model, model, "base").(*sessionAgent)
	restarted.dataDirectory = dataDir
	_, err = restarted.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, Prompt: "same task"})
	require.ErrorContains(t, err, "circuit")
	require.Len(t, model.calls, 2, "persisted failures stop a new runtime before another provider call")
}
