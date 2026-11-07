package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/legacy-ai/floyd/internal/proto"
)

func (c *Client) RenameSession(ctx context.Context, wid, sid, title string) error {
	var updated proto.Session
	return c.runtimeControl(ctx, fmt.Sprintf("/workspaces/%s/sessions/%s/metadata", url.PathEscape(wid), url.PathEscape(sid)), proto.SessionMetadataRequest{Title: &title}, &updated)
}
