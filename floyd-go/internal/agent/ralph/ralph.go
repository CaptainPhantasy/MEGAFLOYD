// Package ralph implements the Ralph Loop — an iterative self-referential
// development loop where the same prompt is fed to the agent repeatedly.
// The agent sees its previous work in files and git history, enabling
// systematic improvement through iteration.
//
// Based on the Ralph Wiggum technique by Geoffrey Huntley.
// See: https://ghuntley.com/ralph/
package ralph

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// StateFileName is the ralph loop state file within .floyd/
	StateFileName = "ralph-loop.state.yaml"
	// DefaultMaxIterations is 0 (unlimited).
	DefaultMaxIterations = 0
)

// State holds the current ralph loop state, persisted to disk.
type State struct {
	Active            bool   `yaml:"active"`
	Iteration         int    `yaml:"iteration"`
	MaxIterations     int    `yaml:"max_iterations"`
	CompletionPromise string `yaml:"completion_promise"`
	Prompt            string `yaml:"prompt"`
	SessionID         string `yaml:"session_id"`
	StartedAt         string `yaml:"started_at"`
}

// Loop manages a ralph loop lifecycle.
type Loop struct {
	mu       sync.Mutex
	stateDir string // .floyd directory path
}

// New creates a new Loop bound to the given .floyd directory.
func New(floydDir string) *Loop {
	return &Loop{stateDir: floydDir}
}

func (l *Loop) statePath() string {
	return filepath.Join(l.stateDir, StateFileName)
}

// Start initializes a new ralph loop with the given parameters.
func (l *Loop) Start(sessionID, prompt string, maxIterations int, completionPromise string) (*State, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if prompt == "" {
		return nil, fmt.Errorf("ralph loop requires a prompt")
	}

	state := &State{
		Active:            true,
		Iteration:         1,
		MaxIterations:     maxIterations,
		CompletionPromise: completionPromise,
		Prompt:            prompt,
		SessionID:         sessionID,
		StartedAt:         time.Now().UTC().Format(time.RFC3339),
	}

	if err := l.writeState(state); err != nil {
		return nil, fmt.Errorf("failed to write ralph state: %w", err)
	}

	slog.Info("Ralph loop started",
		"session_id", sessionID,
		"max_iterations", maxIterations,
		"has_promise", completionPromise != "",
	)

	return state, nil
}

// Check evaluates whether the loop should continue after an agent turn.
// Returns (shouldContinue, nextPrompt, systemMessage).
// If shouldContinue is false, the loop is done and the state file is cleaned up.
func (l *Loop) Check(sessionID, lastAssistantOutput string) (bool, string, string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	state, err := l.readState()
	if err != nil || state == nil || !state.Active {
		return false, "", ""
	}

	// Session isolation: only the session that started the loop can continue it.
	if state.SessionID != "" && state.SessionID != sessionID {
		return false, "", ""
	}

	// Check completion promise
	if state.CompletionPromise != "" {
		promiseText := extractPromise(lastAssistantOutput)
		if promiseText == state.CompletionPromise {
			slog.Info("Ralph loop: completion promise detected",
				"promise", state.CompletionPromise,
				"iteration", state.Iteration,
			)
			l.cleanup()
			return false, "", ""
		}
	}

	// Check max iterations
	if state.MaxIterations > 0 && state.Iteration >= state.MaxIterations {
		slog.Info("Ralph loop: max iterations reached",
			"max", state.MaxIterations,
			"iteration", state.Iteration,
		)
		l.cleanup()
		return false, "", ""
	}

	// Continue — increment iteration
	state.Iteration++
	if writeErr := l.writeState(state); writeErr != nil {
		slog.Error("Ralph loop: failed to update state", "error", writeErr)
		l.cleanup()
		return false, "", ""
	}

	// Build system message
	var sysMsg string
	if state.CompletionPromise != "" {
		sysMsg = fmt.Sprintf(
			"Ralph iteration %d | To complete: output <promise>%s</promise> (ONLY when genuinely true)",
			state.Iteration, state.CompletionPromise,
		)
	} else {
		sysMsg = fmt.Sprintf(
			"Ralph iteration %d | No completion promise set — loop continues until max iterations",
			state.Iteration,
		)
	}

	return true, state.Prompt, sysMsg
}

// Cancel stops an active ralph loop. Returns the iteration it was at, or error.
func (l *Loop) Cancel() (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	state, err := l.readState()
	if err != nil {
		return 0, fmt.Errorf("no active ralph loop found")
	}
	if state == nil || !state.Active {
		return 0, fmt.Errorf("no active ralph loop found")
	}

	iteration := state.Iteration
	l.cleanup()

	slog.Info("Ralph loop cancelled", "iteration", iteration)
	return iteration, nil
}

// IsActive returns true if a ralph loop is currently running.
func (l *Loop) IsActive() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	state, err := l.readState()
	if err != nil {
		return false
	}
	return state != nil && state.Active
}

// Status returns the current loop state, or nil if inactive.
func (l *Loop) Status() *State {
	l.mu.Lock()
	defer l.mu.Unlock()

	state, _ := l.readState()
	if state == nil || !state.Active {
		return nil
	}
	return state
}

func (l *Loop) readState() (*State, error) {
	data, err := os.ReadFile(l.statePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state State
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("corrupt ralph state: %w", err)
	}
	return &state, nil
}

func (l *Loop) writeState(state *State) error {
	if err := os.MkdirAll(l.stateDir, 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}

	return os.WriteFile(l.statePath(), data, 0o644)
}

func (l *Loop) cleanup() {
	os.Remove(l.statePath())
}

var promiseRegex = regexp.MustCompile(`<promise>(.*?)</promise>`)

func extractPromise(text string) string {
	match := promiseRegex.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

// ParseArgs parses ralph loop command arguments.
// Accepts: PROMPT [--max-iterations N] [--completion-promise TEXT]
func ParseArgs(args string) (prompt string, maxIterations int, completionPromise string, err error) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return "", 0, "", fmt.Errorf("no prompt provided")
	}

	var promptParts []string
	maxIterations = DefaultMaxIterations

	for i := 0; i < len(parts); i++ {
		switch parts[i] {
		case "--max-iterations":
			if i+1 >= len(parts) {
				return "", 0, "", fmt.Errorf("--max-iterations requires a number")
			}
			i++
			n, parseErr := strconv.Atoi(parts[i])
			if parseErr != nil {
				return "", 0, "", fmt.Errorf("--max-iterations must be a number, got: %s", parts[i])
			}
			maxIterations = n
		case "--completion-promise":
			if i+1 >= len(parts) {
				return "", 0, "", fmt.Errorf("--completion-promise requires text")
			}
			i++
			// Collect until next flag or end
			var promiseParts []string
			for i < len(parts) && !strings.HasPrefix(parts[i], "--") {
				promiseParts = append(promiseParts, parts[i])
				i++
			}
			i-- // back up since the for loop will increment
			completionPromise = strings.Join(promiseParts, " ")
		default:
			promptParts = append(promptParts, parts[i])
		}
	}

	prompt = strings.Join(promptParts, " ")
	if prompt == "" {
		return "", 0, "", fmt.Errorf("no prompt provided")
	}

	return prompt, maxIterations, completionPromise, nil
}
