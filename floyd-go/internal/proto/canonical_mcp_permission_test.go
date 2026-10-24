package proto_test

import (
	"encoding/json"
	"testing"

	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/stretchr/testify/require"
)

// MCP sends its tool input as a JSON string in Params. Both the SSE request
// and the approval posted back by a client must preserve that value.
func TestCanonicalMCPPermissionRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		params any
	}{
		{"MCP_JSON_string", `{"query":"canonical gateway status"}`},
		{"object", map[string]any{"query": "canonical gateway status"}},
		{"array", []any{"status", true}},
		{"boolean", true},
		{"number", float64(7)},
		{"null", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := proto.PermissionRequest{
				ID: "canonical-permission", SessionID: "canonical-session",
				ToolCallID: "canonical-call", ToolName: "mcp_inference_gateway_execute",
				Action: "execute", Params: tc.params, Path: "/canonical/workspace",
			}
			wire, err := json.Marshal(request)
			require.NoError(t, err)
			var received proto.PermissionRequest
			require.NoError(t, json.Unmarshal(wire, &received), "SSE permission request")
			require.Equal(t, request, received)

			wire, err = json.Marshal(proto.PermissionGrant{Permission: received, Action: proto.PermissionAllow})
			require.NoError(t, err)
			var grant proto.PermissionGrant
			require.NoError(t, json.Unmarshal(wire, &grant), "client approval POST")
			require.Equal(t, request, grant.Permission)
			require.Equal(t, proto.PermissionAllow, grant.Action)

			wire, err = json.Marshal(proto.CreatePermissionRequest{
				SessionID: request.SessionID, ToolCallID: request.ToolCallID,
				ToolName: request.ToolName, Params: tc.params,
			})
			require.NoError(t, err)
			var created proto.CreatePermissionRequest
			require.NoError(t, json.Unmarshal(wire, &created))
			require.Equal(t, tc.params, created.Params)
		})
	}
	t.Run("native_types_remain_validated", func(t *testing.T) {
		params := proto.BashPermissionsParams{Command: "pwd", WorkingDir: "/canonical/workspace"}
		request := proto.PermissionRequest{ToolName: proto.BashToolName, Params: params}
		wire, err := json.Marshal(request)
		require.NoError(t, err)
		var received proto.PermissionRequest
		require.NoError(t, json.Unmarshal(wire, &received))
		require.Equal(t, params, received.Params)

		request.Params = "invalid native parameter type"
		wire, err = json.Marshal(request)
		require.NoError(t, err)
		require.Error(t, json.Unmarshal(wire, &received))
	})
}
