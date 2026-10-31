package hyper

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchCreditsCacheAndHTTPResults(t *testing.T) {
	var calls atomic.Int64
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/credits" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			http.Error(w, "bad request", 400)
			return
		}
		switch calls.Load() {
		case 1:
			_, _ = w.Write([]byte(`{"balance":42}`))
		case 2:
			_, _ = w.Write([]byte(`{"balance_usd":1.25}`))
		case 3:
			http.Error(w, "denied", http.StatusUnauthorized)
		default:
			_, _ = w.Write([]byte(`{invalid`))
		}
	}))
	defer fixture.Close()
	original := BaseURL
	BaseURL = func() string { return fixture.URL }
	hasBalance.Store(false)
	t.Cleanup(func() { BaseURL = original; hasBalance.Store(false) })
	SetBalance(17)
	value, err := FetchCredits(t.Context(), "fixture-token")
	require.NoError(t, err)
	require.NotNil(t, value)
	require.Equal(t, 17, *value)
	require.Zero(t, calls.Load())
	value, err = FetchCredits(t.Context(), "fixture-token")
	require.NoError(t, err)
	require.Equal(t, 42, *value)
	value, err = FetchCredits(t.Context(), "fixture-token")
	require.NoError(t, err)
	require.Nil(t, value)
	_, err = FetchCredits(t.Context(), "fixture-token")
	require.ErrorContains(t, err, "401")
	_, err = FetchCredits(t.Context(), "fixture-token")
	require.ErrorContains(t, err, "decode response")
}
