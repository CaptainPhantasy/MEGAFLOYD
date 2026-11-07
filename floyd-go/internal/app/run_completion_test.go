package app

import (
	"testing"

	"github.com/legacy-ai/floyd/internal/agent/notify"
	"github.com/legacy-ai/floyd/internal/pubsub"
	"github.com/stretchr/testify/require"
)

func TestRunCompletionSkipsOtherSessionsAndRuns(t *testing.T) {
	events := make(chan pubsub.Event[notify.RunComplete], 3)
	for _, entry := range []notify.RunComplete{{SessionID: "child", RunID: "mine", Text: "child text"}, {SessionID: "session", RunID: "other", Text: "other text"}, {SessionID: "session", RunID: "mine", Text: "own text"}} {
		events <- pubsub.Event[notify.RunComplete]{Payload: entry}
	}
	result, ok := takeRunCompletion(events, "session", "mine")
	require.True(t, ok)
	require.Equal(t, "own text", result.Text)
	_, ok = takeRunCompletion(events, "session", "mine")
	require.False(t, ok)
	close(events)
	_, ok = takeRunCompletion(events, "session", "mine")
	require.False(t, ok)
}
