package megafloydweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/legacy-ai/floyd/internal/megafloyd"
)

const (
	defaultStartupTimeout = 15 * time.Second
	defaultTokenTimeout   = 3 * time.Second
	defaultRCTimeout      = 20 * time.Second
	dialTimeout           = 200 * time.Millisecond
	pollInterval          = 100 * time.Millisecond
)

// Options configures a web handoff.
type Options struct {
	// HomeDir is the megafloyd home. Empty means HomeDir().
	HomeDir string
	// DataDir is the Node server's data directory. Empty means DataDir().
	DataDir string
	// CWD is the workspace the Node server should run in.
	CWD string
	// Executable helps discovery walk up from the running binary.
	Executable string
	// SessionID is the session the printed URL opens.
	SessionID string
	// GoHost and GoDataDir identify the authoritative agent backend.
	GoHost    string
	GoDataDir string
	// Port is the web server port. Zero means Port().
	Port int
	// Remote binds every interface. It is explicit only.
	Remote bool
	// RemoteControl exposes the server through the Floyd relay.
	RemoteControl bool
	// BaseEnv is the parent environment. Nil means os.Environ().
	BaseEnv []string
	// Tailscale is a pre-detected identity. The zero value triggers
	// detection unless SkipTailscale is set.
	Tailscale Tailscale
	// SkipTailscale disables detection entirely.
	SkipTailscale bool
	// DisableServe skips Tailscale Serve registration.
	DisableServe bool
	// Paths overrides resolved executables and assets.
	Paths Paths
	// Per-path discovery overrides.
	NodePath   string
	ServerPath string
	BundlePath string
	// Timeouts are exposed for callers that need tighter bounds.
	StartupTimeout       time.Duration
	TokenTimeout         time.Duration
	RemoteControlTimeout time.Duration
}

// Result describes a running (or reused) web server.
type Result struct {
	URL              string
	LocalURL         string
	RemoteControlURL string
	Token            string
	Port             int
	Bind             Bind
	PID              int
	Reused           bool
	Tailscale        Tailscale
	LogPath          string
	Warnings         []string
}

// StartWeb ensures the web server is running and returns its URLs. It is
// the engine entry point for a /web slash command.
func StartWeb(ctx context.Context, opts Options) (Result, error) {
	opts.RemoteControl = false
	return Start(ctx, opts)
}

// StartRemoteControl ensures the web server is running with the Floyd
// relay enabled and returns the remote-control URL. It is the engine entry
// point for a /remote-control slash command.
func StartRemoteControl(ctx context.Context, opts Options) (Result, error) {
	opts.RemoteControl = true
	return Start(ctx, opts)
}

// Start ensures the Node web server is running on the deterministic port,
// registers the Tailscale Serve route when possible, and returns the
// clickable URLs.
//
// A port that already accepts connections is treated as a running server
// and reused. The bearer token is read from the server's data directory;
// when it is not available yet the URL is returned without it and a
// warning explains why.
func Start(ctx context.Context, opts Options) (Result, error) {
	if megafloyd.IsActive() && opts.GoHost == "" {
		return Result{}, errors.New("web handoff requires the shared Go backend; restart MegaFloyd with client/server mode enabled")
	}
	if opts.GoHost != "" && opts.RemoteControl {
		return Result{}, errors.New("the shared Go runtime is available through the local web interface or Tailscale; relay remote control is not configured")
	}
	home := opts.HomeDir
	if home == "" {
		home = HomeDir()
	}
	if home == "" {
		return Result{}, errors.New("megafloyd home directory is not configured")
	}
	port := opts.Port
	if port == 0 {
		port = Port()
	}

	baseEnv := opts.BaseEnv
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	if opts.Remote && opts.RemoteControl {
		return Result{}, errors.New("remote control cannot use a non-loopback bind")
	}
	if opts.RemoteControl && strings.TrimSpace(LookupEnv(baseEnv, relayEnv)) == "" {
		return Result{}, fmt.Errorf(
			"remote control requires %s", relayEnv,
		)
	}

	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = DataDir(baseEnv, home)
	}

	var warnings []string
	info := opts.Tailscale
	if !info.Available() && !opts.SkipTailscale {
		detected, err := Detect(ctx)
		if err != nil {
			warnings = append(warnings, err.Error())
		} else {
			info = detected
		}
	}

	runningHost := ""
	if isListening("127.0.0.1", port) && probeHealth(ctx, "127.0.0.1", port) {
		runningHost = "127.0.0.1"
	}
	if runningHost == "" && info.IPv4 != "" && isListening(info.IPv4, port) &&
		probeHealth(ctx, info.IPv4, port) {
		runningHost = info.IPv4
	}
	if runningHost == "" &&
		(isListening("127.0.0.1", port) || (info.IPv4 != "" && isListening(info.IPv4, port))) {
		return Result{}, fmt.Errorf(
			"port %d is already in use by another service; set MEGAFLOYD_WEB_PORT",
			port,
		)
	}
	alreadyRunning := runningHost != ""
	if alreadyRunning && opts.GoHost != "" {
		if err := verifyRuntime(ctx, runningHost, port, opts); err != nil {
			return Result{}, fmt.Errorf("web server on port %d belongs to a different runtime: %w; close that server or select MEGAFLOYD_WEB_PORT", port, err)
		}
	}

	// Resolve the server and bundle before touching the tailnet serve
	// configuration, so a missing checkout fails cleanly and side-effect
	// free.
	var paths Paths
	if !alreadyRunning {
		paths = opts.Paths
		if paths.Server == "" || paths.Node == "" || paths.Bundle == "" {
			nodePath := opts.NodePath
			if nodePath == "" {
				nodePath = paths.Node
			}
			serverPath := opts.ServerPath
			if serverPath == "" {
				serverPath = paths.Server
			}
			bundlePath := opts.BundlePath
			if bundlePath == "" {
				bundlePath = paths.Bundle
			}
			resolved, err := Resolve(ResolveOptions{
				CWD:        opts.CWD,
				Executable: opts.Executable,
				NodePath:   nodePath,
				ServerPath: serverPath,
				BundlePath: bundlePath,
			})
			if err != nil {
				return Result{}, err
			}
			if paths.Node == "" {
				paths.Node = resolved.Node
			}
			if paths.Server == "" {
				paths.Server = resolved.Server
			}
			if paths.Bundle == "" {
				paths.Bundle = resolved.Bundle
			}
		}
	}

	serveOK := false
	if !opts.DisableServe && !opts.Remote && !opts.RemoteControl && info.DNSName != "" {
		if err := EnsureServe(ctx, port); err != nil {
			warnings = append(warnings, fmt.Sprintf("%v; use the local URL", err))
		} else {
			serveOK = true
		}
	}

	bind := ChooseBind(BindOptions{
		Remote:        opts.Remote,
		RemoteControl: opts.RemoteControl,
		Serve:         serveOK,
		Tailscale:     info,
	})
	if bind.Mode == BindAll {
		warnings = append(warnings,
			"bound to all interfaces; only do this on a trusted network")
	}

	childEnv := BuildChildEnv(baseEnv, info, dataDir)
	if opts.GoHost != "" {
		childEnv = setEnv(childEnv, "MEGAFLOYD_HOME", home)
		childEnv = setEnv(childEnv, "MEGAFLOYD_GO_HOST", opts.GoHost)
		childEnv = setEnv(childEnv, "MEGAFLOYD_GO_CWD", opts.CWD)
		childEnv = setEnv(childEnv, "MEGAFLOYD_GO_DATA_DIR", opts.GoDataDir)
	}
	result := Result{
		Port:      port,
		Bind:      bind,
		Tailscale: info,
		LogPath:   LogPath(home),
		Warnings:  warnings,
	}

	if !alreadyRunning {
		process, err := Spawn(SpawnOptions{
			HomeDir:       home,
			Dir:           opts.CWD,
			Env:           childEnv,
			Port:          port,
			Bind:          bind,
			Paths:         paths,
			AllowedHosts:  AllowedHosts(childEnv),
			RemoteControl: opts.RemoteControl,
		})
		if err != nil {
			return Result{}, err
		}
		result.PID = process.PID
		result.LogPath = process.LogPath

		startupTimeout := opts.StartupTimeout
		if startupTimeout <= 0 {
			startupTimeout = defaultStartupTimeout
		}
		if err := waitForServer(ctx, bind.Host, port, startupTimeout); err != nil {
			return Result{}, fmt.Errorf(
				"web server did not become ready: %w; see %s",
				err, result.LogPath,
			)
		}
	} else {
		result.Reused = true
	}

	if opts.GoHost != "" {
		if err := verifyRuntime(ctx, bind.Host, port, opts); err != nil {
			return Result{}, fmt.Errorf("web server does not use the expected Go runtime: %w; see %s", err, result.LogPath)
		}
	} else {
		tokenTimeout := opts.TokenTimeout
		if tokenTimeout <= 0 {
			tokenTimeout = defaultTokenTimeout
		}
		tokenPath := filepath.Join(dataDir, TokenFileName)
		result.Token = waitForToken(ctx, tokenPath, tokenTimeout)
		if result.Token == "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("web server token not found at %s; the URL will not include a token", tokenPath))
		}

	}

	result.URL = SessionURL(TailnetOrigin(info, serveOK, port), opts.SessionID, result.Token)
	localHost := "127.0.0.1"
	if bind.Mode == BindTailscale {
		localHost = bind.Host
	}
	result.LocalURL = SessionURL(HTTPOrigin(localHost, port), opts.SessionID, result.Token)

	if opts.RemoteControl {
		rcTimeout := opts.RemoteControlTimeout
		if rcTimeout <= 0 {
			rcTimeout = defaultRCTimeout
		}
		url, err := WaitRemoteControlURL(ctx, dataDir, rcTimeout)
		if err != nil {
			return result, fmt.Errorf("%w; see %s", err, result.LogPath)
		}
		result.RemoteControlURL = url
	}

	return result, nil
}

// WaitRemoteControlURL waits for the Node server to publish its relay URL
// in <dataDir>/server/rc.json.
func WaitRemoteControlURL(ctx context.Context, dataDir string, timeout time.Duration) (string, error) {
	path := filepath.Join(dataDir, "server", "rc.json")
	deadline := time.Now().Add(timeout)
	for {
		if url := readRemoteControlURL(path); url != "" {
			return url, nil
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("remote control did not publish a URL at %s", path)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func readRemoteControlURL(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var lock struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return ""
	}
	return strings.TrimSpace(lock.URL)
}

func isListening(host string, port int) bool {
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	conn, err := net.DialTimeout(
		"tcp",
		net.JoinHostPort(host, strconv.Itoa(port)),
		dialTimeout,
	)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitForServer(ctx context.Context, host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if probeHealth(ctx, host, port) {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for the web server health route")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func waitForToken(ctx context.Context, path string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for {
		if token := readToken(path); token != "" {
			return token
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return ""
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(pollInterval):
		}
	}
}

func readToken(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
