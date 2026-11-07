package dialog

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/question"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/stretchr/testify/require"
)

func TestQuestionFormWaitsForAcknowledgment(t *testing.T) {
	t.Parallel()
	sty := styles.CharmtonePantera()
	f := NewQuestionForm(&sty, question.Request{ID: "batch", Questions: []question.Question{{ID: "q", Type: question.TypeFreeText, Text: "What should change?", Description: "Describe the change."}}})
	f.SetFocused(true)
	f.HandlePaste(tea.PasteMsg{Content: "Keep my answer"})
	var submitted []question.Answer
	f.OnAnswer = func(a []question.Answer) { submitted = a }
	done, _ := f.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.False(t, done, "the form must remain until delivery is acknowledged")
	require.Equal(t, "Keep my answer", submitted[0].FillInText)
	require.True(t, f.Submitting())
	f.SubmissionFailed("Temporary delivery failure")
	second, _ := f.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.False(t, second)
	require.Equal(t, "Keep my answer", submitted[0].FillInText, "failed delivery keeps the full answer for retry")
}
