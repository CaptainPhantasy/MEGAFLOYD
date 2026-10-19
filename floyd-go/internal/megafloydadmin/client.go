package megafloydadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20

// TokenSource supplies a usable access token.
type TokenSource interface {
	AccessToken(context.Context) (string, error)
}

// HTTPDoer is the subset of *http.Client the client needs.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client calls the company admin plane.
type Client struct {
	baseURL      string
	http         HTTPDoer
	tokens       TokenSource
	usersPath    string
	invitesPath  string
	grantPath    string
	revokePath   string
	grantMethod  string
	revokeMethod string
	userAgent    string
	timeout      time.Duration
}

// New validates cfg and returns a client.
func New(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf(
			"%w: admin base URL is empty; set MEGAFLOYD_ADMIN_BASE_URL "+
				"or MEGAFLOYD_AUTH_HOST",
			ErrNotConfigured,
		)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf(
			"%w: invalid admin base URL %q", ErrNotConfigured, baseURL,
		)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf(
			"%w: admin base URL must be http or https", ErrNotConfigured,
		)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	baseURL = strings.TrimRight(parsed.String(), "/")
	if cfg.Tokens == nil {
		return nil, fmt.Errorf(
			"%w: admin client has no token source", ErrNotConfigured,
		)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultHTTPTimeout
	}
	doer := cfg.HTTPClient
	if doer == nil {
		doer = &http.Client{
			Timeout: timeout,
			// An admin API route never legitimately redirects. Refusing
			// redirects keeps the Bearer token on the configured origin
			// and surfaces a 3xx as an API error instead of HTML.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	return &Client{
		baseURL:      baseURL,
		http:         doer,
		tokens:       cfg.Tokens,
		usersPath:    normalizePath(cfg.UsersPath, DefaultUsersPath),
		invitesPath:  normalizePath(cfg.InvitesPath, DefaultInvitesPath),
		grantPath:    strings.TrimSpace(cfg.GrantPath),
		revokePath:   strings.TrimSpace(cfg.RevokePath),
		grantMethod:  normalizeMethod(cfg.GrantMethod),
		revokeMethod: normalizeMethod(cfg.RevokeMethod),
		userAgent:    firstNonEmpty(cfg.UserAgent, DefaultUserAgent),
		timeout:      timeout,
	}, nil
}

// User is one company account. The verified response shape is:
//
//	{"users": [{"id": "...", "username": "...", "enabled": true,
//	            "last_login_at": ...}]}
type User struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	Enabled     bool            `json:"enabled"`
	LastLoginAt json.RawMessage `json:"last_login_at"`
}

// LastLoginString renders last_login_at without assuming whether the
// server sends a string, a number, or null.
func (u User) LastLoginString() string {
	raw := strings.TrimSpace(string(u.LastLoginAt))
	if raw == "" || raw == "null" {
		return "never"
	}
	var text string
	if err := json.Unmarshal(u.LastLoginAt, &text); err == nil {
		if text == "" {
			return "never"
		}
		return text
	}
	return raw
}

// Invite is the verified invitation response.
type Invite struct {
	URL string `json:"url"`
}

// GrantRequest names one account and one tool or model.
type GrantRequest struct {
	Email string
	Kind  string
	Name  string
}

// MutationResult reports a configured grant or revoke response.
type MutationResult struct {
	StatusCode int
	Body       json.RawMessage
}

// ListUsers returns every company account.
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	var response struct {
		Users []User `json:"users"`
	}
	if err := c.do(ctx, http.MethodGet, c.usersPath, nil, &response); err != nil {
		return nil, err
	}
	return response.Users, nil
}

// Invite creates an invitation for a new account. The verified request
// field is "username", so the supplied value is sent unchanged under that
// key even when it looks like an email address.
func (c *Client) Invite(ctx context.Context, username string) (Invite, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return Invite{}, fmt.Errorf(
			"%w: invite needs an account name", ErrNotConfigured,
		)
	}
	body := struct {
		Username string `json:"username"`
	}{Username: username}
	var invite Invite
	if err := c.do(ctx, http.MethodPost, c.invitesPath, body, &invite); err != nil {
		return Invite{}, err
	}
	return invite, nil
}

// FindUser resolves an account name. It prefers an exact match and falls
// back to a case-insensitive match when that is unambiguous.
func (c *Client) FindUser(ctx context.Context, username string) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, fmt.Errorf(
			"%w: account name is empty", ErrNotConfigured,
		)
	}
	users, err := c.ListUsers(ctx)
	if err != nil {
		return User{}, err
	}
	for _, user := range users {
		if user.Username == username {
			return user, nil
		}
	}
	var matches []User
	for _, user := range users {
		if strings.EqualFold(user.Username, username) {
			matches = append(matches, user)
		}
	}
	switch len(matches) {
	case 0:
		return User{}, fmt.Errorf("%w: %q", ErrUserNotFound, username)
	case 1:
		return matches[0], nil
	default:
		return User{}, fmt.Errorf(
			"%w: %q matches %d accounts", ErrUserAmbiguous, username,
			len(matches),
		)
	}
}

// SetUserEnabled enables or disables one account by ID.
func (c *Client) SetUserEnabled(
	ctx context.Context, userID string, enabled bool,
) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("%w: account ID is empty", ErrNotConfigured)
	}
	action := "disable"
	if enabled {
		action = "enable"
	}
	path := strings.TrimRight(c.usersPath, "/") + "/" +
		url.PathEscape(userID) + "/" + action
	return c.do(ctx, http.MethodPost, path, struct{}{}, nil)
}

// DisableUser resolves an account name and disables it. An account that is
// already disabled is returned unchanged without a write.
func (c *Client) DisableUser(ctx context.Context, username string) (User, error) {
	user, err := c.FindUser(ctx, username)
	if err != nil {
		return User{}, err
	}
	if !user.Enabled {
		return user, nil
	}
	if err := c.SetUserEnabled(ctx, user.ID, false); err != nil {
		return User{}, err
	}
	user.Enabled = false
	return user, nil
}

// Grant grants a tool or model to one account. The live admin plane has no
// verified per-user grant route, so this returns ErrUnverifiedEndpoint
// unless Config.GrantPath was set by the operator.
func (c *Client) Grant(
	ctx context.Context, request GrantRequest,
) (MutationResult, error) {
	return c.mutateConfigured(
		ctx, c.grantPath, c.grantMethod, "MEGAFLOYD_ADMIN_GRANT_PATH",
		request,
	)
}

// RevokeGrant revokes a tool or model from one account. The live admin
// plane has no verified per-user revoke route, so this returns
// ErrUnverifiedEndpoint unless Config.RevokePath was set by the operator.
func (c *Client) RevokeGrant(
	ctx context.Context, request GrantRequest,
) (MutationResult, error) {
	return c.mutateConfigured(
		ctx, c.revokePath, c.revokeMethod, "MEGAFLOYD_ADMIN_REVOKE_PATH",
		request,
	)
}

func (c *Client) mutateConfigured(
	ctx context.Context, path, method, envName string, request GrantRequest,
) (MutationResult, error) {
	if err := request.validate(); err != nil {
		return MutationResult{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return MutationResult{}, fmt.Errorf(
			"%w: no per-user %s grant route was observed on the live admin "+
				"plane; set %s (and optionally %s_METHOD) to use a "+
				"deployment-specific route",
			ErrUnverifiedEndpoint, request.Kind, envName, envName,
		)
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return MutationResult{}, fmt.Errorf(
			"%w: unsupported mutation method %q", ErrNotConfigured, method,
		)
	}
	path = expandPath(path, request)
	body := struct {
		Email string `json:"email"`
		Kind  string `json:"kind"`
		Name  string `json:"name"`
	}{
		Email: request.Email,
		Kind:  request.Kind,
		Name:  request.Name,
	}
	status, raw, err := c.doRaw(ctx, method, path, body)
	if err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{StatusCode: status}
	if len(raw) > 0 {
		result.Body = json.RawMessage(raw)
	}
	return result, nil
}

func (r GrantRequest) validate() error {
	if strings.TrimSpace(r.Email) == "" {
		return fmt.Errorf("%w: grant needs an account", ErrNotConfigured)
	}
	if r.Kind != "tool" && r.Kind != "model" {
		return fmt.Errorf(
			"%w: kind must be tool or model, got %q",
			ErrNotConfigured, r.Kind,
		)
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf(
			"%w: grant needs a tool or model name", ErrNotConfigured,
		)
	}
	return nil
}

func (c *Client) do(
	ctx context.Context, method, path string, body, out any,
) error {
	_, raw, err := c.doRaw(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf(
				"%w: decode admin response: %v", ErrInvalidResponse, err,
			)
		}
	}
	return nil
}

func (c *Client) doRaw(
	ctx context.Context, method, path string, body any,
) (int, []byte, error) {
	endpoint, err := c.endpoint(path)
	if err != nil {
		return 0, nil, err
	}

	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf(
				"%w: encode request: %v", ErrInvalidRequest, err,
			)
		}
	}

	requestCtx := ctx
	if c.timeout > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf(
			"megafloydadmin: get access token: %w", err,
		)
	}
	if token == "" {
		return 0, nil, fmt.Errorf(
			"%w: access token is empty", ErrUnauthorized,
		)
	}

	request, err := http.NewRequestWithContext(
		requestCtx, method, endpoint, bytes.NewReader(payload),
	)
	if err != nil {
		return 0, nil, fmt.Errorf(
			"%w: build request: %v", ErrInvalidRequest, err,
		)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.http.Do(request)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return response.StatusCode, nil, fmt.Errorf(
			"%w: read response: %v", ErrConnection, err,
		)
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response.StatusCode, raw, nil
	}

	apiError := decodeAPIError(response.StatusCode, raw)
	apiError.Code = redactSecret(apiError.Code, token)
	apiError.Description = redactSecret(apiError.Description, token)
	apiError.Body = redactSecret(apiError.Body, token)
	if location := response.Header.Get("Location"); location != "" {
		apiError.Description = strings.TrimSpace(
			apiError.Description + " (redirect to " + location + ")",
		)
	}
	return response.StatusCode, raw, apiError
}

func (c *Client) endpoint(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%w: endpoint path is empty", ErrNotConfigured)
	}
	if strings.Contains(path, "://") {
		return "", fmt.Errorf(
			"%w: endpoint path must be relative to the admin base URL",
			ErrNotConfigured,
		)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL + path, nil
}

func decodeAPIError(status int, raw []byte) *APIError {
	apiError := &APIError{
		StatusCode: status,
		Body:       truncate(strings.TrimSpace(string(raw)), 512),
		sentinel:   statusSentinel(status),
	}
	var envelope struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil {
		apiError.Code = envelope.Error
		apiError.Description = envelope.ErrorDescription
	}
	return apiError
}

func normalizePath(value, fallback string) string {
	path := strings.TrimSpace(value)
	if path == "" {
		path = fallback
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func normalizeMethod(value string) string {
	method := strings.ToUpper(strings.TrimSpace(value))
	if method == "" {
		return http.MethodPost
	}
	return method
}

func expandPath(path string, request GrantRequest) string {
	replacer := strings.NewReplacer(
		"{email}", url.PathEscape(request.Email),
		"{kind}", url.PathEscape(request.Kind),
		"{name}", url.PathEscape(request.Name),
	)
	return replacer.Replace(path)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func redactSecret(value, secret string) string {
	if value == "" || secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[redacted]")
}
