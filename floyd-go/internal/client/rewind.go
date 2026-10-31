package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/rewind"
)

func (c *Client) RewindSession(ctx context.Context, wid, sid string, count int) (*rewind.Result, error) {
	var result rewind.Result
	err := c.runtimeControl(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/rewind", url.PathEscape(wid), url.PathEscape(sid)), proto.SessionUndoRequest{Count: count}, &result)
	return &result, err
}
