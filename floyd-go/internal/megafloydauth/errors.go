package megafloydauth

import (
	"errors"
	"fmt"
	"strings"
)

// Typed errors returned by the device-code flow. Callers branch with
// errors.Is; server detail stays in the error text.
var (
	// ErrNotConfigured means the flow has no OAuth host, no token store,
	// or no storage name to work with.
	ErrNotConfigured = errors.New("megafloydauth: not configured")
	// ErrConnection is a transport-level failure (dial, TLS, timeout, or
	// a truncated response body). Refresh retries these.
	ErrConnection = errors.New("megafloydauth: OAuth request failed")
	// ErrInvalidResponse means a 2xx response did not carry the fields
	// the donor client requires.
	ErrInvalidResponse = errors.New("megafloydauth: invalid OAuth response")
	// ErrUnauthorized is terminal: the refresh token was rejected with
	// 401, 403, or invalid_grant, and the user must log in again.
	ErrUnauthorized = errors.New("megafloydauth: token refresh unauthorized; log in again")
	// ErrAccessDenied means the user refused the device authorization.
	ErrAccessDenied = errors.New("megafloydauth: authorization denied")
	// ErrLoginTimeout means the local 15-minute login budget ran out.
	ErrLoginTimeout = errors.New("megafloydauth: device authorization timed out")
	// ErrNotLoggedIn means no stored token exists.
	ErrNotLoggedIn = errors.New("megafloydauth: not logged in")
	// ErrLoginRevoked means the stored record is a tombstone: the user was
	// logged in before and the server rejected the refresh token since.
	ErrLoginRevoked = errors.New("megafloydauth: previous login was revoked; log in again")
)

// HTTPError reports a non-success HTTP status from an OAuth endpoint.
type HTTPError struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Code is the server's "error" field, when present.
	Code string
	// Description is the server's error detail, when present.
	Description string
}

// Error implements error.
func (e *HTTPError) Error() string {
	detail := strings.TrimSpace(e.Description)
	if detail == "" {
		detail = strings.TrimSpace(e.Code)
	}
	if detail == "" {
		return fmt.Sprintf("megafloydauth: OAuth endpoint returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("megafloydauth: OAuth endpoint returned HTTP %d: %s", e.StatusCode, detail)
}

// Retryable reports the donor's retryable status set.
func (e *HTTPError) Retryable() bool {
	switch e.StatusCode {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// errorDetail extracts a human-readable message from an error response,
// mirroring the donor's extractApiErrorMessage precedence.
func errorDetail(data map[string]any) string {
	if data == nil {
		return ""
	}
	if detail := nonEmptyString(data["error_description"]); detail != "" {
		return detail
	}
	if detail := nonEmptyString(data["message"]); detail != "" {
		return detail
	}
	if detail := nonEmptyString(data["detail"]); detail != "" {
		return detail
	}
	if code := nonEmptyString(data["error"]); code != "" {
		return code
	}
	if nested, ok := data["error"].(map[string]any); ok {
		for _, key := range []string{"message", "error_description", "detail", "code", "type"} {
			if detail := nonEmptyString(nested[key]); detail != "" {
				return detail
			}
		}
	}
	if items, ok := data["errors"].([]any); ok {
		for _, item := range items {
			if record, ok := item.(map[string]any); ok {
				if detail := errorDetail(record); detail != "" {
					return detail
				}
			}
		}
	}
	return ""
}

func nonEmptyString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
