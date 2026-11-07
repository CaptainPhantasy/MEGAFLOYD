package agent

import (
	"testing"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func TestNamedChildRecoveryRejectsMissingOrMalformedIdentity(t *testing.T) {
	for _, test := range []struct {
		name     string
		identity any
	}{
		{name: "missing"},
		{name: "malformed", identity: "not an agent snapshot"},
		{name: "missing restrictions", identity: map[string]any{"version": 1, "definition": map[string]any{"id": "task", "model": "small"}}},
		{name: "unknown version", identity: map[string]any{"version": 9, "definition": map[string]any{"id": "task", "model": "small"}, "tools": []string{"view"}, "mcp": map[string][]string{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := newToolSetTestCoordinator(t)
			parent, err := c.sessions.Create(t.Context(), "parent")
			require.NoError(t, err)
			id := c.sessions.CreateAgentToolSessionID(uuid.NewString(), uuid.NewString())
			_, err = c.sessions.CreateTaskSession(t.Context(), id, parent.ID, "saved child")
			require.NoError(t, err)
			if test.identity != nil {
				_, err = c.sessions.(session.MetadataMutator).MutateMetadata(t.Context(), id, func(metadata map[string]any) error {
					metadata[runtimeAgentMetadataKey] = test.identity
					return nil
				})
				require.NoError(t, err)
			}
			child, wasChild, err := c.restoreRuntimeChild(t.Context(), id)
			require.True(t, wasChild)
			require.Error(t, err)
			require.Nil(t, child)
			_, exists := c.RuntimeChildAgent(id)
			require.False(t, exists, "invalid saved identity cannot create a coder fallback")
		})
	}
}
