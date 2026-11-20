package message

import (
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestToAIMessage_InlinesStructuredTextAndKeepsImagesAsFiles(t *testing.T) {
	msg := &Message{
		Role: User,
		Parts: []ContentPart{
			TextContent{Text: "hello"},
			BinaryContent{Path: "brief.json", MIMEType: "application/json", Data: []byte(`{"a":1}`)},
			BinaryContent{Path: "shot.png", MIMEType: "image/png", Data: []byte("pngbytes")},
		},
	}

	messages := msg.ToAIMessage()
	require.Len(t, messages, 1)

	var text string
	var files int
	for _, part := range messages[0].Content {
		switch value := part.(type) {
		case fantasy.TextPart:
			text = value.Text
		case fantasy.FilePart:
			files++
		}
	}
	require.Contains(t, text, `{"a":1}`)
	require.Contains(t, text, "brief.json")
	require.Equal(t, 1, files, "only the image becomes a file part")
}

func TestIsTextualMIME(t *testing.T) {
	for _, mediaType := range []string{
		"text/plain", "text/markdown", "application/json", "application/vnd.api+json",
		"application/yaml", "application/xml", "application/ld+json; charset=utf-8",
		"application/toml", "application/javascript",
	} {
		require.True(t, IsTextualMIME(mediaType), mediaType)
	}
	for _, mediaType := range []string{
		"image/png", "application/pdf", "application/octet-stream", "audio/wav", "video/mp4",
	} {
		require.False(t, IsTextualMIME(mediaType), mediaType)
	}
}
