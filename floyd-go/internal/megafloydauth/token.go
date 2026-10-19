package megafloydauth

import (
	"time"

	"github.com/legacy-ai/floyd/internal/oauth"
)

// Token is the f7 on-disk token bundle. Field names and the snake_case JSON
// wire format match packages/oauth/src/types.ts and the live file
// ~/.floyd7/credentials/floyd-code-env-e45bd67489796b47.json.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// Valid reports whether the bundle carries a usable access token. A bundle
// with an empty access token is the donor's "revoked" tombstone: the user
// was logged in before and must log in again.
func (t Token) Valid() bool {
	return t.AccessToken != ""
}

// RefreshThreshold returns how long before expiry the token should be
// refreshed: half the original lifetime, with a five-minute floor. This
// mirrors the donor's defaultRefreshThreshold.
func (t Token) RefreshThreshold() time.Duration {
	threshold := time.Duration(float64(t.ExpiresIn) * RefreshThresholdRatio * float64(time.Second))
	return max(threshold, MinRefreshThreshold)
}

// Expired reports whether the token is inside its refresh window. A zero
// ExpiresAt is treated as unknown and left to the caller.
func (t Token) Expired(now time.Time) bool {
	if t.ExpiresAt == 0 {
		return false
	}
	// Strict comparison, matching the donor's `remaining < threshold`:
	// a token expiring exactly at now+threshold is still usable.
	return time.Unix(t.ExpiresAt, 0).Before(now.Add(t.RefreshThreshold()))
}

// OAuthToken converts the bundle into the chassis token type used by
// config.ProviderConfig.OAuthToken.
func (t Token) OAuthToken() *oauth.Token {
	return &oauth.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		ExpiresIn:    int(t.ExpiresIn),
		ExpiresAt:    t.ExpiresAt,
	}
}

// TokenFromOAuth converts a chassis token into the f7 wire format.
func TokenFromOAuth(t *oauth.Token) Token {
	if t == nil {
		return Token{}
	}
	return Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		ExpiresAt:    t.ExpiresAt,
		ExpiresIn:    int64(t.ExpiresIn),
	}
}
