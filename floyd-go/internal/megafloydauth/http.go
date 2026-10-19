package megafloydauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultDeviceAuthorizationPath is the device-authorization endpoint.
	DefaultDeviceAuthorizationPath = "/api/oauth/device_authorization"
	// DefaultTokenPath serves both the device-code grant and refresh.
	DefaultTokenPath = "/api/oauth/token"

	// DeviceCodeGrantType is the RFC 8628 device-code grant identifier.
	DeviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"

	// DefaultHTTPTimeout bounds one HTTP request. The donor uses 30s.
	DefaultHTTPTimeout = 30 * time.Second

	// maxResponseBytes caps how much of a response body is read.
	maxResponseBytes = 1 << 20
)

// endpointURL joins the OAuth host with a configured or default path.
func (c Config) endpointURL(configuredPath, defaultPath string) (string, error) {
	host := normalizeEndpoint(c.OAuthHost)
	if host == "" {
		return "", fmt.Errorf("%w: OAuth host is empty", ErrNotConfigured)
	}
	path := strings.TrimSpace(configuredPath)
	if path == "" {
		path = defaultPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return host + path, nil
}

// postForm sends one form-encoded POST and decodes a JSON object. A
// non-JSON body yields an empty map, matching the donor's tolerance.
// Transport failures wrap ErrConnection so refresh can classify them.
func (c *Client) postForm(
	ctx context.Context, endpoint string, params url.Values,
) (int, map[string]any, error) {
	headers, err := c.Config.Headers()
	if err != nil {
		return 0, nil, err
	}
	userAgent, err := c.Config.UserAgent()
	if err != nil {
		return 0, nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, DefaultHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx, http.MethodPost, endpoint, strings.NewReader(params.Encode()),
	)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: build request: %w", ErrConnection, err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %w", ErrConnection, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: read response: %w", ErrConnection, err)
	}
	data := map[string]any{}
	_ = json.Unmarshal(body, &data)
	return resp.StatusCode, data, nil
}

// httpClient resolves the injected client, then the Config client, then
// http.DefaultClient.
func (c *Client) httpClient() HTTPDoer {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	if c.Config.HTTPClient != nil {
		return c.Config.HTTPClient
	}
	return http.DefaultClient
}

// flexFloat accepts a JSON number, a numeric string, or null. The donor
// coerces numeric fields with Number(), which also accepts strings.
type flexFloat struct {
	value float64
	set   bool
}

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexFloat) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "null" {
		return nil
	}
	text = strings.Trim(text, `"`)
	number, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	f.value = number
	f.set = true
	return nil
}
