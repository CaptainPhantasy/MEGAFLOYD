package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/legacy-ai/floyd/internal/megafloydhandoff"
	"github.com/legacy-ai/floyd/internal/proto"
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
		return decodeHandoff(rsp, "handoff")
	}
	rsp, err := c.post(ctx, path, nil, jsonBody(map[string]string{"target": target}), nil)
	if err != nil {
		return megafloydhandoff.Descriptor{}, err
	}
	defer rsp.Body.Close()
	return decodeHandoff(rsp, "handoff")
}

// EnsureHandoffWeb leaves browser opening to the caller (for desktop embedding).
//
// A failed handoff must never look like a successful one. The server answers a
// refusal with an error envelope, and the old code decoded that envelope into a
// zero-valued descriptor while reporting success — which handed the desktop an
// empty web address and turned every startup into a restart loop.
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
	result, err := decodeHandoff(rsp, "web handoff")
	if err != nil {
		return megafloydhandoff.Descriptor{}, err
	}
	if strings.TrimSpace(result.WebURL) == "" {
		return megafloydhandoff.Descriptor{}, errors.New("web handoff returned an empty web address")
	}
	return result, nil
}

// decodeHandoff turns an HTTP response into a descriptor, converting any
// unsuccessful status into an error that preserves the server's own message.
func decodeHandoff(rsp *http.Response, what string) (megafloydhandoff.Descriptor, error) {
	if rsp.StatusCode != http.StatusOK {
		var failure proto.Error
		if err := json.NewDecoder(rsp.Body).Decode(&failure); err == nil && failure.Message != "" {
			return megafloydhandoff.Descriptor{}, fmt.Errorf("%s failed: %s", what, failure.Message)
		}
		return megafloydhandoff.Descriptor{}, fmt.Errorf("%s failed: %s", what, rsp.Status)
	}
	var result megafloydhandoff.Descriptor
	if err := json.NewDecoder(rsp.Body).Decode(&result); err != nil {
		return megafloydhandoff.Descriptor{}, fmt.Errorf("%s returned an unreadable result: %w", what, err)
	}
	return result, nil
}
