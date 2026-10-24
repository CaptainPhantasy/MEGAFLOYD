package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/legacy-ai/floyd/internal/proto"
)

func (c *Client) SuggestPrompt(ctx context.Context, workspaceID, sessionID, prefix string) (string, error) {
	path := fmt.Sprintf("/workspaces/%s/agent/sessions/%s/suggestion", url.PathEscape(workspaceID), url.PathEscape(sessionID))
	rsp, err := c.post(ctx, path, nil, jsonBody(proto.SuggestPromptRequest{Prefix: prefix}), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return "", fmt.Errorf("suggest prompt: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("suggest prompt: status %d: %s", rsp.StatusCode, decodeErrorMessage(rsp.Body))
	}
	var result proto.SuggestPromptResponse
	if err := json.NewDecoder(rsp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode suggestion: %w", err)
	}
	return result.Suggestion, nil
}
