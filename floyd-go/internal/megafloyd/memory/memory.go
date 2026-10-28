// Package memory writes the project memory checkpoint that the prompt layer
// injects into its dynamic context.
//
// The checkpoint is a small JSON document describing where the last session
// was working. It is stored under the account's own data directory: the
// canonical MegaFloyd binary keeps project state in <cwd>/.megafloyd, while
// the legacy scaffold falls back to <cwd>/.floyd. [Path] reports the default
// location for a working directory and [Writer.DataDir] overrides it with the
// store the caller already resolved. The prompt package reads the checkpoint
// on every request (internal/agent/prompt/cache.go), so this package owns the
// missing write side: startup and shutdown both record a fresh checkpoint.
//
// Documents are capped at [MaxSize] bytes and written atomically. They carry
// no prompt contents and no secrets: only a timestamp, the working
// directory, the last session identifier, a short git summary, and an
// optional caller-supplied note.
package memory

import (
	"github.com/legacy-ai/floyd/internal/megafloyd"
	"path/filepath"
	"time"
)

const (
	// DirName is the legacy project metadata directory under the working
	// directory. The canonical binary uses megafloyd.ProjectDirName() instead,
	// so the git probe resolves the directory it actually wrote to rather than
	// assuming this name.
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

// Path returns the default checkpoint path for a working directory, using the
// directory name this identity selects. A caller that already resolved the
// account's workspace store must set [Writer.DataDir], which keeps the
// checkpoint outside the project tree.
func Path(cwd string) string {
	return filepath.Join(cwd, megafloyd.ProjectDirName(), FileName)
}
