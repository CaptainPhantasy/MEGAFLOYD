package chat

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/stretchr/testify/require"
)

func TestAssistantPlainStyleChangesRenderedOutput(t *testing.T) {
	sty := styles.CharmtonePantera()
	msg := finishedAssistantMessage("style", "**bold** and `code`")
	plain := NewAssistantMessageItem(&sty, msg, "plain").(*AssistantMessageItem)
	markdown := NewAssistantMessageItem(&sty, msg, "markdown").(*AssistantMessageItem)
	require.Contains(t, ansi.Strip(plain.Render(80)), "**bold**")
	require.NotContains(t, ansi.Strip(markdown.Render(80)), "**bold**")
	require.NotEqual(t, plain.Render(80), markdown.Render(80))
}
