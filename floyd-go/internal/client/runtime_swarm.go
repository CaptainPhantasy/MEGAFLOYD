package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/legacy-ai/floyd/internal/proto"
)

// GetSwarm reads the session's parallel-dispatch state from the server.
func (c *Client) GetSwarm(ctx context.Context, id, sessionID string) (proto.SwarmState, error) {
	var out proto.SwarmState
	rsp, err := c.get(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/swarm", id, sessionID), nil, nil)
	if err != nil {
		return out, fmt.Errorf("failed to get swarm state: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("failed to get swarm state: status code %d", rsp.StatusCode)
	}
	if err := json.NewDecoder(rsp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("failed to decode swarm state: %w", err)
	}
	return out, nil
}

// SetSwarm updates the session's parallel-dispatch state on the server.
// Nil fields are left unchanged.
func (c *Client) SetSwarm(ctx context.Context, id, sessionID string, active *bool, parallel *int) (proto.SwarmState, error) {
	var out proto.SwarmState
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/swarm", id, sessionID), nil,
		jsonBody(proto.RuntimeSwarmRequest{Active: active, MaxParallel: parallel}),
		http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return out, fmt.Errorf("failed to set swarm state: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("failed to set swarm state: status code %d", rsp.StatusCode)
	}
	if err := json.NewDecoder(rsp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("failed to decode swarm state: %w", err)
	}
	return out, nil
}
