package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
)

type stubAgentTool struct{}

func (s *stubAgentTool) Info() fantasy.ToolInfo { return fantasy.ToolInfo{Name: "stub"} }
func (s *stubAgentTool) Run(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return fantasy.ToolResponse{}, nil
}
func (s *stubAgentTool) ProviderOptions() fantasy.ProviderOptions   { return nil }
func (s *stubAgentTool) SetProviderOptions(fantasy.ProviderOptions) {}

func provenanceModel() Model {
	return Model{
		ModelCfg:   config.SelectedModel{Provider: "deepseek", Model: "deepseek-v4-flash"},
		CatwalkCfg: catwalk.Model{SupportsImages: true},
	}
}

func provenancePNG(seed byte) []byte {
	payload := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{seed}, 24)...)
	return payload
}

func storedPNG(seed byte) string {
	return base64.StdEncoding.EncodeToString(provenancePNG(seed))
}

// deepSeekRequestBody mirrors the OpenAI-compatible body DeepSeek receives for
// a user turn: messages[1].content[1] is the image part named in the error.
func deepSeekRequestBody(payload []byte) []byte {
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(payload)
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "system"},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "look"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
			}},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return encoded
}

func filePartCount(messages []fantasy.Message) int {
	count := 0
	for _, msg := range messages {
		for _, part := range msg.Content {
			if _, ok := fantasy.AsMessagePart[fantasy.FilePart](part); ok {
				count++
			}
		}
	}
	return count
}

func toolResultText(t *testing.T, msg fantasy.Message) string {
	t.Helper()
	require.Len(t, msg.Content, 1)
	toolResult, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](msg.Content[0])
	require.True(t, ok)
	text, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](toolResult.Output)
	require.True(t, ok)
	return text.Text
}

func TestDeepSeekRejectionFixture(t *testing.T) {
	payload := provenancePNG(7)
	providerErr := &fantasy.ProviderError{
		Title:       "Bad Request",
		Message:     ".messages[1].content[1]: You have uploaded an invalid image. The image data is corrupted or truncated and could not be decoded.",
		RequestBody: deepSeekRequestBody(payload),
	}
	mediaID, ok := attributeRejectedMedia(providerErr)
	require.True(t, ok)
	require.Equal(t, mediaPartID(payload, "image/png"), mediaID)
}

func TestNoCoordinateFallback(t *testing.T) {
	env := testEnv(t)
	agent := testSessionAgent(env, nil, nil, "test prompt").(*sessionAgent)
	model := provenanceModel()
	runMedia := []string{mediaPartID(provenancePNG(1), "image/png")}
	providerErr := &fantasy.ProviderError{Title: "Bad Request", Message: "You have uploaded an invalid image."}

	require.True(t, agent.handleMediaRejection("s", model, providerErr, runMedia))
	verdict, ok := agent.mediaVerdictFor("s", model, runMedia[0])
	require.True(t, ok)
	require.Equal(t, reasonUnidentifiedThisTurn, verdict.Reason)

	other := mediaPartID(provenancePNG(2), "image/png")
	_, ok = agent.mediaVerdictFor("s", model, other)
	require.False(t, ok, "only the media introduced by the failing run is recorded")
}

func TestBadAndGoodImageSameTurn(t *testing.T) {
	env := testEnv(t)
	agent := testSessionAgent(env, nil, nil, "test prompt").(*sessionAgent)
	model := provenanceModel()
	good, bad := storedPNG(1), storedPNG(2)
	badID, ok := mediaIDForStoredData(bad, "image/png")
	require.True(t, ok)
	agent.recordMediaVerdict("s", model, badID, reasonRejectedByProvider, "test")

	result := agent.workaroundProviderMediaLimitations([]fantasy.Message{
		imageToolMessage("good", "image/png", good),
		imageToolMessage("bad", "image/png", bad),
	}, model, true, "s")

	require.Len(t, result, 3)
	require.Equal(t, fantasy.MessageRoleTool, result[0].Role)
	require.Equal(t, fantasy.MessageRoleTool, result[1].Role)
	require.Equal(t, fantasy.MessageRoleUser, result[2].Role)
	require.Contains(t, toolResultText(t, result[0]), "loaded")
	require.Contains(t, toolResultText(t, result[1]), reasonRejectedByProvider)
	require.Equal(t, 1, filePartCount(result), "the good image is still delivered")
}

func TestVerdictScopedByModelAndSession(t *testing.T) {
	env := testEnv(t)
	agent := testSessionAgent(env, nil, nil, "test prompt").(*sessionAgent)
	model := provenanceModel()
	payload := storedPNG(3)
	mediaID, ok := mediaIDForStoredData(payload, "image/png")
	require.True(t, ok)
	agent.recordMediaVerdict("s", model, mediaID, reasonRejectedByProvider, "test")

	otherModel := model
	otherModel.ModelCfg.Model = "another-model"
	for name, tc := range map[string]struct {
		session string
		model   Model
	}{
		"other session": {"t", model},
		"other model":   {"s", otherModel},
	} {
		t.Run(name, func(t *testing.T) {
			result := agent.workaroundProviderMediaLimitations(
				[]fantasy.Message{imageToolMessage("call", "image/png", payload)},
				tc.model, true, tc.session)
			require.Equal(t, 1, filePartCount(result))
		})
	}
}

func TestRestartReRejectionOnly(t *testing.T) {
	env := testEnv(t)
	model := provenanceModel()
	payload := storedPNG(4)
	mediaID, ok := mediaIDForStoredData(payload, "image/png")
	require.True(t, ok)

	first := testSessionAgent(env, nil, nil, "first").(*sessionAgent)
	first.recordMediaVerdict("s", model, mediaID, reasonRejectedByProvider, "test")

	restarted := testSessionAgent(env, nil, nil, "restarted").(*sessionAgent)
	before := restarted.workaroundProviderMediaLimitations(
		[]fantasy.Message{imageToolMessage("call", "image/png", payload)}, model, true, "s")
	require.Equal(t, 1, filePartCount(before), "a restart forgets the verdict and re-sends once")

	restarted.recordMediaVerdict("s", model, mediaID, reasonRejectedByProvider, "test")
	after := restarted.workaroundProviderMediaLimitations(
		[]fantasy.Message{imageToolMessage("call", "image/png", payload)}, model, true, "s")
	require.Equal(t, 0, filePartCount(after), "the re-rejection is attributed and withheld")
	require.Contains(t, toolResultText(t, after[0]), reasonRejectedByProvider)
}

func TestNoToolReexecution(t *testing.T) {
	prepared := fantasy.PrepareStepResult{Tools: []fantasy.AgentTool{&stubAgentTool{}}}
	applyMediaRetryToolPolicy(&prepared, true)
	require.True(t, prepared.DisableAllTools, "a retry after tool results cannot start a tool")
	require.Empty(t, prepared.Tools)

	ordinary := fantasy.PrepareStepResult{Tools: []fantasy.AgentTool{&stubAgentTool{}}}
	applyMediaRetryToolPolicy(&ordinary, false)
	require.False(t, ordinary.DisableAllTools)
	require.Len(t, ordinary.Tools, 1)
}

func TestNoBase64InLogs(t *testing.T) {
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(previous)

	payload := provenancePNG(9)
	mediaID := mediaPartID(payload, "image/png")
	logMediaRejection("s", provenanceModel(), mediaID, reasonRejectedByProvider, "provider_error")
	logged := buffer.String()

	require.Contains(t, logged, hashPrefix(mediaID))
	require.NotContains(t, logged, base64.StdEncoding.EncodeToString(payload))
	require.NotContains(t, logged, string(payload))
}

func TestDoubleEncodedHashStable(t *testing.T) {
	raw := provenancePNG(5)
	once := base64.StdEncoding.EncodeToString(raw)
	twice := base64.StdEncoding.EncodeToString([]byte(once))

	first, ok := mediaIDForStoredData(once, "image/png")
	require.True(t, ok)
	second, ok := mediaIDForStoredData(twice, "image/png")
	require.True(t, ok)
	require.Equal(t, first, second, "the identity describes the image, not the storage accident")
}

func TestNativeProviderWithholdsOnlyRejected(t *testing.T) {
	env := testEnv(t)
	agent := testSessionAgent(env, nil, nil, "test prompt").(*sessionAgent)
	model := Model{
		ModelCfg:   config.SelectedModel{Provider: string(catwalk.InferenceProviderAnthropic), Model: "claude"},
		CatwalkCfg: catwalk.Model{SupportsImages: true},
	}
	good, bad := storedPNG(6), storedPNG(7)
	badID, ok := mediaIDForStoredData(bad, "image/png")
	require.True(t, ok)
	agent.recordMediaVerdict("s", model, badID, reasonRejectedByProvider, "test")

	result := agent.workaroundProviderMediaLimitations([]fantasy.Message{
		imageToolMessage("good", "image/png", good),
		imageToolMessage("bad", "image/png", bad),
	}, model, true, "s")

	require.Len(t, result, 2, "native providers keep tool-result media in place")
	require.Equal(t, 0, filePartCount(result))
	goodResult, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](result[0].Content[0])
	require.True(t, ok)
	_, goodIsMedia := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](goodResult.Output)
	require.True(t, goodIsMedia, "the good image stays native media")
	require.Contains(t, toolResultText(t, result[1]), reasonRejectedByProvider)
}

func TestModelUnsupportedAndNonImageNotes(t *testing.T) {
	env := testEnv(t)
	agent := testSessionAgent(env, nil, nil, "test prompt").(*sessionAgent)
	model := provenanceModel()

	unsupported := agent.workaroundProviderMediaLimitations(
		[]fantasy.Message{imageToolMessage("call", "image/png", storedPNG(8))}, model, false, "s")
	require.Contains(t, toolResultText(t, unsupported[0]), reasonModelUnsupported)
	require.Equal(t, 0, filePartCount(unsupported))

	nonImage := agent.workaroundProviderMediaLimitations(
		[]fantasy.Message{imageToolMessage("call", "application/json", base64.StdEncoding.EncodeToString([]byte(`{}`)))},
		model, true, "s")
	require.Contains(t, toolResultText(t, nonImage[0]), reasonNonImageAttachment)
	require.Equal(t, 0, filePartCount(nonImage))
}

func TestFilterProviderMedia(t *testing.T) {
	env := testEnv(t)
	agent := testSessionAgent(env, nil, nil, "test prompt").(*sessionAgent)
	model := provenanceModel()
	good := provenancePNG(10)
	bad := provenancePNG(11)
	badID := mediaPartID(bad, "image/png")
	agent.recordMediaVerdict("s", model, badID, reasonRejectedByProvider, "test")

	prompt, files := agent.filterProviderMedia("look", []fantasy.FilePart{
		{Data: good, MediaType: "image/png", Filename: "good.png"},
		{Data: bad, MediaType: "image/png", Filename: "bad.png"},
	}, "s", model, true)

	require.Len(t, files, 1)
	require.Equal(t, "good.png", files[0].Filename)
	require.Contains(t, prompt, reasonRejectedByProvider)
	require.Contains(t, prompt, "bad.png")
	require.False(t, strings.Contains(prompt, base64.StdEncoding.EncodeToString(bad)))
}
