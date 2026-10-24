// Package megafloydhandoff opens clients of the existing canonical runtime.
package megafloydhandoff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/legacy-ai/floyd/internal/megafloydweb"
)

// Options comes from a validated native workspace and session.
type Options struct {
	CWD, DataDir, SessionID, Host string
	Home, Executable              string
	WebPort                       int
}

// Descriptor keeps every client on the same native workspace and session.
type Descriptor struct {
	CWD             string `json:"cwd"`
	DataDir         string `json:"data_dir"`
	SessionID       string `json:"session_id"`
	Host            string `json:"host"`
	WebURL          string `json:"web_url"`
	DesktopURL      string `json:"desktop_url"`
	TerminalCommand string `json:"terminal_command"`
	Target          string `json:"target,omitempty"`
	Opened          bool   `json:"opened,omitempty"`
}

type installation struct{ app, launcher, platform string }

var webOrigins sync.Map
var webStartMu sync.Mutex
var ErrInvalidTarget = errors.New("handoff target must be web, tui, or desktop")

func installed(executable string) (installation, error) {
	if executable == "" {
		var err error
		executable, err = os.Executable()
		if err != nil {
			return installation{}, err
		}
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	dir := filepath.Dir(executable)
	for range 12 {
		if strings.HasSuffix(dir, ".app") {
			root := filepath.Dir(dir)
			launcher := filepath.Join(root, "bin", "megafloyd")
			platform := filepath.Join(dir, "Contents", "Resources", "megafloyd")
			if isFile(launcher) && isFile(filepath.Join(platform, "bin", "megafloyd")) {
				return installation{dir, launcher, platform}, nil
			}
		}
		// A source verification binary can still discover its canonical install.
		app := filepath.Join(dir, "current", "MEGAFLOYD.app")
		if resolved, err := filepath.EvalSymlinks(app); err == nil {
			launcher := filepath.Join(filepath.Dir(resolved), "bin", "megafloyd")
			platform := filepath.Join(resolved, "Contents", "Resources", "megafloyd")
			if isFile(launcher) && isFile(filepath.Join(platform, "bin", "megafloyd")) {
				return installation{resolved, launcher, platform}, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return installation{}, errors.New("canonical MegaFloyd installation was not found beside this executable")
}
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
func normalize(opts Options) (Options, error) {
	if opts.CWD == "" || !filepath.IsAbs(opts.CWD) {
		return opts, errors.New("handoff requires the absolute workspace directory")
	}
	if info, err := os.Stat(opts.CWD); err != nil || !info.IsDir() {
		return opts, fmt.Errorf("handoff workspace is not a directory: %s", opts.CWD)
	}
	if opts.Host == "" {
		return opts, errors.New("handoff requires the shared Go server host")
	}
	if opts.DataDir == "" || !filepath.IsAbs(opts.DataDir) {
		return opts, errors.New("handoff requires the absolute workspace data directory")
	}
	if opts.Home == "" {
		var err error
		opts.Home, err = megafloyd.HomeDir()
		if err != nil {
			return opts, err
		}
	}
	return opts, nil
}
func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func terminalCommand(opts Options, inst installation) string {
	args := []string{inst.launcher, "--host", opts.Host, "--cwd", opts.CWD, "--data-dir", opts.DataDir}
	if opts.SessionID != "" {
		args = append(args, "--session", opts.SessionID)
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = quote(arg)
	}
	return "env MEGAFLOYD_HOME=" + quote(opts.Home) + " " + strings.Join(quoted, " ")
}
func descriptor(opts Options, inst installation) Descriptor {
	link := url.URL{Scheme: "megafloyd", Host: "workspace"}
	if opts.SessionID != "" {
		link.Host = "session"
		link.Path = "/" + opts.SessionID
	}
	query := url.Values{"cwd": {opts.CWD}, "host": {opts.Host}, "data_dir": {opts.DataDir}, "home": {opts.Home}}
	link.RawQuery = query.Encode()
	return Descriptor{CWD: opts.CWD, DataDir: opts.DataDir, SessionID: opts.SessionID, Host: opts.Host, DesktopURL: link.String(), TerminalCommand: terminalCommand(opts, inst)}
}

// Describe returns launch information without starting any client.
func Describe(ctx context.Context, opts Options) (Descriptor, error) {
	opts, err := normalize(opts)
	if err != nil {
		return Descriptor{}, err
	}
	inst, err := installed(opts.Executable)
	if err != nil {
		return Descriptor{}, err
	}
	result := descriptor(opts, inst)
	if origin := findWeb(ctx, opts); origin != "" {
		result.WebURL = megafloydweb.SessionURL(origin, opts.SessionID, "")
	}
	return result, nil
}

// RegisterWeb records a listening adapter only after proving its Go identity.
func RegisterWeb(ctx context.Context, host, origin string) error {
	origin, err := localOrigin(origin)
	if err != nil {
		return err
	}
	if err := probeWeb(ctx, origin, Options{Host: host}, false); err != nil {
		return err
	}
	webOrigins.Store(host, origin)
	return nil
}
func localOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Port() == "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("web handoff requires a local HTTP origin with an explicit port")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("web handoff origin must use a loopback address")
	}
	return u.Scheme + "://" + u.Host, nil
}
func readJSON(ctx context.Context, endpoint string, target any) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("web identity probe redirected") }}
	rsp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("web identity returned HTTP %d", rsp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(rsp.Body, 64<<10)).Decode(target)
}
func probeWeb(ctx context.Context, origin string, opts Options, sessionCheck bool) error {
	var identity struct {
		Code int `json:"code"`
		Data struct {
			Engine, Host, CWD string
			DataDir           string `json:"data_dir"`
		} `json:"data"`
	}
	if err := readJSON(ctx, origin+"/api/v1/runtime", &identity); err != nil {
		return err
	}
	if identity.Code != 0 || identity.Data.Engine != "go" || identity.Data.Host != opts.Host {
		return errors.New("web adapter belongs to a different Go runtime")
	}
	if !sessionCheck {
		return nil
	}
	if opts.SessionID == "" {
		if !samePath(identity.Data.CWD, opts.CWD) || !samePath(identity.Data.DataDir, opts.DataDir) {
			return errors.New("select a session before opening another workspace in the web client")
		}
		return nil
	}
	var session struct {
		Code int `json:"code"`
		Data struct {
			ID       string `json:"id"`
			Metadata struct {
				CWD string `json:"cwd"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if err := readJSON(ctx, origin+"/api/v1/sessions/"+url.PathEscape(opts.SessionID), &session); err != nil {
		return err
	}
	if session.Code != 0 || session.Data.ID != opts.SessionID || !samePath(session.Data.Metadata.CWD, opts.CWD) {
		return errors.New("web adapter did not resolve the requested native workspace and session")
	}
	return nil
}
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	if v, e := filepath.EvalSymlinks(a); e == nil {
		a = v
	}
	if v, e := filepath.EvalSymlinks(b); e == nil {
		b = v
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
func findWeb(ctx context.Context, opts Options) string {
	candidates := []string{fmt.Sprintf("http://127.0.0.1:%d", preferredPort(opts))}
	if origin, ok := webOrigins.Load(opts.Host); ok {
		candidates = append([]string{origin.(string)}, candidates...)
	}
	for _, origin := range candidates {
		if probeWeb(ctx, origin, opts, true) == nil {
			return origin
		}
	}
	return ""
}
func ensureWeb(ctx context.Context, opts Options, inst installation) (string, error) {
	webStartMu.Lock()
	defer webStartMu.Unlock()
	if origin := findWeb(ctx, opts); origin != "" {
		return origin, nil
	}
	port, err := availableWebPort(opts)
	if err != nil {
		return "", err
	}
	result, err := megafloydweb.StartWeb(ctx, megafloydweb.Options{HomeDir: opts.Home, Port: port, CWD: opts.CWD, SessionID: opts.SessionID, GoHost: opts.Host, GoDataDir: opts.DataDir, SkipTailscale: true, DisableServe: true, Paths: megafloydweb.Paths{Node: filepath.Join(inst.platform, "bin", "node"), Server: filepath.Join(inst.platform, "web", "dist", "main.mjs"), Bundle: filepath.Join(inst.platform, "web", "dist-web")}})
	if err != nil {
		return "", err
	}
	origin := fmt.Sprintf("http://127.0.0.1:%d", result.Port)
	if err := probeWeb(ctx, origin, opts, true); err != nil {
		return "", err
	}
	webOrigins.Store(opts.Host, origin)
	return origin, nil
}

// EnsureWeb starts or reuses the shared adapter without opening a browser.
func EnsureWeb(ctx context.Context, opts Options) (Descriptor, error) {
	opts, err := normalize(opts)
	if err != nil {
		return Descriptor{}, err
	}
	inst, err := installed(opts.Executable)
	if err != nil {
		return Descriptor{}, err
	}
	result := descriptor(opts, inst)
	result.Target = "web"
	origin, err := ensureWeb(ctx, opts, inst)
	if err != nil {
		return result, err
	}
	result.WebURL = megafloydweb.SessionURL(origin, opts.SessionID, "")
	return result, nil
}

// availableWebPort never takes a listener owned by another runtime. Explicit
// choices fail; the ordinary default can use another free loopback port.
func availableWebPort(opts Options) (int, error) {
	port := preferredPort(opts)
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		if opts.WebPort > 0 {
			return 0, fmt.Errorf("requested web port %d is unavailable: %w", port, err)
		}
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, fmt.Errorf("choose an available web port: %w", err)
		}
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func preferredPort(opts Options) int {
	if opts.WebPort > 0 {
		return opts.WebPort
	}
	return megafloydweb.Port()
}

// Open launches a client without owning or cancelling its native agent run.
func Open(ctx context.Context, opts Options, target string) (Descriptor, error) {
	if target != "web" && target != "tui" && target != "desktop" {
		return Descriptor{}, ErrInvalidTarget
	}
	opts, err := normalize(opts)
	if err != nil {
		return Descriptor{}, err
	}
	inst, err := installed(opts.Executable)
	if err != nil {
		return Descriptor{}, err
	}
	result := descriptor(opts, inst)
	result.Target = target
	if target == "web" {
		origin, err := ensureWeb(ctx, opts, inst)
		if err != nil {
			return result, err
		}
		result.WebURL = megafloydweb.SessionURL(origin, opts.SessionID, "")
		err = openURL(ctx, result.WebURL)
		if err != nil {
			return result, err
		}
	} else if target == "desktop" {
		if runtime.GOOS != "darwin" {
			return result, errors.New("this desktop package requires macOS")
		}
		if err := exec.CommandContext(ctx, "/usr/bin/open", "-a", inst.app, "--env", "MEGAFLOYD_HOME="+opts.Home, "-u", result.DesktopURL).Run(); err != nil {
			return result, fmt.Errorf("open MegaFloyd desktop: %w", err)
		}
	} else {
		if err := openTerminal(ctx, opts, result.TerminalCommand); err != nil {
			return result, err
		}
	}
	result.Opened = true
	return result, nil
}
func openURL(ctx context.Context, raw string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "/usr/bin/open", raw)
	case "linux":
		cmd = exec.CommandContext(ctx, "xdg-open", raw)
	default:
		return errors.New("automatic browser opening is unsupported on this platform")
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
func openTerminal(ctx context.Context, opts Options, command string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("open a terminal and run: %s", command)
	}
	dir := filepath.Join(opts.Home, "handoff")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, "session-*.command")
	if err != nil {
		return err
	}
	name := file.Name()
	// The one-use script removes only itself, then the terminal owns the client.
	script := "#!/bin/sh\nset -eu\nrm -f -- \"$0\"\nexec " + command + "\n"
	if _, err = file.WriteString(script); err == nil {
		err = file.Chmod(0o700)
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := exec.CommandContext(ctx, "/usr/bin/open", "-a", "Terminal", name).Run(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("open terminal: %w", err)
	}
	return nil
}
