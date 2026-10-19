package megafloydauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testNowUnix = int64(1_700_000_000)

// newTestClient returns a Client pointed at an httptest server. Every
// test in this package uses it; no test touches a live host.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewClient(Config{
		OAuthHost: server.URL,
		ClientID:  "test-client",
		Platform:  DefaultPlatform,
		Version:   "1.2.3",
		HomeDir:   t.TempDir(),
		Now:       func() time.Time { return time.Unix(testNowUnix, 0) },
	})
}

type sleepRecorder struct {
	mu        sync.Mutex
	durations []time.Duration
}

func (r *sleepRecorder) sleep(context.Context, time.Duration) error {
	return nil
}

func (r *sleepRecorder) record(_ context.Context, d time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.durations = append(r.durations, d)
	return nil
}

func (r *sleepRecorder) recorded() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Duration(nil), r.durations...)
}

func TestRequestDeviceAuthorization(t *testing.T) {
	t.Parallel()

	var (
		gotMethod      string
		gotPath        string
		gotContentType string
		gotAccept      string
		gotUserAgent   string
		gotHeaders     http.Header
		gotForm        url.Values
	)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		gotUserAgent = r.Header.Get("User-Agent")
		gotHeaders = r.Header.Clone()
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, _ = io.WriteString(w, `{
			"user_code": "ABCD-EFGH",
			"device_code": "device-123",
			"verification_uri": "https://example.test/verify",
			"verification_uri_complete": "https://example.test/verify?user_code=ABCD-EFGH",
			"expires_in": 900,
			"interval": 5
		}`)
	})

	auth, err := client.RequestDeviceAuthorization(t.Context())
	require.NoError(t, err)

	require.Equal(t, http.MethodPost, gotMethod)
	require.Equal(t, DefaultDeviceAuthorizationPath, gotPath)
	require.Equal(t, "application/x-www-form-urlencoded", gotContentType)
	require.Equal(t, "application/json", gotAccept)
	require.Equal(t, "megafloyd/1.2.3", gotUserAgent)
	require.Equal(t, "test-client", gotForm.Get("client_id"))

	require.Equal(t, DefaultPlatform, gotHeaders.Get("X-Msh-Platform"))
	require.Equal(t, "1.2.3", gotHeaders.Get("X-Msh-Version"))
	require.NotEmpty(t, gotHeaders.Get("X-Msh-Device-Id"))
	require.NotEmpty(t, gotHeaders.Get("X-Msh-Device-Name"))
	require.NotEmpty(t, gotHeaders.Get("X-Msh-Device-Model"))
	require.NotEmpty(t, gotHeaders.Get("X-Msh-Os-Version"))

	require.Equal(t, "ABCD-EFGH", auth.UserCode)
	require.Equal(t, "device-123", auth.DeviceCode)
	require.Equal(t, "https://example.test/verify", auth.VerificationURI)
	require.Equal(t, "https://example.test/verify?user_code=ABCD-EFGH", auth.VerificationURIComplete)
	require.Equal(t, 900*time.Second, auth.ExpiresIn)
	require.Equal(t, 5*time.Second, auth.Interval)
}

func TestRequestDeviceAuthorizationDefaults(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
			"user_code": "CODE",
			"device_code": "dev",
			"verification_uri_complete": "https://example.test/verify",
			"expires_in": null
		}`)
	})
	auth, err := client.RequestDeviceAuthorization(t.Context())
	require.NoError(t, err)
	require.Zero(t, auth.ExpiresIn)
	require.Equal(t, DefaultPollInterval, auth.Interval)
}

func TestRequestDeviceAuthorizationAcceptsNumericStrings(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
			"user_code": "CODE",
			"device_code": "dev",
			"verification_uri_complete": "https://example.test/verify",
			"expires_in": "900",
			"interval": "7"
		}`)
	})
	auth, err := client.RequestDeviceAuthorization(t.Context())
	require.NoError(t, err)
	require.Equal(t, 900*time.Second, auth.ExpiresIn)
	require.Equal(t, 7*time.Second, auth.Interval)
}

func TestRequestDeviceAuthorizationUsesConfiguredPath(t *testing.T) {
	t.Parallel()

	var gotPath string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{
			"user_code": "CODE",
			"device_code": "dev",
			"verification_uri_complete": "https://example.test/verify"
		}`)
	})
	client.Config.DeviceAuthorizationPath = "/custom/device_authorization"
	_, err := client.RequestDeviceAuthorization(t.Context())
	require.NoError(t, err)
	require.Equal(t, "/custom/device_authorization", gotPath)
}

func TestRequestDeviceAuthorizationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "server error",
			status:     http.StatusBadGateway,
			body:       `{"error":"boom","error_description":"upstream failed"}`,
			wantStatus: http.StatusBadGateway,
			wantCode:   "boom",
		},
		{
			name:       "bad client",
			status:     http.StatusBadRequest,
			body:       `{"error":"invalid_client"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_client",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			_, err := client.RequestDeviceAuthorization(t.Context())
			require.Error(t, err)
			var httpErr *HTTPError
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, tt.wantStatus, httpErr.StatusCode)
			require.Equal(t, tt.wantCode, httpErr.Code)
		})
	}
}

func TestRequestDeviceAuthorizationMissingFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing user_code",
			body: `{"device_code":"dev","verification_uri_complete":"https://example.test"}`,
		},
		{
			name: "missing device_code",
			body: `{"user_code":"CODE","verification_uri_complete":"https://example.test"}`,
		},
		{
			name: "missing verification_uri_complete",
			body: `{"user_code":"CODE","device_code":"dev"}`,
		},
		{
			name: "non-json",
			body: `<html>not json</html>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tt.body)
			})
			_, err := client.RequestDeviceAuthorization(t.Context())
			require.ErrorIs(t, err, ErrInvalidResponse)
		})
	}
}

func TestRequestDeviceAuthorizationRequiresHost(t *testing.T) {
	t.Parallel()

	client := NewClient(Config{ClientID: "test-client"})
	_, err := client.RequestDeviceAuthorization(t.Context())
	require.ErrorIs(t, err, ErrNotConfigured)
}

func TestPollDeviceTokenSuccess(t *testing.T) {
	t.Parallel()

	var gotForm url.Values
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, _ = io.WriteString(w, `{
			"access_token": "access-1",
			"refresh_token": "refresh-1",
			"expires_in": 3600,
			"scope": "floyd-code",
			"token_type": "Bearer"
		}`)
	})

	result, err := client.PollDeviceToken(t.Context(), "device-123")
	require.NoError(t, err)
	require.Equal(t, PollSuccess, result.Status)
	require.Equal(t, "test-client", gotForm.Get("client_id"))
	require.Equal(t, "device-123", gotForm.Get("device_code"))
	require.Equal(t, DeviceCodeGrantType, gotForm.Get("grant_type"))

	require.Equal(t, "access-1", result.Token.AccessToken)
	require.Equal(t, "refresh-1", result.Token.RefreshToken)
	require.Equal(t, testNowUnix+3600, result.Token.ExpiresAt)
	require.Equal(t, int64(3600), result.Token.ExpiresIn)
	require.Equal(t, "floyd-code", result.Token.Scope)
	require.Equal(t, "Bearer", result.Token.TokenType)
}

func TestPollDeviceTokenStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   int
		body     string
		want     PollStatus
		wantCode string
	}{
		{
			name:     "authorization pending",
			status:   http.StatusBadRequest,
			body:     `{"error":"authorization_pending"}`,
			want:     PollPending,
			wantCode: "authorization_pending",
		},
		{
			name:     "slow down",
			status:   http.StatusBadRequest,
			body:     `{"error":"slow_down","error_description":"take it easy"}`,
			want:     PollPending,
			wantCode: "slow_down",
		},
		{
			name:     "expired token",
			status:   http.StatusBadRequest,
			body:     `{"error":"expired_token"}`,
			want:     PollExpired,
			wantCode: "expired_token",
		},
		{
			name:     "access denied",
			status:   http.StatusBadRequest,
			body:     `{"error":"access_denied","error_description":"user refused"}`,
			want:     PollDenied,
			wantCode: "access_denied",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			result, err := client.PollDeviceToken(t.Context(), "device")
			require.NoError(t, err)
			require.Equal(t, tt.want, result.Status)
			require.Equal(t, tt.wantCode, result.ErrorCode)
		})
	}
}

func TestPollDeviceTokenErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		check      func(*testing.T, error)
	}{
		{
			name:   "unknown error code",
			status: http.StatusBadRequest,
			body:   `{"error":"weird_error","error_description":"nope"}`,
			check: func(t *testing.T, err error) {
				t.Helper()
				var httpErr *HTTPError
				require.ErrorAs(t, err, &httpErr)
				require.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
				require.Equal(t, "weird_error", httpErr.Code)
				require.Contains(t, httpErr.Error(), "nope")
			},
		},
		{
			name:   "server error",
			status: http.StatusServiceUnavailable,
			body:   `{"error":"unavailable"}`,
			check: func(t *testing.T, err error) {
				t.Helper()
				var httpErr *HTTPError
				require.ErrorAs(t, err, &httpErr)
				require.True(t, httpErr.Retryable())
			},
		},
		{
			name:   "missing refresh token",
			status: http.StatusOK,
			body:   `{"access_token":"a","expires_in":3600}`,
			check: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, ErrInvalidResponse)
			},
		},
		{
			name:   "empty access token",
			status: http.StatusOK,
			body:   `{"access_token":"","refresh_token":"r","expires_in":3600}`,
			check: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, ErrInvalidResponse)
			},
		},
		{
			name:   "invalid expires_in",
			status: http.StatusOK,
			body:   `{"access_token":"a","refresh_token":"r","expires_in":0}`,
			check: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, ErrInvalidResponse)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			_, err := client.PollDeviceToken(t.Context(), "device")
			require.Error(t, err)
			tt.check(t, err)
		})
	}
}

func TestRefreshAccessTokenSuccess(t *testing.T) {
	t.Parallel()

	var gotForm url.Values
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, _ = io.WriteString(w, `{
			"access_token": "access-2",
			"refresh_token": "refresh-2",
			"expires_in": 7200
		}`)
	})

	token, err := client.RefreshAccessToken(t.Context(), "refresh-1")
	require.NoError(t, err)
	require.Equal(t, "test-client", gotForm.Get("client_id"))
	require.Equal(t, "refresh_token", gotForm.Get("grant_type"))
	require.Equal(t, "refresh-1", gotForm.Get("refresh_token"))
	require.Equal(t, "access-2", token.AccessToken)
	require.Equal(t, "refresh-2", token.RefreshToken)
	require.Equal(t, testNowUnix+7200, token.ExpiresAt)
	require.Equal(t, "Bearer", token.TokenType)
}

func TestRefreshAccessTokenTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":"invalid_token"}`},
		{name: "forbidden", status: http.StatusForbidden, body: `{"error":"forbidden"}`},
		{name: "invalid grant", status: http.StatusBadRequest, body: `{"error":"invalid_grant"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			recorder := &sleepRecorder{}
			client.Config.Sleep = recorder.record
			_, err := client.RefreshAccessToken(t.Context(), "refresh-1")
			require.ErrorIs(t, err, ErrUnauthorized)
			require.Empty(t, recorder.recorded())
		})
	}
}

func TestRefreshAccessTokenRetriesRetryableStatus(t *testing.T) {
	t.Parallel()

	var calls int
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":"rate_limited"}`)
			return
		}
		_, _ = io.WriteString(w, `{
			"access_token": "access-2",
			"refresh_token": "refresh-2",
			"expires_in": 3600
		}`)
	})
	recorder := &sleepRecorder{}
	client.Config.Sleep = recorder.record

	token, err := client.RefreshAccessToken(t.Context(), "refresh-1")
	require.NoError(t, err)
	require.Equal(t, "access-2", token.AccessToken)
	require.Equal(t, 2, calls)
	require.Equal(t, []time.Duration{time.Second}, recorder.recorded())
}

func TestRefreshAccessTokenRetryExhaustion(t *testing.T) {
	t.Parallel()

	var calls int
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"boom"}`)
	})
	recorder := &sleepRecorder{}
	client.Config.Sleep = recorder.record

	_, err := client.RefreshAccessToken(t.Context(), "refresh-1")
	require.Error(t, err)
	var httpErr *HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
	require.Equal(t, 3, calls)
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, recorder.recorded())
}

func TestRefreshAccessTokenRetriesTransportErrors(t *testing.T) {
	t.Parallel()

	doer := &flakyDoer{
		failures: 2,
		response: `{"access_token":"access-2","refresh_token":"refresh-2","expires_in":3600}`,
	}
	client := NewClient(Config{
		OAuthHost:  "https://auth.example.test",
		ClientID:   "test-client",
		Platform:   DefaultPlatform,
		Version:    "1.2.3",
		HomeDir:    t.TempDir(),
		Now:        func() time.Time { return time.Unix(testNowUnix, 0) },
		HTTPClient: doer,
	})
	recorder := &sleepRecorder{}
	client.Config.Sleep = recorder.record

	token, err := client.RefreshAccessToken(t.Context(), "refresh-1")
	require.NoError(t, err)
	require.Equal(t, "access-2", token.AccessToken)
	require.Equal(t, 3, doer.calls)
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, recorder.recorded())
}

func TestRefreshAccessTokenRequiresRefreshToken(t *testing.T) {
	t.Parallel()

	client := NewClient(Config{})
	_, err := client.RefreshAccessToken(t.Context(), "  ")
	require.ErrorIs(t, err, ErrUnauthorized)
}

// flakyDoer fails a fixed number of requests before returning a JSON token.
type flakyDoer struct {
	mu       sync.Mutex
	calls    int
	failures int
	response string
}

func (d *flakyDoer) Do(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if d.calls <= d.failures {
		return nil, errors.New("connection reset")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(d.response)),
		Request:    req,
	}, nil
}
