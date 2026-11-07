package agent

import (
	"testing"

	"github.com/legacy-ai/floyd/internal/message"
	"github.com/stretchr/testify/require"
)

func TestSummaryInstructionReachesModelWithoutBecomingPolicy(t *testing.T) {
	env := testEnv(t)
	sess, err := env.sessions.Create(t.Context(), "compact instruction")
	require.NoError(t, err)
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Original user requirement"}}})
	require.NoError(t, err)
	model := &intelligenceModel{finishStreamModel: finishStreamModel{text: "Summary with acceptance."}}
	child := testSessionAgent(env, model, model, "base")
	instruction := "Preserve all acceptance conditions verbatim."
	require.NoError(t, child.Summarize(WithSummaryInstruction(t.Context(), instruction), sess.ID, nil, nil))
	require.Contains(t, intelligenceCallText(model.calls[0]), instruction)
	require.Empty(t, summaryInstruction(t.Context()), "summary preferences must not persist into later operations")
}
