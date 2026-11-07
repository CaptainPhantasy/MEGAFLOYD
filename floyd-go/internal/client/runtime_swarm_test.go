package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/stretchr/testify/require"
)

// TestSwarmRoundTrip pins the wire contract of the swarm endpoint: the
// path the TUI calls, the request fields, and the decoded state.
func TestSwarmRoundTrip(t *testing.T) {
	t.Parallel()

	var gotPath, gotMethod string
	var got proto.RuntimeSwarmRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &got))
			require.NoError(t, json.NewEncoder(w).Encode(proto.SwarmState{Active: true, MaxParallel: 5}))
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(proto.SwarmState{Active: false, MaxParallel: 3}))
	}))
	defer srv.Close()

	cl, err := NewClient("", "tcp", strings.TrimPrefix(srv.URL, "http://"))
	require.NoError(t, err)

	st, err := cl.GetSwarm(t.Context(), "ws-1", "sess-1")
	require.NoError(t, err)
	require.Equal(t, "/v1/workspaces/ws-1/sessions/sess-1/swarm", gotPath)
	require.Equal(t, http.MethodGet, gotMethod)
	require.False(t, st.Active)
	require.Equal(t, 3, st.MaxParallel)

	active, parallel := true, 5
	st, err = cl.SetSwarm(t.Context(), "ws-1", "sess-1", &active, &parallel)
	require.NoError(t, err)
	require.Equal(t, http.MethodPost, gotMethod)
	require.NotNil(t, got.Active)
	require.True(t, *got.Active)
	require.NotNil(t, got.MaxParallel)
	require.Equal(t, 5, *got.MaxParallel)
	require.True(t, st.Active)
	require.Equal(t, 5, st.MaxParallel)
}
