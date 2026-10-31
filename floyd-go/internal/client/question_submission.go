package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/legacy-ai/floyd/internal/proto"
)

// CancelQuestionRequest cancels only the form the user actually dismissed.
func (c *Client) CancelQuestionRequest(ctx context.Context, workspaceID, requestID string) (bool, error) {
	request := struct {
		BatchRequestID string `json:"batch_request_id"`
	}{requestID}
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/questions/cancel", workspaceID), nil, jsonBody(request), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return false, fmt.Errorf("cancel question: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return false, questionResponseError(rsp)
	}
	var response proto.QuestionAnswerResponse
	if err := json.NewDecoder(rsp.Body).Decode(&response); err != nil {
		return false, fmt.Errorf("read cancellation acknowledgment: %w", err)
	}
	return response.Resolved, nil
}

func questionResponseError(response *http.Response) error {
	var detail proto.Error
	if err := json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&detail); err == nil && detail.Message != "" {
		return fmt.Errorf("question submission: %s", detail.Message)
	}
	return fmt.Errorf("question submission: status code %d", response.StatusCode)
}
