package megafloydadmin

import (
	"errors"
	"fmt"
	"strings"
)

// Typed errors returned by the admin client. Callers branch with
// errors.Is; server detail stays in the error text.
var (
	// ErrNotConfigured means the client has no usable base URL, path,
	// token source, or request value.
	ErrNotConfigured = errors.New("megafloydadmin: not configured")
	// ErrUnauthorized means the admin plane rejected the credential
	// (Bearer token or administrator key) with 401 or 403.
	ErrUnauthorized = errors.New("megafloydadmin: admin request unauthorized")
	// ErrEndpointNotFound means the admin plane returned 404 for a
	// configured route.
	ErrEndpointNotFound = errors.New("megafloydadmin: admin endpoint not found")
	// ErrUnverifiedEndpoint means the requested operation has no route
	// that was observed on the live server and no override was supplied.
	ErrUnverifiedEndpoint = errors.New("megafloydadmin: unverified endpoint")
	// ErrInvalidResponse means a 2xx response could not be decoded.
	ErrInvalidResponse = errors.New("megafloydadmin: invalid admin response")
	// ErrInvalidRequest means a request could not be encoded.
	ErrInvalidRequest = errors.New("megafloydadmin: invalid admin request")
	// ErrConnection is a transport-level failure (dial, TLS, timeout, or
	// a truncated response body).
	ErrConnection = errors.New("megafloydadmin: admin request failed")
	// ErrUserNotFound means no account matched the supplied name.
	ErrUserNotFound = errors.New("megafloydadmin: account not found")
	// ErrUserAmbiguous means a case-insensitive account lookup matched
	// more than one account.
	ErrUserAmbiguous = errors.New("megafloydadmin: account name is ambiguous")
)

// APIError reports a non-success HTTP status from an admin endpoint.
type APIError struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Code is the server's "error" field, when present.
	Code string
	// Description is the server's "error_description" field, when
	// present.
	Description string
	// Body is a short excerpt of the response body, when the server did
	// not return the standard JSON envelope.
	Body string

	sentinel error
}

// Error implements error.
func (e *APIError) Error() string {
	detail := strings.TrimSpace(e.Description)
	if detail == "" {
		detail = strings.TrimSpace(e.Code)
	}
	if detail == "" {
		detail = strings.TrimSpace(e.Body)
	}
	if detail == "" {
		return fmt.Sprintf("megafloydadmin: admin endpoint returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf(
		"megafloydadmin: admin endpoint returned HTTP %d: %s",
		e.StatusCode, detail,
	)
}

// Unwrap exposes the status sentinel so errors.Is works for 401, 403,
// and 404.
func (e *APIError) Unwrap() error {
	return e.sentinel
}

// statusSentinel maps an HTTP status to the package sentinel callers use.
func statusSentinel(status int) error {
	switch status {
	case 401, 403:
		return ErrUnauthorized
	case 404:
		return ErrEndpointNotFound
	default:
		return nil
	}
}
