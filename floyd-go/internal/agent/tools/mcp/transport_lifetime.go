package mcp

import (
	"context"
	"io"
	"net/http"
	"sync"
)

type lifetimeRoundTripper struct {
	ctx   context.Context
	inner http.RoundTripper
}

func (rt lifetimeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := rt.ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(rt.ctx, cancel)
	cleanup := func() { stop(); cancel() }
	response, err := rt.inner.RoundTrip(req.Clone(ctx))
	if err != nil {
		cleanup()
		return nil, err
	}
	if response.Body == nil {
		cleanup()
		return response, nil
	}
	response.Body = &lifetimeBody{ReadCloser: response.Body, cleanup: cleanup}
	return response, nil
}

type lifetimeBody struct {
	io.ReadCloser
	once    sync.Once
	cleanup func()
}

func (body *lifetimeBody) Close() error {
	err := body.ReadCloser.Close()
	body.once.Do(body.cleanup)
	return err
}
