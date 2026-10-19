package megafloydauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HTTPDoer is the subset of http.Client used by the flow.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// DeviceAuthorization is an RFC 8628 §3.2 device authorization response.
type DeviceAuthorization struct {
	UserCode                string
	DeviceCode              string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               time.Duration
	Interval                time.Duration
}

// PollStatus classifies one token poll.
type PollStatus int

const (
	// PollPending means the user has not approved yet (RFC 8628
	// authorization_pending or slow_down).
	PollPending PollStatus = iota
	// PollSuccess means a token was issued.
	PollSuccess
	// PollExpired means the device code expired; start over.
	PollExpired
	// PollDenied means the user or an administrator refused the device.
	PollDenied
)

// PollResult is the outcome of one token poll. Token is set only for
// PollSuccess; ErrorCode and Description carry server detail otherwise.
type PollResult struct {
	Status      PollStatus
	Token       Token
	ErrorCode   string
	Description string
}

// DeviceFlow is the device-code OAuth client.
//
// Endpoint shape, taken from the donor client:
//   - POST {OAuthHost}/api/oauth/device_authorization
//   - POST {OAuthHost}/api/oauth/token (device_code grant)
//   - POST {OAuthHost}/api/oauth/token (refresh_token grant)
//
// All three are form-encoded and carry the client_id. The live routes have
// not been exercised from Go; see docs/auth-integration.md unknowns 1-2.
type DeviceFlow interface {
	RequestDeviceAuthorization(ctx context.Context) (DeviceAuthorization, error)
	PollDeviceToken(ctx context.Context, deviceCode string) (PollResult, error)
	RefreshAccessToken(ctx context.Context, refreshToken string) (Token, error)
}

// TokenSource supplies a fresh access token to a provider. Manager
// implements it; nothing in the current chassis consumes it yet.
type TokenSource interface {
	AccessToken(ctx context.Context) (string, error)
}

// Client implements DeviceFlow over HTTP. Config supplies the host,
// endpoints, identity headers, clock, and sleeper.
type Client struct {
	// Config is the resolved flow configuration.
	Config Config
	// HTTPClient overrides Config.HTTPClient when set.
	HTTPClient HTTPDoer
}

// NewClient returns a Client for cfg.
func NewClient(cfg Config) *Client {
	return &Client{Config: cfg}
}

type deviceAuthorizationResponse struct {
	UserCode                string    `json:"user_code"`
	DeviceCode              string    `json:"device_code"`
	VerificationURI         string    `json:"verification_uri"`
	VerificationURIComplete string    `json:"verification_uri_complete"`
	ExpiresIn               flexFloat `json:"expires_in"`
	Interval                flexFloat `json:"interval"`
}

// RequestDeviceAuthorization starts a device authorization request.
func (c *Client) RequestDeviceAuthorization(ctx context.Context) (DeviceAuthorization, error) {
	endpoint, err := c.Config.endpointURL(
		c.Config.DeviceAuthorizationPath, DefaultDeviceAuthorizationPath,
	)
	if err != nil {
		return DeviceAuthorization{}, err
	}
	status, data, err := c.postForm(ctx, endpoint, url.Values{
		"client_id": {c.Config.clientID()},
	})
	if err != nil {
		return DeviceAuthorization{}, err
	}
	if status != 200 {
		return DeviceAuthorization{}, &HTTPError{
			StatusCode:  status,
			Code:        nonEmptyString(data["error"]),
			Description: errorDetail(data),
		}
	}

	var payload deviceAuthorizationResponse
	if err := decodePayload(data, &payload); err != nil {
		return DeviceAuthorization{}, err
	}
	if payload.UserCode == "" {
		return DeviceAuthorization{}, fmt.Errorf(
			"%w: device authorization response missing user_code", ErrInvalidResponse,
		)
	}
	if payload.DeviceCode == "" {
		return DeviceAuthorization{}, fmt.Errorf(
			"%w: device authorization response missing device_code", ErrInvalidResponse,
		)
	}
	if payload.VerificationURIComplete == "" {
		return DeviceAuthorization{}, fmt.Errorf(
			"%w: device authorization response missing verification_uri_complete", ErrInvalidResponse,
		)
	}

	auth := DeviceAuthorization{
		UserCode:                payload.UserCode,
		DeviceCode:              payload.DeviceCode,
		VerificationURI:         payload.VerificationURI,
		VerificationURIComplete: payload.VerificationURIComplete,
	}
	if payload.ExpiresIn.set && payload.ExpiresIn.value > 0 {
		auth.ExpiresIn = time.Duration(payload.ExpiresIn.value * float64(time.Second))
	}
	auth.Interval = DefaultPollInterval
	if payload.Interval.set && payload.Interval.value > 0 {
		auth.Interval = time.Duration(payload.Interval.value * float64(time.Second))
	}
	return auth, nil
}

// PollDeviceToken performs one device-code token poll.
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string) (PollResult, error) {
	endpoint, err := c.Config.endpointURL(c.Config.TokenPath, DefaultTokenPath)
	if err != nil {
		return PollResult{}, err
	}
	status, data, err := c.postForm(ctx, endpoint, url.Values{
		"client_id":   {c.Config.clientID()},
		"device_code": {deviceCode},
		"grant_type":  {DeviceCodeGrantType},
	})
	if err != nil {
		return PollResult{}, err
	}

	if status == 200 {
		if _, present := data["access_token"]; present {
			token, err := tokenFromResponse(data, c.Config.NowFunc()())
			if err != nil {
				return PollResult{}, err
			}
			return PollResult{Status: PollSuccess, Token: token}, nil
		}
	}
	if status >= 500 {
		return PollResult{}, &HTTPError{
			StatusCode:  status,
			Code:        nonEmptyString(data["error"]),
			Description: errorDetail(data),
		}
	}

	code := nonEmptyString(data["error"])
	if code == "" {
		code = "unknown_error"
	}
	description := nonEmptyString(data["error_description"])
	if description == "" {
		description = errorDetail(data)
	}
	switch code {
	case "authorization_pending", "slow_down":
		return PollResult{Status: PollPending, ErrorCode: code, Description: description}, nil
	case "expired_token":
		return PollResult{Status: PollExpired, ErrorCode: code, Description: description}, nil
	case "access_denied":
		return PollResult{Status: PollDenied, ErrorCode: code, Description: description}, nil
	default:
		return PollResult{}, &HTTPError{
			StatusCode:  status,
			Code:        code,
			Description: description,
		}
	}
}

// RefreshAccessToken exchanges a refresh token for a new bundle.
//
// Transport failures and 429/500/502/503/504 are retried up to three
// attempts with 1s then 2s backoff, matching the donor. 401, 403, and
// invalid_grant are terminal and wrap ErrUnauthorized.
func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (Token, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return Token{}, fmt.Errorf("%w: no refresh token", ErrUnauthorized)
	}
	endpoint, err := c.Config.endpointURL(c.Config.TokenPath, DefaultTokenPath)
	if err != nil {
		return Token{}, err
	}
	params := url.Values{
		"client_id":     {c.Config.clientID()},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	const attempts = 3
	var lastErr error
	for attempt := range attempts {
		token, retryable, err := c.refreshOnce(ctx, endpoint, params)
		if err == nil {
			return token, nil
		}
		if !retryable {
			return Token{}, err
		}
		lastErr = err
		if attempt == attempts-1 {
			break
		}
		backoff := time.Duration(1<<attempt) * time.Second
		if err := c.Config.SleepFunc()(ctx, backoff); err != nil {
			return Token{}, err
		}
	}
	return Token{}, lastErr
}

// refreshOnce performs one refresh request and classifies the failure.
func (c *Client) refreshOnce(
	ctx context.Context, endpoint string, params url.Values,
) (Token, bool, error) {
	status, data, err := c.postForm(ctx, endpoint, params)
	if err != nil {
		// Transport failures are retryable.
		return Token{}, errors.Is(err, ErrConnection), err
	}
	if status == 200 {
		if _, present := data["access_token"]; present {
			token, err := tokenFromResponse(data, c.Config.NowFunc()())
			return token, false, err
		}
	}

	code := nonEmptyString(data["error"])
	if status == 401 || status == 403 || code == "invalid_grant" {
		detail := errorDetail(data)
		if detail == "" {
			detail = "token refresh unauthorized"
		}
		return Token{}, false, fmt.Errorf("%w: %s", ErrUnauthorized, detail)
	}
	httpErr := &HTTPError{
		StatusCode:  status,
		Code:        code,
		Description: errorDetail(data),
	}
	return Token{}, httpErr.Retryable(), httpErr
}

// tokenFromResponse validates and converts a token response. The three
// load-bearing fields are required, matching the donor.
func tokenFromResponse(data map[string]any, now time.Time) (Token, error) {
	accessToken := nonEmptyString(data["access_token"])
	if accessToken == "" {
		return Token{}, fmt.Errorf("%w: missing access_token", ErrInvalidResponse)
	}
	refreshToken := nonEmptyString(data["refresh_token"])
	if refreshToken == "" {
		return Token{}, fmt.Errorf("%w: missing refresh_token", ErrInvalidResponse)
	}
	expiresIn, ok := numberValue(data["expires_in"])
	if !ok || math.IsNaN(expiresIn) || math.IsInf(expiresIn, 0) || expiresIn <= 0 {
		return Token{}, fmt.Errorf("%w: missing or invalid expires_in", ErrInvalidResponse)
	}
	scope, _ := data["scope"].(string)
	tokenType, _ := data["token_type"].(string)
	if tokenType == "" {
		tokenType = "Bearer"
	}
	seconds := int64(math.Floor(expiresIn))
	return Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    now.Unix() + seconds,
		Scope:        scope,
		TokenType:    tokenType,
		ExpiresIn:    seconds,
	}, nil
}

// decodePayload re-encodes the decoded JSON object into a typed struct.
// json.Unmarshal already validated the object; re-encoding only fails on
// values the response cannot contain.
func decodePayload(data map[string]any, payload any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidResponse, err)
	}
	if err := json.Unmarshal(encoded, payload); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func numberValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(number), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
