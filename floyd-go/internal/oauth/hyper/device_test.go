package hyper

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	hyperprovider "github.com/legacy-ai/floyd/internal/agent/hyper"
	"github.com/legacy-ai/floyd/internal/oauth"
	"github.com/stretchr/testify/require"
)

func TestDeviceFlowExchangeAndIntrospectionUseProviderContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing JSON content type")
		}
		if r.Header.Get("User-Agent") != "floyd" {
			t.Error("missing client user agent")
		}
		var input map[string]string
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
		}
		switch r.URL.Path {
		case "/device/auth":
			if r.Method != http.MethodPost || input["device_name"] == "" {
				t.Error("invalid device auth request")
			}
			_, _ = w.Write([]byte(`{"device_code":"fixture-code","user_code":"ABCD","verification_url":"https://example.test/activate","expires_in":60}`))
		case "/device/auth/fixture-code":
			if r.Method != http.MethodGet {
				t.Error("poll must use GET")
			}
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		case "/token/exchange":
			if input["refresh_token"] != "fixture-refresh" {
				t.Error("wrong refresh token")
			}
			_, _ = w.Write([]byte(`{"access_token":"fixture-access","refresh_token":"rotated-fixture","expires_in":3600}`))
		case "/token/introspect":
			if input["token"] != "fixture-access" {
				t.Error("wrong access token")
			}
			_, _ = w.Write([]byte(`{"active":true,"sub":"fixture-user","org_id":"fixture-org"}`))
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	original := hyperprovider.BaseURL
	hyperprovider.BaseURL = func() string { return server.URL }
	t.Cleanup(func() { hyperprovider.BaseURL = original })
	auth, err := InitiateDeviceAuth(t.Context())
	require.NoError(t, err)
	require.Equal(t, "ABCD", auth.UserCode)
	pending, err := pollOnce(t.Context(), auth.DeviceCode)
	require.NoError(t, err)
	require.Equal(t, "authorization_pending", pending.Error)
	token, err := ExchangeToken(t.Context(), "fixture-refresh")
	require.NoError(t, err)
	require.Equal(t, "fixture-access", token.AccessToken)
	require.Equal(t, "rotated-fixture", token.RefreshToken)
	require.False(t, token.IsExpired())
	active, err := IntrospectToken(t.Context(), token.AccessToken)
	require.NoError(t, err)
	require.True(t, active.Active)
	require.Equal(t, "fixture-user", active.Sub)
}

func TestDeviceProviderErrorsAndCancellationAreNotSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()
	original := hyperprovider.BaseURL
	hyperprovider.BaseURL = func() string { return server.URL }
	t.Cleanup(func() { hyperprovider.BaseURL = original })
	_, err := InitiateDeviceAuth(t.Context())
	require.ErrorContains(t, err, "status 400")
	_, err = pollOnce(t.Context(), "fixture")
	require.ErrorContains(t, err, "status 400")
	_, err = IntrospectToken(t.Context(), "fixture")
	require.ErrorContains(t, err, "status 400")
	_, err = ExchangeToken(t.Context(), "fixture")
	var exchange *oauth.TokenExchangeError
	require.ErrorAs(t, err, &exchange)
	require.True(t, exchange.IsRefreshTokenRevoked())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Now()
	_, err = PollForToken(ctx, "fixture", 60)
	require.True(t, errors.Is(err, context.Canceled))
	require.Less(t, time.Since(start), time.Second)
	_, err = PollForToken(t.Context(), "fixture", 0)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
