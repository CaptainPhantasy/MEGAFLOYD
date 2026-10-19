package megafloydweb

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// commandSpec is everything needed to build the Node server command. It is
// separate from Spawn so tests can inspect the command without starting a
// process.
type commandSpec struct {
	Node          string
	Server        string
	Dir           string
	Env           []string
	Port          int
	Bind          Bind
	AllowedHosts  []string
	RemoteControl bool
}

// args builds the Node command line. It mirrors the verified `floyd web`
// options: --no-open, --port, optional --host, --insecure-no-tls for a
// non-loopback bind, --rc for remote control, and repeated --allowed-host.
func (s commandSpec) args() []string {
	args := []string{
		s.Server,
		"web",
		"--no-open",
		"--port", strconv.Itoa(s.Port),
		"--host", s.Bind.Host,
	}
	if !IsLoopback(s.Bind.Host) {
		args = append(args, "--insecure-no-tls")
	}
	if s.RemoteControl {
		args = append(args, "--rc")
	}
	for _, host := range s.AllowedHosts {
		host = strings.TrimSpace(host)
		if host == "" || strings.HasPrefix(host, "-") {
			continue
		}
		args = append(args, "--allowed-host", host)
	}
	return args
}

// newCommand validates the spec and builds an exec.Cmd without starting it.
func newCommand(spec commandSpec) (*exec.Cmd, error) {
	switch {
	case spec.Server == "":
		return nil, errors.New("web server script is not configured")
	case spec.Node == "":
		return nil, errors.New("node executable is not configured")
	case spec.Port < 1 || spec.Port > 65535:
		return nil, fmt.Errorf("invalid web server port %d", spec.Port)
	case spec.Bind.Host == "":
		return nil, errors.New("web server bind host is not configured")
	}

	cmd := exec.Command(spec.Node, spec.args()...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	return cmd, nil
}
