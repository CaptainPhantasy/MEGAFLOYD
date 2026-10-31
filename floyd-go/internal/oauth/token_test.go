package oauth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTokenExpiryRefreshWindowAndMissingExpiry(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	for _, tc := range []struct {
		name    string
		token   Token
		expired bool
	}{
		{"missing", Token{}, true},
		{"expired", Token{ExpiresAt: now - 1}, true},
		{"minimum buffer", Token{ExpiresIn: 60, ExpiresAt: now + 20}, true},
		{"percentage buffer", Token{ExpiresIn: 3600, ExpiresAt: now + 300}, true},
		{"usable", Token{ExpiresIn: 3600, ExpiresAt: now + 1800}, false},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.expired, tc.token.IsExpired()) })
	}
	token := Token{ExpiresIn: 3600}
	token.SetExpiresAt()
	require.InDelta(t, now+3600, token.ExpiresAt, 2)
	absolute := Token{ExpiresIn: -1, ExpiresAt: now + 3600}
	absolute.SetExpiresAt()
	require.Equal(t, now+3600, absolute.ExpiresAt)
	absolute.SetExpiresIn()
	require.InDelta(t, 3600, absolute.ExpiresIn, 2)
	missing := Token{ExpiresIn: -1}
	missing.SetExpiresAt()
	require.True(t, missing.IsExpired())
}

func TestOAuthClientSurvivesTokenPersistenceAndExchangeFailuresRemainTyped(t *testing.T) {
	t.Parallel()
	token := Token{AccessToken: "fixture-access", RefreshToken: "fixture-refresh", Client: &OAuthClient{ClientID: "client", AuthURL: "https://example.test/auth", TokenURL: "https://example.test/token", AuthStyle: 2}}
	raw, err := json.Marshal(token)
	require.NoError(t, err)
	var restored Token
	require.NoError(t, json.Unmarshal(raw, &restored))
	require.Equal(t, token, restored)
	revoked := &TokenExchangeError{StatusCode: 400, Body: `{"error":"invalid_grant"}`}
	require.True(t, revoked.IsRefreshTokenRevoked())
	require.Contains(t, revoked.Error(), "status 400")
	require.False(t, (&TokenExchangeError{StatusCode: 503, Body: "temporarily unavailable"}).IsRefreshTokenRevoked())
}
