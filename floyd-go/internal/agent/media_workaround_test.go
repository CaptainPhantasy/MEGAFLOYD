package agent

import (
	"encoding/base64"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
)

func imageToolMessage(callID, mediaType, payload string) fantasy.Message {
	return fantasy.Message{
		Role: fantasy.MessageRoleTool,
		Content: []fantasy.MessagePart{
			fantasy.ToolResultPart{
				ToolCallID: callID,
				Output: fantasy.ToolResultOutputContentMedia{
					Data:      payload,
					MediaType: mediaType,
				},
			},
		},
	}
}

func TestWorkaroundProviderMediaLimitations_NonImageMediaStaysText(t *testing.T) {
	env := testEnv(t)
	agent := testSessionAgent(env, nil, nil, "test prompt").(*sessionAgent)

	payload := base64.StdEncoding.EncodeToString([]byte(`{"key":"value"}`))
	messages := []fantasy.Message{imageToolMessage("call_json", "application/json", payload)}
	largeModel := Model{
		ModelCfg:   config.SelectedModel{Provider: "deepseek"},
		CatwalkCfg: catwalk.Model{SupportsImages: true},
	}

	result := agent.workaroundProviderMediaLimitations(messages, largeModel, largeModel.CatwalkCfg.SupportsImages, "s")

	require.Len(t, result, 1, "non-image media must not create a file-part user message")
	require.Equal(t, fantasy.MessageRoleTool, result[0].Role)
	tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](result[0].Content[0])
	require.True(t, ok)
	text, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](tr.Output)
	require.True(t, ok)
	require.Contains(t, text.Text, "non_image_attachment")
}

func TestWorkaroundProviderMediaLimitations_MediaFollowsWholeToolRun(t *testing.T) {
	env := testEnv(t)
	agent := testSessionAgent(env, nil, nil, "test prompt").(*sessionAgent)

	payload := base64.StdEncoding.EncodeToString([]byte("fake-png-data"))
	messages := []fantasy.Message{
		imageToolMessage("call_1", "image/png", payload),
		imageToolMessage("call_2", "image/png", payload),
		fantasy.NewUserMessage("continue"),
	}
	largeModel := Model{
		ModelCfg:   config.SelectedModel{Provider: "deepseek"},
		CatwalkCfg: catwalk.Model{SupportsImages: true},
	}

	result := agent.workaroundProviderMediaLimitations(messages, largeModel, largeModel.CatwalkCfg.SupportsImages, "s")

	// The two tool results stay adjacent; the media user messages are emitted
	// only after the run closes, before the original user message.
	require.Len(t, result, 5)
	require.Equal(t, fantasy.MessageRoleTool, result[0].Role)
	require.Equal(t, fantasy.MessageRoleTool, result[1].Role)
	require.Equal(t, fantasy.MessageRoleUser, result[2].Role)
	require.Equal(t, fantasy.MessageRoleUser, result[3].Role)
	require.Equal(t, fantasy.MessageRoleUser, result[4].Role)
	files := 0
	for _, msg := range result[2:4] {
		for _, part := range msg.Content {
			if _, ok := fantasy.AsMessagePart[fantasy.FilePart](part); ok {
				files++
			}
		}
	}
	require.Equal(t, 2, files)
}

func TestDecodeMediaData_RepairsDoubleEncodedImage(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 16)...)
	once := base64.StdEncoding.EncodeToString(png)
	twice := base64.StdEncoding.EncodeToString([]byte(once))

	for name, payload := range map[string]string{"single": once, "double": twice} {
		t.Run(name, func(t *testing.T) {
			decoded, err := decodeMediaData(payload)
			require.NoError(t, err)
			require.Equal(t, png, decoded)
		})
	}

	_, err := decodeMediaData("not-base64!!!")
	require.Error(t, err)
}

func TestIsUnsupportedImageError(t *testing.T) {
	rejected := &fantasy.ProviderError{
		Title:   "Bad Request",
		Message: ".messages[140].content[1]: You have uploaded an unsupported image. Please make sure your image is valid and in a supported image format.",
	}
	require.True(t, isUnsupportedImageError(rejected))
	require.False(t, isUnsupportedImageError(&fantasy.ProviderError{Title: "Rate limit", Message: "too many requests"}))
	require.False(t, isUnsupportedImageError(nil))
}
