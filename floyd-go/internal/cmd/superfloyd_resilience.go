package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/paranoia"
	"github.com/legacy-ai/floyd/internal/ui/model"
)

// runtimeProfile distinguishes the plain Floyd binary lane from the
// SuperFloyd resilience lane.
//
// TODO(port-wave4): the donor exposes this through
// config.RuntimeProfile with an options.runtime_profile config surface;
// the chassis config has neither yet, so the profile is derived from the
// binary name and FLOYD_RUNTIME_PROFILE only.
type runtimeProfile string

const (
	runtimeProfileFloyd      runtimeProfile = "floyd"
	runtimeProfileSuperFloyd runtimeProfile = "superfloyd"
)

// normalizeRuntimeProfile maps free-form profile names onto the known
// lanes, defaulting to the plain Floyd lane.
func normalizeRuntimeProfile(v string) runtimeProfile {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "superfloyd", "beast", "balanced", "balance", "safe", "sf":
		return runtimeProfileSuperFloyd
	default:
		return runtimeProfileFloyd
	}
}

type runQualityGateStatus struct {
	Enabled    bool
	Applied    bool
	Reason     string
	Checks     []string
	Violations []string
}

type runtimeFailure struct {
	Hash      string `json:"hash"`
	Message   string `json:"message"`
	Occurred  int64  `json:"occurred"`
	ExitClass string `json:"exit_class"`
}

type runtimeHealthState struct {
	Failures []runtimeFailure `json:"failures"`
}

func qualityGatesEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("SUPERFLOYD_QUALITY_GATES")))
	if v == "" {
		return true
	}
	return v != "0" && v != "false" && v != "off"
}

func degradationControlsEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("SUPERFLOYD_DEGRADATION_CONTROLS")))
	if v == "" {
		return true
	}
	return v != "0" && v != "false" && v != "off"
}

func consistencyLockEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("SUPERFLOYD_CONSISTENCY_LOCK")))
	if v == "" {
		return true
	}
	return v != "0" && v != "false" && v != "off"
}

func autoStabilizeEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("SUPERFLOYD_AUTOSTABILIZE")))
	if v == "" {
		return true
	}
	return v != "0" && v != "false" && v != "off"
}

func isSuperFloydBinary() bool {
	name := strings.ToLower(strings.TrimSpace(filepathBase(os.Args[0])))
	return profileFromBinaryName(name) == runtimeProfileSuperFloyd
}

func profileFromBinaryName(name string) runtimeProfile {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "beast", "balanced", "balance", "safe", "sf":
		return runtimeProfileSuperFloyd
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(name)), "superfloyd") {
		return runtimeProfileSuperFloyd
	}
	return runtimeProfileFloyd
}

func resolveRuntimeProfile(binName string) runtimeProfile {
	if envProfile := strings.TrimSpace(os.Getenv("FLOYD_RUNTIME_PROFILE")); envProfile != "" {
		return normalizeRuntimeProfile(envProfile)
	}

	return profileFromBinaryName(binName)
}

func SetupSuperFloydMode(binName string) {
	name := strings.ToLower(binName)
	if !isSuperFloydBinary() {
		return
	}

	// Runtime Paranoia Check (Poison Pill + Consistency)
	RunSafetyDiagnostics()

	// NOTE: Do NOT redirect FLOYD_GLOBAL_DATA here. Session/DB isolation
	// already comes from --data-dir (each wrapper pins its own). Redirecting
	// shared config resurrected a fossil floyd.json (/Volumes/Storage/.floyd)
	// full of deprecated providers (openrouter/anthropic/zhipu) and stale
	// model pins, shadowing the real config on every superfloyd run.
	// Providers, models, and OAuth tokens MUST stay shared across lanes.

	// Identity Lock: Fingerprint the env to ensure consistency
	lockEnv()

	// Default safety systems ON for SuperFloyd family
	if os.Getenv("SUPERFLOYD_QUALITY_GATES") == "" {
		_ = os.Setenv("SUPERFLOYD_QUALITY_GATES", "1")
	}
	if os.Getenv("SUPERFLOYD_DEGRADATION_CONTROLS") == "" {
		_ = os.Setenv("SUPERFLOYD_DEGRADATION_CONTROLS", "1")
	}
	if os.Getenv("SUPERFLOYD_CONSISTENCY_LOCK") == "" {
		_ = os.Setenv("SUPERFLOYD_CONSISTENCY_LOCK", "1")
	}
	if os.Getenv("SUPERFLOYD_AUTOSTABILIZE") == "" {
		_ = os.Setenv("SUPERFLOYD_AUTOSTABILIZE", "1")
	}

	// Default parallelism if not set
	currParallel := os.Getenv("SUPERFLOYD_MAX_PARALLEL")

	switch {
	case name == "beast":
		if currParallel == "" {
			_ = os.Setenv("SUPERFLOYD_MAX_PARALLEL", "16")
		}
	case name == "balanced" || name == "balance" || name == "sf" || name == "superfloyd":
		if currParallel == "" {
			_ = os.Setenv("SUPERFLOYD_MAX_PARALLEL", "12")
		}
	case name == "safe":
		if currParallel == "" {
			_ = os.Setenv("SUPERFLOYD_MAX_PARALLEL", "6")
		}
	}

	// Context Singularity Summary
	if isSuperFloydBinary() {
		ShowContextSummary()
	}
}

// ShowContextSummary provides a concise overview of the codebase context.
func ShowContextSummary() {
	cwd, _ := os.Getwd()
	fmt.Printf("\n[superfloyd-eye] Context Singularity initialized in %s\n", cwd)

	// Simple heuristic for "density" with safety limits
	files := 0
	maxFiles := 10000
	start := time.Now()

	_ = filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}

		// Optimization: skip common massive directories and hidden ones
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") ||
				name == "node_modules" ||
				name == "vendor" ||
				name == "dist" ||
				name == "bin" ||
				name == "Library" ||
				name == "Applications" {
				return filepath.SkipDir
			}
			return nil
		}

		files++

		// Safety: stop if we hit too many files or take too long (2s)
		if files >= maxFiles || time.Since(start) > 2*time.Second {
			return fmt.Errorf("limit reached")
		}

		return nil
	})

	plus := ""
	if files >= maxFiles {
		plus = "+"
	}

	fmt.Printf("[superfloyd-eye] Target Density: %d%s active components detected\n", files, plus)
	fmt.Printf("[superfloyd-eye] Paranoia State: Zero-Branch Determinism Active\n\n")
}

// lockEnv fingerprints the current environment and sets a consistency lock.
// If the environment drifts significantly while the lock is active, SuperFloyd
// will signal a violation.
func lockEnv() {
	if os.Getenv("SUPERFLOYD_IDENTITY_LOCK") != "" {
		return
	}

	// Create a fingerprint of critical environment variables
	h := crc32.NewIEEE()
	vars := []string{
		"USER",
		"HOME",
		"SHELL",
		"TERM",
		"FLOYD_GLOBAL_DATA",
		"SUPERFLOYD_MAX_PARALLEL",
	}
	for _, v := range vars {
		h.Write([]byte(os.Getenv(v)))
	}

	fingerprint := fmt.Sprintf("%08x", h.Sum32())
	_ = os.Setenv("SUPERFLOYD_IDENTITY_LOCK", fingerprint)
}

func applyRunQualityGates(prompt string) runQualityGateStatus {
	status := runQualityGateStatus{Enabled: qualityGatesEnabled()}
	if !status.Enabled {
		status.Reason = "disabled via SUPERFLOYD_QUALITY_GATES"
		return status
	}

	p := strings.TrimSpace(prompt)
	status.Checks = append(status.Checks,
		"prompt_non_empty",
		"prompt_has_min_signal",
		"prompt_not_oversized_without_autostabilize",
	)

	if p == "" {
		status.Violations = append(status.Violations, "prompt is empty")
	}
	if len([]rune(p)) < 5 {
		status.Violations = append(status.Violations, "prompt too short for meaningful execution")
	}
	if len([]rune(p)) > 200000 {
		status.Violations = append(status.Violations, "prompt exceeds hard safety size")
	}

	status.Applied = true
	if len(status.Violations) > 0 {
		status.Reason = "quality gate violation"
	} else {
		status.Reason = "passed"
	}
	return status
}

func enforceConsistencyLock(cfg *config.ConfigStore) error {
	if !consistencyLockEnabled() || !isSuperFloydBinary() {
		return nil
	}

	if model.AcceptSuggestionPrimaryBinding != "ctrl+y" {
		return fmt.Errorf("consistency lock failed: accept-suggestion binding drifted from ctrl+y")
	}

	if cfg == nil {
		return fmt.Errorf("consistency lock failed: nil config")
	}
	if _, ok := cfg.Config().MCP["floyd-supercache-server"]; !ok {
		if _, fallback := cfg.Config().MCP["floyd-supercache"]; !fallback {
			return fmt.Errorf("consistency lock failed: required MCP config missing (floyd-supercache-server)")
		}
	}

	bootPath := filepath.Join(cfg.WorkingDir(), "FLOYD.md")
	if b, err := os.ReadFile(bootPath); err == nil {
		// Match case-insensitively: valid boot contracts use "Floyd",
		// "FLOYD", or project-specific titles. Only an empty file or
		// one that never mentions Floyd counts as drift.
		if len(strings.TrimSpace(string(b))) == 0 || !strings.Contains(strings.ToUpper(string(b)), "FLOYD") {
			return fmt.Errorf("consistency lock failed: boot contract drift in %s", bootPath)
		}
	}

	return nil
}

func applyAutoStabilizeIfNeeded(ctx context.Context, cfg *config.ConfigStore, prompt string) string {
	if !isSuperFloydBinary() || !autoStabilizeEnabled() || !degradationControlsEnabled() {
		return prompt
	}

	trimmed := strings.TrimSpace(prompt)
	if len(trimmed) == 0 {
		return prompt
	}

	// 1. Check for prompt size degradation
	maxRunes := 50000
	runes := []rune(trimmed)
	if len(runes) > maxRunes {
		fmt.Printf("\n[superfloyd-warn] Prompt degradation detected: size %d exceeds soft limit %d.\n", len(runes), maxRunes)
		if isInteractive() {
			fmt.Print("[superfloyd-negotiate] Accept truncation for stability? (Y/n): ")
			var response string
			fmt.Scanln(&response)
			if strings.ToLower(response) == "n" {
				fmt.Println("[superfloyd-eye] Proceeding with high-load context. Reliability score minimized.")
				return prompt
			}
		}
		return string(runes[:maxRunes]) + "\n\n[superfloyd-auto-stabilize] Prompt was truncated to maintain reliability under high-load context conditions."
	}

	// 2. Check for environmental performance degradation
	if cfg != nil {
		if shouldStabilizeFromBenchmarks(ctx, cfg) {
			fmt.Println("\n[superfloyd-warn] Environmental degradation detected (latency/token pressure).")
			if isInteractive() {
				fmt.Print("[superfloyd-negotiate] Enable strict concise mode to prevent failure? (Y/n): ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) == "n" {
					fmt.Println("[superfloyd-eye] Negative feedback acknowledged. Maintaining full speculative branching.")
					return prompt
				}
			}
			return trimmed + "\n\n[superfloyd-auto-stabilize] Use concise responses, deterministic steps, and minimal speculative branching."
		}
	}

	return prompt
}

func isInteractive() bool {
	// Simple check for terminal
	fi, _ := os.Stdin.Stat()
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func shouldStabilizeFromBenchmarks(ctx context.Context, cfg *config.ConfigStore) bool {
	if cfg == nil {
		return false
	}
	conn, err := db.Connect(ctx, cfg.Config().Options.DataDirectory)
	if err != nil {
		return false
	}
	defer conn.Close()

	stats, err := gatherStats(ctx, conn)
	if err != nil {
		return false
	}
	if stats.Total.TotalSessions < 10 {
		return false
	}
	// Trigger stabilize mode on pressure signals.
	if stats.AvgResponseTimeMs >= 22000 || stats.Total.AvgTokensPerSession >= 90000 {
		return true
	}
	return false
}

func enforceRetryBudget(dataDir string) error {
	if !isSuperFloydBinary() || !degradationControlsEnabled() {
		return nil
	}
	state, _ := loadRuntimeHealth(dataDir)
	now := time.Now().Unix()
	state.Failures = keepRecentFailures(state.Failures, now-3600)
	if len(state.Failures) >= 6 {
		return fmt.Errorf("retry budget exceeded: %d failures in last hour; stabilize before retrying", len(state.Failures))
	}
	return nil
}

func maybeTripCircuitBreaker(dataDir string, runErr error) error {
	if runErr == nil || !isSuperFloydBinary() || !degradationControlsEnabled() {
		return runErr
	}

	state, _ := loadRuntimeHealth(dataDir)
	now := time.Now().Unix()
	state.Failures = keepRecentFailures(state.Failures, now-600)

	msg := strings.TrimSpace(runErr.Error())
	hash := failureHash(msg)
	state.Failures = append(state.Failures, runtimeFailure{
		Hash:      hash,
		Message:   msg,
		Occurred:  now,
		ExitClass: "run",
	})
	_ = saveRuntimeHealth(dataDir, state)

	hits := 0
	for _, f := range state.Failures {
		if f.Hash == hash {
			hits++
		}
	}
	if hits >= 2 {
		return fmt.Errorf("circuit breaker engaged for repeated run failure (%s). gather new observation before retry", hash)
	}
	return runErr
}

func recordRunSuccess(dataDir string) {
	state, _ := loadRuntimeHealth(dataDir)
	state.Failures = nil
	_ = saveRuntimeHealth(dataDir, state)
}

func loadRuntimeHealth(dataDir string) (runtimeHealthState, error) {
	path := filepath.Join(dataDir, "runtime_health.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return runtimeHealthState{}, nil
		}
		return runtimeHealthState{}, err
	}
	var st runtimeHealthState
	if err := json.Unmarshal(b, &st); err != nil {
		return runtimeHealthState{}, nil
	}
	return st, nil
}

func saveRuntimeHealth(dataDir string, st runtimeHealthState) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dataDir, "runtime_health.json")
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func keepRecentFailures(items []runtimeFailure, cutoff int64) []runtimeFailure {
	out := make([]runtimeFailure, 0, len(items))
	for _, it := range items {
		if it.Occurred >= cutoff {
			out = append(out, it)
		}
	}
	return out
}

func failureHash(msg string) string {
	h := crc32.ChecksumIEEE([]byte(strings.ToLower(strings.TrimSpace(msg))))
	return fmt.Sprintf("%08x", h)
}

func filepathBase(path string) string {
	if path == "" {
		return ""
	}
	i := strings.LastIndex(path, "/")
	if i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return path
}

// RunSafetyDiagnostics performs aggressive runtime integrity checks.
func RunSafetyDiagnostics() {
	fmt.Print("[superfloyd-eye] Running Zero-Branch Determinism Diagnostic...")
	if err := paranoia.RuntimeCheck(); err != nil {
		fmt.Printf("\n[superfloyd-FATAL] Runtime integrity compromised: %v\n", err)
		fmt.Println("[superfloyd-FATAL] Determinism failed. Execution blocked for safety.")
		os.Exit(1)
	}
	fmt.Println(" OK")

	fmt.Print("[superfloyd-eye] Running Poison Pill Test (NaN integrity)...")
	if err := paranoia.PoisonPill(); err != nil {
		fmt.Printf("\n[superfloyd-FATAL] Poison Pill failure: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(" OK")
}
