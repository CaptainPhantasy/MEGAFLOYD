package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateChecksMegaFloydReleaseMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/CaptainPhantasy/MEGAFLOYD/releases/latest", r.URL.Path)
		fmt.Fprint(w, `{"tag_name":"v1.2.3","html_url":"https://github.com/CaptainPhantasy/MEGAFLOYD/releases/tag/v1.2.3"}`)
	}))
	defer server.Close()
	old := latestReleaseURL
	latestReleaseURL = server.URL + "/repos/" + releaseRepository + "/releases/latest"
	defer func() { latestReleaseURL = old }()
	c := newUpdateCommand()
	var output bytes.Buffer
	c.SetOut(&output)
	c.SetArgs([]string{"--check"})
	require.NoError(t, c.Execute())
	require.Contains(t, output.String(), `"latest":"v1.2.3"`)
	require.Contains(t, output.String(), releaseRepository)
	require.ErrorContains(t, fetchReleaseAsset(context.Background(), releaseAsset{Name: "unexpected", URL: "https://example.com/untrusted"}, "unused"), "unexpected release asset source")
}

func TestUpdateFindsFullPackageVerifier(t *testing.T) {
	path, err := releaseInstaller()
	require.NoError(t, err)
	require.Contains(t, path, "megafloyd-release.py")
}

type releaseRoundTrip func(*http.Request) (*http.Response, error)

func (f releaseRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestReleaseAuthenticationStaysOnGitHub(t *testing.T) {
	t.Setenv("GH_TOKEN", "fixture-token")
	oldClient, oldURL := updateHTTPClient, latestReleaseURL
	defer func() { updateHTTPClient, latestReleaseURL = oldClient, oldURL }()
	for _, target := range []string{"https://api.github.com/repos/CaptainPhantasy/MEGAFLOYD/releases/latest", "https://example.invalid/releases/latest"} {
		latestReleaseURL = target
		updateHTTPClient = &http.Client{Transport: releaseRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "api.github.com" {
				require.Equal(t, "Bearer fixture-token", r.Header.Get("Authorization"))
			} else {
				require.Empty(t, r.Header.Get("Authorization"))
			}
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
		})}
		release, err := latestCLIRelease(context.Background())
		require.NoError(t, err)
		require.Nil(t, release)
	}
}

func TestReleaseRateLimitReportsRecovery(t *testing.T) {
	old := updateHTTPClient
	defer func() { updateHTTPClient = old }()
	updateHTTPClient = &http.Client{Transport: releaseRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"X-Ratelimit-Remaining": []string{"0"}}}, nil
	})}
	_, err := latestCLIRelease(context.Background())
	require.ErrorContains(t, err, "GH_TOKEN")
}
