// Package memory writes the project memory checkpoint that the prompt layer
// injects into its dynamic context.
//
// The checkpoint lives at <cwd>/.floyd/.supercache and is a small JSON
// document describing where the last session was working. The prompt package
// reads it on every request (internal/agent/prompt/cache.go), so this package
// owns the missing write side: startup and shutdown both record a fresh
// checkpoint.
//
// Documents are capped at [MaxSize] bytes and written atomically. They carry
// no prompt contents and no secrets: only a timestamp, the working
// directory, the last session identifier, a short git summary, and an
// optional caller-supplied note.
package memory

import (
	"path/filepath"
	"time"
)

const (
	// DirName is the project metadata directory under the working directory.
	DirName = ".floyd"
	// FileName is the checkpoint file the prompt layer reads.
	FileName = ".supercache"
	// MaxSize is the hard upper bound for a checkpoint file, matching the
	// prompt layer's read cap.
	MaxSize = 2048
	// DefaultGitTimeout bounds the git summary probe.
	DefaultGitTimeout = time.Second
)

// Checkpoint is the JSON document persisted at [Path].
type Checkpoint struct {
	Timestamp time.Time `json:"timestamp"`
	CWD       string    `json:"cwd"`
	SessionID string    `json:"session_id,omitempty"`
	Git       *GitState `json:"git,omitempty"`
	Note      string    `json:"note,omitempty"`
}

// GitState summarizes the working tree at checkpoint time. File names are
// deliberately omitted: the prompt layer only needs the branch and how dirty
// the tree is.
type GitState struct {
	Branch  string `json:"branch,omitempty"`
	Dirty   bool   `json:"dirty"`
	Changes int    `json:"changes,omitempty"`
}

// Path returns the checkpoint path for a working directory.
func Path(cwd string) string {
	return filepath.Join(cwd, DirName, FileName)
}
