// Package runhealth bounds repeated failed runs across all MEGAFLOYD clients.
package runhealth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/legacy-ai/floyd/internal/lock"
)

const (
	RetryBudgetWindow   = time.Hour
	RetryBudgetFailures = 6
	CircuitWindow       = 10 * time.Minute
	CircuitHits         = 2
)

type Failure struct {
	Hash      string `json:"hash"`
	Message   string `json:"message"`
	Occurred  int64  `json:"occurred"`
	ExitClass string `json:"exit_class"`
	Task      string `json:"task,omitempty"`
}

type State struct {
	Failures []Failure `json:"failures"`
}

// Enabled respects the explicit profile override and the existing toggle.
func Enabled() bool {
	profile := strings.ToLower(strings.TrimSpace(os.Getenv("FLOYD_RUNTIME_PROFILE")))
	if profile == "" {
		profile = filepath.Base(os.Args[0])
	}
	if !IsResilientProfile(profile) {
		return false
	}
	toggle := strings.ToLower(strings.TrimSpace(os.Getenv("SUPERFLOYD_DEGRADATION_CONTROLS")))
	return toggle != "0" && toggle != "false" && toggle != "off"
}

func IsResilientProfile(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "beast", "balanced", "balance", "safe", "sf":
		return true
	}
	return strings.Contains(name, "megafloyd") || strings.Contains(name, "superfloyd")
}

// TaskKey identifies retries without writing the user's prompt to disk.
func TaskKey(sessionID, prompt string) string {
	return digest(sessionID + "\x00" + strings.TrimSpace(prompt))
}

// Begin checks the persisted budget before a model run. The returned function
// records its final outcome exactly once, preserving the original error.
func Begin(ctx context.Context, dataDir, task string) (func(error) error, error) {
	if !Enabled() {
		return func(err error) error { return err }, nil
	}
	if task == "" {
		return nil, errors.New("run health: task key is required")
	}
	err := withState(ctx, dataDir, func(state *State) (bool, error) {
		now := time.Now()
		recent := recentFailures(state.Failures, now.Add(-RetryBudgetWindow).Unix())
		if len(recent) >= RetryBudgetFailures {
			return false, fmt.Errorf("retry budget exceeded: %d failed runs in the last hour; wait for the oldest failure to expire", len(recent))
		}
		counts := map[string]int{}
		for _, failure := range recentFailures(recent, now.Add(-CircuitWindow).Unix()) {
			if failure.Task == task {
				counts[failure.Hash]++
				if counts[failure.Hash] >= CircuitHits {
					return false, fmt.Errorf("circuit breaker engaged for this task (%s); change the task after investigating the failure or wait ten minutes", failure.Hash)
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	var once sync.Once
	var result error
	return func(runErr error) error {
		once.Do(func() {
			result = runErr
			if errors.Is(runErr, context.Canceled) {
				return
			}
			recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			repeated := false
			recordErr := withState(recordCtx, dataDir, func(state *State) (bool, error) {
				now := time.Now()
				state.Failures = recentFailures(state.Failures, now.Add(-RetryBudgetWindow).Unix())
				if runErr == nil {
					kept := state.Failures[:0]
					for _, failure := range state.Failures {
						if failure.Task != task {
							kept = append(kept, failure)
						}
					}
					state.Failures = kept
					return true, nil
				}
				msg := strings.TrimSpace(runErr.Error())
				hash := digest(strings.ToLower(msg))
				state.Failures = append(state.Failures, Failure{
					Hash: hash, Message: msg, Occurred: now.Unix(), ExitClass: "run", Task: task,
				})
				hits := 0
				for _, failure := range recentFailures(state.Failures, now.Add(-CircuitWindow).Unix()) {
					if failure.Task == task && failure.Hash == hash {
						hits++
					}
				}
				repeated = hits >= CircuitHits
				return true, nil
			})
			if recordErr != nil {
				result = errors.Join(runErr, fmt.Errorf("persist run health: %w", recordErr))
			} else if repeated {
				result = fmt.Errorf("circuit breaker engaged after repeated task failure: %w", runErr)
			}
		})
		return result
	}, nil
}

// Load reads the same state consumed by the runtime and CLI health reports.
func Load(dataDir string) (State, error) {
	if strings.TrimSpace(dataDir) == "" {
		return State{}, errors.New("run health: resolved data directory is required")
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "runtime_health.json"))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read run health: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode run health: %w", err)
	}
	return state, nil
}

func withState(ctx context.Context, dataDir string, update func(*State) (bool, error)) error {
	if strings.TrimSpace(dataDir) == "" {
		return errors.New("run health: resolved data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create run health directory: %w", err)
	}
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	release, err := lock.File(lockCtx, filepath.Join(dataDir, "runtime_health.lock"))
	if err != nil {
		return fmt.Errorf("lock run health: %w", err)
	}
	defer release()
	state, err := Load(dataDir)
	if err != nil {
		return err
	}
	changed, err := update(&state)
	if err != nil || !changed {
		return err
	}
	return save(dataDir, state)
}

func save(dataDir string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dataDir, ".runtime-health-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dataDir, "runtime_health.json"))
}

func recentFailures(failures []Failure, cutoff int64) []Failure {
	recent := make([]Failure, 0, len(failures))
	for _, failure := range failures {
		if failure.Occurred >= cutoff {
			recent = append(recent, failure)
		}
	}
	return recent
}

func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}
