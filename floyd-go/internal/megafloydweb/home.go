package megafloydweb

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// DefaultPort is the Node server's default port. A fixed port makes
	// "start or reuse" deterministic across invocations and matches the
	// existing tailnet serve route.
	DefaultPort = 58627

	// TokenFileName is written by the Node server on first boot.
	TokenFileName = "server.token"

	// BundleDirName is the committed web client directory inside the
	// Node server's package root.
	BundleDirName = "dist-web"

	// LogFileName is the detached server's combined stdout/stderr log.
	LogFileName = "logs/web-server.log"

	// DefaultDataDirName is the server's data directory under the
	// megafloyd home when MEGAFLOYD_WEB_DATA is not explicitly set.
	DefaultDataDirName = "web"
)

const (
	allowedHostsEnv = "FLOYD_CODE_ALLOWED_HOSTS"
	codeHomeEnv     = "FLOYD_CODE_HOME"
	webDataEnv      = "MEGAFLOYD_WEB_DATA"
	rolePromptEnv   = "FLOYD_ROLE_ADDITIONAL"
	homeEnv         = "MEGAFLOYD_HOME"
	nodeEnv         = "MEGAFLOYD_WEB_NODE"
	electronNodeEnv = "ELECTRON_RUN_AS_NODE"
	scriptEnv       = "MEGAFLOYD_WEB_SCRIPT"
	bundleEnv       = "MEGAFLOYD_WEB_BUNDLE"
	codeRootEnv     = "MEGAFLOYD_CODE_ROOT"
	portEnv         = "MEGAFLOYD_WEB_PORT"
	remoteEnv       = "MEGAFLOYD_WEB_REMOTE"
	relayEnv        = "FLOYD_CODE_REMOTE_CONTROL_RELAY_URL"
)

// HomeDir returns the megafloyd home directory. MEGAFLOYD_HOME wins;
// otherwise it is ~/.megafloyd, matching internal/megafloydauth.
func HomeDir() string {
	if dir := strings.TrimSpace(os.Getenv(homeEnv)); dir != "" {
		return dir
	}
	dir, err := os.UserHomeDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, ".megafloyd")
}

// Port returns the web server port. MEGAFLOYD_WEB_PORT overrides the
// default; invalid or out-of-range values fall back to DefaultPort.
func Port() int {
	raw := strings.TrimSpace(os.Getenv(portEnv))
	if raw == "" {
		return DefaultPort
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return DefaultPort
	}
	return port
}

// Remote reports whether the server should bind every interface. It is
// opt-in: MEGAFLOYD_WEB_REMOTE accepts the usual boolean spellings.
func Remote() bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(remoteEnv)))
	return err == nil && value
}

// DataDir returns the canonical web store. Only MEGAFLOYD_WEB_DATA can
// override its default; inherited parent-harness homes are ignored.
func DataDir(env []string, home string) string {
	if dir := strings.TrimSpace(LookupEnv(env, webDataEnv)); dir != "" {
		return dir
	}
	return filepath.Join(home, DefaultDataDirName)
}

// LogPath returns the detached server's log file path.
func LogPath(home string) string {
	return filepath.Join(home, filepath.FromSlash(LogFileName))
}

// LookupEnv returns the last non-empty value for key in an environment
// slice. Later entries win, mirroring how exec.Cmd resolves duplicates.
func LookupEnv(env []string, key string) string {
	value := ""
	for _, entry := range env {
		k, v, ok := strings.Cut(entry, "=")
		if ok && k == key {
			value = v
		}
	}
	return value
}
