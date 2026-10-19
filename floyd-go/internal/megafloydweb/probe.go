package megafloydweb

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

const probeTimeout = 750 * time.Millisecond

// probeHealth reports whether a Floyd Code web server answers its
// unauthenticated health route on host:port.
func probeHealth(ctx context.Context, host string, port int) bool {
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	target := "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/api/v1/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			OK bool `json:"ok"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&envelope); err != nil {
		return false
	}
	return envelope.Code == 0 && envelope.Data.OK
}
