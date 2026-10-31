package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/legacy-ai/floyd/internal/config"
)

func (c *Client) ApplyRuntimeOptions(ctx context.Context, id string, options config.RuntimeOptions) error {
	rsp, err := c.post(ctx, fmt.Sprintf("/workspaces/%s/config/runtime-options", id), nil, jsonBody(options), http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("runtime options rejected: status %d", rsp.StatusCode)
	}
	return nil
}
