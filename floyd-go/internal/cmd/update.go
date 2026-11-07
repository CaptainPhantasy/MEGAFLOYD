package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/legacy-ai/floyd/internal/version"
	"github.com/spf13/cobra"
)

const releaseRepository = "CaptainPhantasy/MEGAFLOYD"

var (
	updateHTTPClient = &http.Client{Timeout: 60 * time.Second}
	latestReleaseURL = "https://api.github.com/repos/" + releaseRepository + "/releases/latest"
)

type releaseAsset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}
type cliRelease struct {
	Tag    string         `json:"tag_name"`
	URL    string         `json:"html_url"`
	Assets []releaseAsset `json:"assets"`
}

func latestCLIRelease(ctx context.Context) (*cliRelease, error) {
	response, err := releaseRequest(ctx, latestReleaseURL, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		return nil, nil
	}
	if response.StatusCode != 200 {
		if response.StatusCode == http.StatusForbidden && response.Header.Get("X-RateLimit-Remaining") == "0" {
			return nil, errors.New("GitHub release API rate limit reached; set GH_TOKEN or GITHUB_TOKEN for authenticated checks, or retry after the rate limit resets")
		}
		return nil, fmt.Errorf("release service returned HTTP %d", response.StatusCode)
	}
	var release cliRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&release); err != nil {
		return nil, err
	}
	if release.Tag == "" {
		return nil, errors.New("release metadata is missing its version")
	}
	return &release, nil
}

func releaseRequest(ctx context.Context, target, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if githubReleaseHost(req.URL) {
		token := os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "MegaFloyd/"+version.Version)
	client := *updateHTTPClient
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		// GitHub's signed asset CDN URLs need no account token. Explicitly
		// strip it even for subdomains, which net/http otherwise trusts.
		if !githubReleaseHost(next.URL) {
			next.Header.Del("Authorization")
		}
		if next.URL.Scheme != "https" {
			return errors.New("refusing an insecure release redirect")
		}
		if len(via) >= 10 {
			return errors.New("too many release redirects")
		}
		if updateHTTPClient.CheckRedirect != nil {
			return updateHTTPClient.CheckRedirect(next, via)
		}
		return nil
	}
	return client.Do(req)
}

func githubReleaseHost(endpoint *url.URL) bool {
	return endpoint.Scheme == "https" && endpoint.User == nil && (endpoint.Host == "api.github.com" || endpoint.Host == "github.com")
}

// Find the updater distributed with the package, or the repository source used
// for this development build. Never execute a script from an arbitrary cwd.
func releaseInstaller() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	for dir := filepath.Dir(exe); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "install.py")
		if _, err := os.Stat(filepath.Join(dir, "release.json")); err == nil {
			if fi, err := os.Stat(candidate); err == nil && fi.Mode().IsRegular() {
				return candidate, nil
			}
		}
	}
	_, source, _, ok := runtime.Caller(0)
	if ok {
		candidate := filepath.Clean(filepath.Join(filepath.Dir(source), "../../..", "scripts/megafloyd-release.py"))
		if fi, err := os.Stat(candidate); err == nil && fi.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", errors.New("the package updater is missing; restore install.py from this MegaFloyd release before updating")
}

func fetchReleaseAsset(ctx context.Context, asset releaseAsset, path string) error {
	// Download only the release assets of this repository. Redirects to GitHub's
	// asset CDN are handled by the HTTP client; arbitrary mirror URLs are rejected.
	if !strings.HasPrefix(asset.URL, "https://github.com/"+releaseRepository+"/releases/download/") {
		return fmt.Errorf("unexpected release asset source for %s", asset.Name)
	}
	target := asset.URL
	if asset.ID > 0 {
		// The API supports bearer tokens for private release assets and
		// returns the bytes or a redirect to GitHub's signed asset CDN.
		target = fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", releaseRepository, asset.ID)
	}
	response, err := releaseRequest(ctx, target, "application/octet-stream")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("download %s: HTTP %d", asset.Name, response.StatusCode)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, io.LimitReader(response.Body, 2<<30))
	return errors.Join(copyErr, file.Close())
}

func newUpdateCommand() *cobra.Command {
	command := &cobra.Command{Use: "update", Short: "Check or install a verified complete MegaFloyd release", Long: "Updates come only from CaptainPhantasy/MEGAFLOYD. The existing package verifier checks the manifest, archive, complete inventory, policy, and native binary before atomic installation. Previous releases are retained.", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		check, _ := c.Flags().GetBool("check")
		prefix, _ := c.Flags().GetString("prefix")
		manifest, _ := c.Flags().GetString("manifest")
		archive, _ := c.Flags().GetString("archive")
		verifyOnly, _ := c.Flags().GetBool("verify-only")
		if manifest == "" {
			release, err := latestCLIRelease(c.Context())
			if err != nil {
				return err
			}
			if release == nil {
				_, err = fmt.Fprintln(c.OutOrStdout(), "No published releases are available in "+releaseRepository+". Current version: "+version.Version)
				return err
			}
			if check {
				return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"repository": releaseRepository, "current": version.Version, "latest": release.Tag, "update_available": strings.TrimPrefix(release.Tag, "v") != strings.TrimPrefix(version.Version, "v"), "url": release.URL})
			}
			if prefix == "" {
				return fmt.Errorf("specify --prefix with the existing installation directory; current version %s, available %s", version.Version, release.Tag)
			}
			stage, err := os.MkdirTemp(prefix, ".megafloyd-download-")
			if err != nil {
				return err
			}
			target := "megafloyd-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
			for _, asset := range release.Assets {
				if asset.Name != "manifest.json" && asset.Name != target {
					continue
				}
				path := filepath.Join(stage, asset.Name)
				if err := fetchReleaseAsset(c.Context(), asset, path); err != nil {
					return err
				}
				if asset.Name == "manifest.json" {
					manifest = path
				} else {
					archive = path
				}
			}
			if manifest == "" || archive == "" {
				return fmt.Errorf("release %s lacks manifest.json or %s; downloads retained in %s", release.Tag, target, stage)
			}
		}
		installer, err := releaseInstaller()
		if err != nil {
			return err
		}
		if prefix == "" {
			return fmt.Errorf("--prefix must identify the installation directory")
		}
		args := []string{installer, "install", "--prefix", prefix, "--manifest", manifest}
		if archive != "" {
			args = append(args, "--archive", archive)
		}
		if check {
			args = append(args, "--check")
		}
		if verifyOnly {
			args = append(args, "--verify-only")
		}
		process := exec.CommandContext(c.Context(), "python3", args...)
		process.Stdin = c.InOrStdin()
		process.Stdout = c.OutOrStdout()
		process.Stderr = c.ErrOrStderr()
		return process.Run()
	}}
	command.Flags().Bool("check", false, "Check available versions without installing")
	command.Flags().String("prefix", "", "Existing absolute installation directory")
	command.Flags().String("manifest", "", "Local release manifest for an offline update")
	command.Flags().String("archive", "", "Local release archive for an offline update")
	command.Flags().Bool("verify-only", false, "Verify the complete release package without installing")
	return command
}
func init() { rootCmd.AddCommand(newUpdateCommand()) }
