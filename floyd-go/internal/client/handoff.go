package client

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/legacy-ai/floyd/internal/megafloydhandoff"
)

// Handoff opens another view of an existing workspace without an agent turn.
func (c *Client) Handoff(ctx context.Context, workspaceID, sessionID, target string) (megafloydhandoff.Descriptor, error) {
	path := "/workspaces/" + url.PathEscape(workspaceID)
	if sessionID != "" {
		path += "/sessions/" + url.PathEscape(sessionID)
	}
	path += "/handoff"
	if target == "" {
		rsp, err := c.get(ctx, path, nil, nil)
		if err != nil {
			return megafloydhandoff.Descriptor{}, err
		}
		defer rsp.Body.Close()
		var result megafloydhandoff.Descriptor
		err = json.NewDecoder(rsp.Body).Decode(&result)
		return result, err
	}
	rsp, err := c.post(ctx, path, nil, jsonBody(map[string]string{"target": target}), nil)
	if err != nil {
		return megafloydhandoff.Descriptor{}, err
	}
	defer rsp.Body.Close()
	var result megafloydhandoff.Descriptor
	err = json.NewDecoder(rsp.Body).Decode(&result)
	return result, err
}

// EnsureHandoffWeb leaves browser opening to the caller (for desktop embedding).
func (c *Client) EnsureHandoffWeb(ctx context.Context, workspaceID, sessionID string, port int) (megafloydhandoff.Descriptor, error) {
	path := "/workspaces/" + url.PathEscape(workspaceID)
	if sessionID != "" {
		path += "/sessions/" + url.PathEscape(sessionID)
	}
	path += "/handoff"
	rsp, err := c.post(ctx, path, nil, jsonBody(map[string]any{"target": "web", "open": false, "preferred_port": port}), nil)
	if err != nil {
		return megafloydhandoff.Descriptor{}, err
	}
	defer rsp.Body.Close()
	var result megafloydhandoff.Descriptor
	err = json.NewDecoder(rsp.Body).Decode(&result)
	return result, err
}
