package megafloydweb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
)

func verifyRuntime(ctx context.Context, host string, port int, opts Options) error {
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, HTTPOrigin(host, port)+"/api/v1/runtime", nil)
	if err != nil {
		return err
	}
	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("runtime identity returned %d", rsp.StatusCode)
	}
	var result struct {
		Code int `json:"code"`
		Data struct {
			Engine      string `json:"engine"`
			Host        string `json:"host"`
			CWD         string `json:"cwd"`
			DataDir     string `json:"data_dir"`
			WorkspaceID string `json:"workspace_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(rsp.Body, 16<<10)).Decode(&result); err != nil {
		return err
	}
	actual := result.Data
	if result.Code != 0 || actual.Engine != "go" || actual.Host != opts.GoHost || actual.WorkspaceID == "" ||
		!sameRuntimePath(actual.CWD, opts.CWD) || !sameRuntimePath(actual.DataDir, opts.GoDataDir) {
		return fmt.Errorf("expected Go backend %s with workspace %s and data %s", opts.GoHost, opts.CWD, opts.GoDataDir)
	}
	return nil
}

func sameRuntimePath(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	if real, err := filepath.EvalSymlinks(a); err == nil {
		a = real
	}
	if real, err := filepath.EvalSymlinks(b); err == nil {
		b = real
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
