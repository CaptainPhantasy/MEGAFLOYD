package megafloydweb

import (
	"fmt"
	"os"
	"path/filepath"
)

// SpawnOptions describes a detached Node web server process.
type SpawnOptions struct {
	HomeDir       string
	Dir           string
	Env           []string
	Port          int
	Bind          Bind
	Paths         Paths
	AllowedHosts  []string
	RemoteControl bool
}

// Process is a started detached server.
type Process struct {
	PID     int
	LogPath string
}

// Spawn starts the Node web server in its own process group with stdin on
// /dev/null and stdout/stderr appended to the megafloyd log file.
//
// The process is released from the Go parent immediately, so the terminal
// pane never waits on it and closing the TUI cannot take the server down.
func Spawn(opts SpawnOptions) (Process, error) {
	if opts.HomeDir == "" {
		return Process{}, fmt.Errorf("megafloyd home directory is not configured")
	}

	logPath := LogPath(opts.HomeDir)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return Process{}, fmt.Errorf("create log directory: %w", err)
	}

	cmd, err := newCommand(commandSpec{
		Node:          opts.Paths.Node,
		Server:        opts.Paths.Server,
		Dir:           opts.Dir,
		Env:           opts.Env,
		Port:          opts.Port,
		Bind:          opts.Bind,
		AllowedHosts:  opts.AllowedHosts,
		RemoteControl: opts.RemoteControl,
	})
	if err != nil {
		return Process{}, err
	}

	stdin, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		return Process{}, fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer stdin.Close()

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return Process{}, fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdin = stdin
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setDetached(cmd)

	if err := cmd.Start(); err != nil {
		return Process{}, fmt.Errorf("start web server: %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return Process{}, fmt.Errorf("detach web server: %w", err)
	}
	return Process{PID: pid, LogPath: logPath}, nil
}
