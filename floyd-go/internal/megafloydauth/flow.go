package megafloydauth

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// ErrNotImplemented is returned by the skeleton flow until the endpoints are
// exercised from Go and promoted to verified.
var ErrNotImplemented = errors.New("megafloydauth: device-code flow is not implemented")

// HTTPDoer is the subset of http.Client used by a future flow
// implementation.
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
// Endpoint shape, taken from the donor client and not yet exercised from Go:
//   - POST {OAuthHost}/api/oauth/device_authorization
//   - POST {OAuthHost}/api/oauth/token (device_code grant)
//   - POST {OAuthHost}/api/oauth/token (refresh_token grant)
//
// All three are form-encoded and carry the client_id. Do not implement this
// interface until docs/auth-integration.md upgrades the endpoints from
// EVIDENCED to VERIFIED.
type DeviceFlow interface {
	RequestDeviceAuthorization(ctx context.Context) (DeviceAuthorization, error)
	PollDeviceToken(ctx context.Context, deviceCode string) (PollResult, error)
	RefreshAccessToken(ctx context.Context, refreshToken string) (Token, error)
}

// TokenSource supplies a fresh access token to a provider.
type TokenSource interface {
	AccessToken(ctx context.Context) (string, error)
}

// UnimplementedFlow satisfies DeviceFlow and fails closed.
type UnimplementedFlow struct{}

// RequestDeviceAuthorization always returns ErrNotImplemented.
func (UnimplementedFlow) RequestDeviceAuthorization(context.Context) (DeviceAuthorization, error) {
	return DeviceAuthorization{}, ErrNotImplemented
}

// PollDeviceToken always returns ErrNotImplemented.
func (UnimplementedFlow) PollDeviceToken(context.Context, string) (PollResult, error) {
	return PollResult{}, ErrNotImplemented
}

// RefreshAccessToken always returns ErrNotImplemented.
func (UnimplementedFlow) RefreshAccessToken(context.Context, string) (Token, error) {
	return Token{}, ErrNotImplemented
}
