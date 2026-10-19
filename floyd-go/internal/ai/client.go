package ai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	ferrors "github.com/legacy-ai/floyd/internal/errors"
)

type RetryOptions struct {
	MaxRetries     int
	InitialDelayMs int
	MaxDelayMs     int
}

type ClientConfig struct {
	APIBaseURL         string
	APIVersion         string
	Timeout            time.Duration
	Retry              RetryOptions
	DefaultModel       string
	DefaultMaxTokens   int
	DefaultTemperature float64
}

var defaultClientConfig = ClientConfig{
	APIBaseURL:         "https://api.legacyai.com",
	APIVersion:         "2023-06-01",
	Timeout:            60 * time.Second,
	Retry:              RetryOptions{MaxRetries: 3, InitialDelayMs: 1000, MaxDelayMs: 10000},
	DefaultModel:       "floyd-3-FLOYD-PINK-20240229",
	DefaultMaxTokens:   4096,
	DefaultTemperature: 0.7,
}

type Client struct {
	config    ClientConfig
	authToken string
	http      *http.Client
}

func NewClient(config ClientConfig, authToken string) *Client {
	cfg := defaultClientConfig
	if config.APIBaseURL != "" {
		cfg.APIBaseURL = config.APIBaseURL
	}
	if config.APIVersion != "" {
		cfg.APIVersion = config.APIVersion
	}
	if config.Timeout > 0 {
		cfg.Timeout = config.Timeout
	}
	if config.Retry.MaxRetries != 0 {
		cfg.Retry = config.Retry
	}
	if config.DefaultModel != "" {
		cfg.DefaultModel = config.DefaultModel
	}
	if config.DefaultMaxTokens != 0 {
		cfg.DefaultMaxTokens = config.DefaultMaxTokens
	}
	if config.DefaultTemperature != 0 {
		cfg.DefaultTemperature = config.DefaultTemperature
	}

	return &Client{
		config:    cfg,
		authToken: authToken,
		http: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (c *Client) Complete(prompt string, options CompletionOptions) (*CompletionResponse, error) {
	messages := []Message{{Role: RoleUser, Content: prompt}}
	return c.CompleteMessages(messages, options)
}

func (c *Client) CompleteMessages(messages []Message, options CompletionOptions) (*CompletionResponse, error) {
	request := CompletionRequest{
		Model:    c.defaultModel(options.Model),
		Messages: messages,
		Stream:   false,
	}
	if options.MaxTokens != nil {
		request.MaxTokens = options.MaxTokens
	} else {
		defaultMax := c.config.DefaultMaxTokens
		request.MaxTokens = &defaultMax
	}
	if options.Temperature != nil {
		request.Temperature = options.Temperature
	} else {
		defaultTemp := c.config.DefaultTemperature
		request.Temperature = &defaultTemp
	}
	if options.TopP != nil {
		request.TopP = options.TopP
	}
	if options.TopK != nil {
		request.TopK = options.TopK
	}
	if len(options.StopSequences) > 0 {
		request.StopSequences = options.StopSequences
	}
	if options.System != "" {
		request.System = options.System
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	responseBody, err := c.sendRequest(buildURL(c.config.APIBaseURL, "/v1/messages"), payload)
	if err != nil {
		return nil, err
	}

	var response CompletionResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) CompleteStream(messages []Message, options CompletionOptions, onEvent func(StreamEvent)) error {
	request := CompletionRequest{
		Model:    c.defaultModel(options.Model),
		Messages: messages,
		Stream:   true,
	}
	if options.MaxTokens != nil {
		request.MaxTokens = options.MaxTokens
	}
	if options.Temperature != nil {
		request.Temperature = options.Temperature
	}
	if options.TopP != nil {
		request.TopP = options.TopP
	}
	if options.TopK != nil {
		request.TopK = options.TopK
	}
	if len(options.StopSequences) > 0 {
		request.StopSequences = options.StopSequences
	}
	if options.System != "" {
		request.System = options.System
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}

	url := buildURL(c.config.APIBaseURL, "/v1/messages")
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.applyHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return c.handleErrorResponse(resp)
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "data: [DONE]" {
			continue
		}
		if strings.HasPrefix(trimmed, "data: ") {
			payload := strings.TrimPrefix(trimmed, "data: ")
			var event StreamEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				slog.Warn("Failed to parse stream event", "error", err)
				continue
			}
			onEvent(event)
		}
	}
}

func (c *Client) TestConnection() bool {
	_, err := c.Complete("Hello", CompletionOptions{MaxTokens: intPtr(10), Temperature: floatPtr(0)})
	return err == nil
}

func (c *Client) Disconnect() error {
	return nil
}

func (c *Client) sendRequest(path string, payload []byte) ([]byte, error) {
	// If path is already a full URL (starts with http:// or https://), use it directly
	// Otherwise, prepend the base URL
	var url string
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		url = path
	} else {
		url = c.config.APIBaseURL + path
	}
	attempts := c.config.Retry.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		c.applyHeaders(req)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if i < attempts-1 {
				time.Sleep(c.retryDelay(i))
				continue
			}
			return nil, err
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if i < attempts-1 {
				time.Sleep(c.retryDelay(i))
				continue
			}
			return nil, readErr
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			err = c.parseErrorResponse(resp.StatusCode, body)
			lastErr = err
			if i < attempts-1 {
				time.Sleep(c.retryDelay(i))
				continue
			}
			return nil, err
		}

		return body, nil
	}

	return nil, lastErr
}

func (c *Client) parseErrorResponse(status int, body []byte) error {
	message := fmt.Sprintf("API request failed with status %d", status)
	if len(body) > 0 {
		message = fmt.Sprintf("API request failed: %s", strings.TrimSpace(string(body)))
	}
	return ferrors.CreateUserError(message, ferrors.UserErrorOptions{
		Category: ferrors.ErrorCategoryAPI,
		Resolution: []string{
			"Check your API key and try again.",
		},
	})
}

func (c *Client) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return c.parseErrorResponse(resp.StatusCode, body)
}

func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("X-Api-Key", c.authToken)
	}
	if c.config.APIVersion != "" {
		req.Header.Set("Legacy AI-version", c.config.APIVersion)
	}
	req.Header.Set("User-Agent", "floyd-code-cli")
}

func (c *Client) retryDelay(attempt int) time.Duration {
	delay := time.Duration(c.config.Retry.InitialDelayMs) * time.Millisecond
	if delay == 0 {
		delay = time.Second
	}
	for i := 0; i < attempt; i++ {
		delay *= 2
		if max := time.Duration(c.config.Retry.MaxDelayMs) * time.Millisecond; max > 0 && delay > max {
			return max
		}
	}
	return delay
}

func (c *Client) defaultModel(model string) string {
	if model != "" {
		return model
	}
	return c.config.DefaultModel
}

// buildURL constructs a full URL from base and path, avoiding duplicate version segments.
// If baseURL already ends with /v1, /v4, etc., and path starts with /v1, it avoids duplication.
func buildURL(baseURL, path string) string {
	// Remove trailing slash from base
	baseURL = strings.TrimSuffix(baseURL, "/")
	// Remove leading slash from path
	path = strings.TrimPrefix(path, "/")

	// Check if base already contains a version path (like /v1, /v4, /paas/v4)
	hasVersionInPath := false
	versionPatterns := []string{"/v1/", "/v4/", "/v1/", "/paas/v", "/api/v"}
	for _, pattern := range versionPatterns {
		if strings.Contains(baseURL, pattern) || strings.HasSuffix(baseURL, strings.TrimSuffix(pattern, "/")) {
			hasVersionInPath = true
			break
		}
	}

	// If base has version path and path starts with v1/messages or v1/chat, use path as-is
	if hasVersionInPath && (strings.HasPrefix(path, "v1/") || strings.HasPrefix(path, "chat/")) {
		return baseURL + "/" + path
	}

	// Default: append path directly
	return baseURL + "/" + path
}

func intPtr(value int) *int {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}
