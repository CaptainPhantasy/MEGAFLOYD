package megafloydauth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// DefaultCredentialKey is the credential slot shared by the default
	// deployment (empty OAuth host and base URL).
	DefaultCredentialKey = "oauth/floyd-code"
	// ScopedCredentialKeyPrefix names slots for custom hosts.
	ScopedCredentialKeyPrefix = "oauth/floyd-code-env-"
	// DefaultStorageName is the token file stem for DefaultCredentialKey.
	DefaultStorageName = "floyd-code"
	// scopedStorageNamePrefix mirrors ScopedCredentialKeyPrefix minus the
	// "oauth/" segment.
	scopedStorageNamePrefix = "floyd-code-env-"
)

// CredentialKey returns the config-level credential slot for an
// (OAuth host, base URL) pair. The algorithm is verified against the live f7
// config: host and base both "https://inference.tail58d565.ts.net:8446"
// produce oauth/floyd-code-env-e45bd67489796b47.
//
// The slot is scoped whenever the environment is not the built-in default,
// so two deployments never share a token.
func CredentialKey(oauthHost, baseURL string) string {
	host := normalizeEndpoint(oauthHost)
	base := normalizeEndpoint(baseURL)
	if host == "" && base == "" {
		return DefaultCredentialKey
	}
	payload, err := json.Marshal(struct {
		OAuthHost string `json:"oauthHost"`
		BaseURL   string `json:"baseUrl"`
	}{OAuthHost: host, BaseURL: base})
	if err != nil {
		// Marshal of two strings cannot fail; keep the signature simple.
		panic(err)
	}
	sum := sha256.Sum256(payload)
	return ScopedCredentialKeyPrefix + hex.EncodeToString(sum[:])[:16]
}

// StorageName maps a credential key to the token file stem. The donor drops
// the "oauth/" segment: oauth/floyd-code -> floyd-code, and
// oauth/floyd-code-env-<hash> -> floyd-code-env-<hash>.
func StorageName(key string) (string, error) {
	switch key {
	case "", "floyd-code", DefaultCredentialKey:
		return DefaultStorageName, nil
	}
	if name, ok := strings.CutPrefix(key, "oauth/"); ok && name != "" {
		if strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, ".") {
			return "", fmt.Errorf("invalid MegaFloyd OAuth token key: %q", key)
		}
		return name, nil
	}
	if !strings.ContainsAny(key, "/\\") && !strings.HasPrefix(key, ".") {
		return key, nil
	}
	return "", fmt.Errorf("invalid MegaFloyd OAuth token key: %q", key)
}

func normalizeEndpoint(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}
