package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestPrivateReleaseAssetUsesAuthenticatedAPI(t *testing.T) {
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("GH_TOKEN", "")
			t.Setenv("GITHUB_TOKEN", "")
			t.Setenv(key, "fixture-private-token")
			oldClient, oldURL := updateHTTPClient, latestReleaseURL
			t.Cleanup(func() { updateHTTPClient, latestReleaseURL = oldClient, oldURL })
			latestReleaseURL = "https://api.github.com/repos/" + releaseRepository + "/releases/latest"
			var requests []string
			updateHTTPClient = &http.Client{Transport: releaseRoundTrip(func(r *http.Request) (*http.Response, error) {
				requests = append(requests, r.URL.String())
				if r.Header.Get("Authorization") != "Bearer fixture-private-token" {
					return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("private asset requires authentication")), Header: http.Header{}}, nil
				}
				body := "verified-fixture-archive"
				if r.URL.String() == latestReleaseURL {
					body = `{"tag_name":"v1.2.3","assets":[{"id":123,"name":"fixture.tar.gz","url":"https://api.github.com/repos/CaptainPhantasy/MEGAFLOYD/releases/assets/123","browser_download_url":"https://github.com/CaptainPhantasy/MEGAFLOYD/releases/download/v1.2.3/fixture.tar.gz"}]}`
				} else {
					require.Equal(t, "https://api.github.com/repos/"+releaseRepository+"/releases/assets/123", r.URL.String())
					require.Equal(t, "application/octet-stream", r.Header.Get("Accept"))
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
			})}
			release, err := latestCLIRelease(context.Background())
			require.NoError(t, err)
			require.Len(t, release.Assets, 1)
			target := filepath.Join(t.TempDir(), "fixture.tar.gz")
			require.NoError(t, fetchReleaseAsset(context.Background(), release.Assets[0], target))
			data, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, "verified-fixture-archive", string(data))
			require.Len(t, requests, 2)
		})
	}
}

func TestReleaseRedirectDoesNotExposeAccountToken(t *testing.T) {
	for _, target := range []string{
		"https://release-assets.githubusercontent.com/fixture.tar.gz",
		"https://untrusted.api.github.com/fixture.tar.gz",
		"https://example.invalid/fixture.tar.gz",
		"http://api.github.com/fixture.tar.gz",
	} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("GH_TOKEN", "fixture-private-token")
			old := updateHTTPClient
			t.Cleanup(func() { updateHTTPClient = old })
			var requests []string
			updateHTTPClient = &http.Client{Transport: releaseRoundTrip(func(r *http.Request) (*http.Response, error) {
				requests = append(requests, r.URL.String())
				if len(requests) == 1 {
					require.Equal(t, "Bearer fixture-private-token", r.Header.Get("Authorization"))
					return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Location": []string{target}}}, nil
				}
				require.Equal(t, target, r.URL.String())
				require.Empty(t, r.Header.Get("Authorization"))
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("fixture-archive")), Header: http.Header{}}, nil
			})}
			path := filepath.Join(t.TempDir(), "fixture.tar.gz")
			err := fetchReleaseAsset(context.Background(), releaseAsset{ID: 123, Name: "fixture.tar.gz", URL: "https://github.com/" + releaseRepository + "/releases/download/v1/fixture.tar.gz"}, path)
			if strings.HasPrefix(target, "http:") {
				require.ErrorContains(t, err, "insecure release redirect")
				require.Len(t, requests, 1)
				_, err = os.Stat(path)
				require.ErrorIs(t, err, os.ErrNotExist)
				return
			}
			require.NoError(t, err)
			require.Len(t, requests, 2)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "fixture-archive", string(data))
		})
	}
}

func TestPublicReleaseAssetAndBrowserFallback(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	old := updateHTTPClient
	t.Cleanup(func() { updateHTTPClient = old })
	for _, id := range []int64{123, 0} {
		updateHTTPClient = &http.Client{Transport: releaseRoundTrip(func(r *http.Request) (*http.Response, error) {
			require.Empty(t, r.Header.Get("Authorization"))
			require.Equal(t, "application/octet-stream", r.Header.Get("Accept"))
			if id > 0 {
				require.Equal(t, "api.github.com", r.URL.Host)
			} else {
				require.Equal(t, "github.com", r.URL.Host)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("public-fixture")), Header: http.Header{}}, nil
		})}
		require.NoError(t, fetchReleaseAsset(context.Background(), releaseAsset{ID: id, Name: "fixture", URL: "https://github.com/" + releaseRepository + "/releases/download/v1/fixture"}, filepath.Join(t.TempDir(), "fixture")))
	}
}
