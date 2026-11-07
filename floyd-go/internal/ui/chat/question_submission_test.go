package chat

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/stretchr/testify/require"
)

func TestExpandedQuestionKeepsCompleteAnswerAndNotes(t *testing.T) {
	t.Parallel()
	sty := styles.CharmtonePantera()
	answer := "Keep the entire answer including the final requirement"
	note := "Also retain the final sentence of this explanatory note"
	rendered := ansi.Strip(formatQuestionAnswers(&sty, "Q1: What should change?\nUser provided: "+answer+"\n\nNotes:\n- "+note, 24))
	normalized := strings.Join(strings.Fields(rendered), " ")
	require.Contains(t, normalized, answer)
	require.Contains(t, normalized, note)
}
