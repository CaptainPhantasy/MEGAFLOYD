package megafloydhandoff

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalHandoffPreservesContextAndShellArguments(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cwd := filepath.Join(root, "owner's project; no shell expansion")
	require.NoError(t, os.Mkdir(cwd, 0o700))
	inst := installation{app: filepath.Join(root, "MEGAFLOYD.app"), launcher: filepath.Join(root, "launch ' exact"), platform: root}
	require.NoError(t, os.WriteFile(inst.launcher, []byte("#!/bin/sh\nprintf '%s\\n' \"$MEGAFLOYD_HOME\" \"$@\"\n"), 0o700))
	opts := Options{CWD: cwd, DataDir: filepath.Join(cwd, ".megafloyd"), Host: "unix:///tmp/megafloyd-example.sock", Home: filepath.Join(root, "own home"), SessionID: "41cffa2c-231a-46d8-b609-a729e74d78a8"}
	result := descriptor(opts, inst)
	out, err := exec.Command("sh", "-c", result.TerminalCommand).Output()
	require.NoError(t, err)
	require.Equal(t, []string{opts.Home, "--host", opts.Host, "--cwd", opts.CWD, "--data-dir", opts.DataDir, "--session", opts.SessionID}, strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"))
	link, err := url.Parse(result.DesktopURL)
	require.NoError(t, err)
	require.Equal(t, "megafloyd", link.Scheme)
	require.Equal(t, "session", link.Host)
	require.Equal(t, "/"+opts.SessionID, link.Path)
	require.Equal(t, opts.CWD, link.Query().Get("cwd"))
	require.Equal(t, opts.DataDir, link.Query().Get("data_dir"))
	require.Equal(t, opts.Host, link.Query().Get("host"))
}

func TestCanonicalHandoffVerifiesSharedAdapterAndSessionOwnership(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "target")
	require.NoError(t, os.Mkdir(target, 0o700))
	const host = "unix:///tmp/megafloyd-shared-test.sock"
	const sid = "session-b"
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/runtime", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"engine": "go", "host": host, "cwd": filepath.Join(root, "primary"), "data_dir": filepath.Join(root, "primary", ".megafloyd")}})
	})
	handler.HandleFunc("GET /api/v1/sessions/{sid}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"id": r.PathValue("sid"), "metadata": map[string]any{"cwd": target}}})
	})
	adapter := httptest.NewServer(handler)
	defer adapter.Close()
	opts := Options{Host: host, CWD: target, DataDir: filepath.Join(target, ".megafloyd"), SessionID: sid}
	require.NoError(t, RegisterWeb(context.Background(), host, adapter.URL))
	t.Cleanup(func() { webOrigins.Delete(host) })
	require.Equal(t, adapter.URL, findWeb(context.Background(), opts), "a second workspace must reuse the same Go adapter")
	foreign := opts
	foreign.CWD = filepath.Join(root, "unrelated")
	require.ErrorContains(t, probeWeb(context.Background(), adapter.URL, foreign, true), "requested native workspace")
	require.ErrorContains(t, RegisterWeb(context.Background(), "unix:///tmp/other.sock", adapter.URL), "different Go runtime")
	require.Error(t, RegisterWeb(context.Background(), host, "http://example.com:80"))
}

func TestCanonicalHandoffUsesInstalledReleaseLauncher(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	app := filepath.Join(root, "releases", "release one", "MEGAFLOYD.app")
	binary := filepath.Join(app, "Contents", "Resources", "megafloyd", "bin", "megafloyd")
	launcher := filepath.Join(filepath.Dir(app), "bin", "megafloyd")
	for _, p := range []string{binary, launcher} {
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
		require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	}
	inst, err := installed(binary)
	require.NoError(t, err)
	resolvedLauncher, err := filepath.EvalSymlinks(launcher)
	require.NoError(t, err)
	require.Equal(t, resolvedLauncher, inst.launcher)
	resolvedApp, err := filepath.EvalSymlinks(app)
	require.NoError(t, err)
	require.Equal(t, resolvedApp, inst.app)
	require.NotEqual(t, binary, inst.launcher, "the release launcher provides the bundled web environment")
}

func TestCanonicalHandoffDefaultPortCollision(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	occupied := listener.Addr().(*net.TCPAddr).Port
	t.Setenv("MEGAFLOYD_WEB_PORT", fmt.Sprint(occupied))
	port, err := availableWebPort(Options{})
	require.NoError(t, err)
	require.NotEqual(t, occupied, port, "another account's adapter keeps its listener")
	free, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	require.NoError(t, free.Close())
	_, err = availableWebPort(Options{WebPort: occupied})
	require.ErrorContains(t, err, "requested web port")
	connection, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err, "the other listener remains available")
	require.NoError(t, connection.Close())
}
