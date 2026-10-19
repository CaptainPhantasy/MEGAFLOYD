package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
)

const (
	// HealthCheckInterval is how often to ping MCP servers
	HealthCheckInterval = 30 * time.Second
	// MaxRestartAttempts is the maximum number of restart attempts before giving up
	MaxRestartAttempts = 5
	// InitialRestartDelay is the initial delay before first restart
	InitialRestartDelay = 1 * time.Second
	// MaxRestartDelay is the maximum delay between restart attempts
	MaxRestartDelay = 60 * time.Second
)

// HealthMonitor manages health checks and auto-restart for MCP servers
type HealthMonitor struct {
	store *config.ConfigStore

	mu sync.RWMutex

	// restart tracks restart state per server
	restart map[string]*restartState

	// stop channel to shutdown the monitor
	stop chan struct{}

	// wg tracks running goroutines
	wg sync.WaitGroup
}

type restartState struct {
	attempts    int
	lastAttempt time.Time
	lastError   error
	backoff     time.Duration
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(store *config.ConfigStore) *HealthMonitor {
	return &HealthMonitor{
		store:   store,
		restart: make(map[string]*restartState),
		stop:    make(chan struct{}),
	}
}

// Start begins the health monitoring loop
func (hm *HealthMonitor) Start(ctx context.Context) {
	hm.wg.Add(1)
	go hm.run(ctx)
}

// Stop shuts down the health monitor
func (hm *HealthMonitor) Stop() {
	close(hm.stop)
	hm.wg.Wait()
}

// run is the main health monitoring loop
func (hm *HealthMonitor) run(ctx context.Context) {
	defer hm.wg.Done()

	ticker := time.NewTicker(HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-hm.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			hm.checkAll(ctx)
		}
	}
}

// checkAll performs health checks on all connected MCP servers
func (hm *HealthMonitor) checkAll(ctx context.Context) {
	states := GetStates()

	for name, info := range states {
		if info.State != StateConnected {
			continue
		}

		// Skip if no client session
		if info.Client == nil {
			continue
		}

		// Get config for timeout
		cfg := hm.store.Config()
		mcpConfig, exists := cfg.MCP[name]
		if !exists {
			continue
		}

		// Ping with timeout
		timeout := mcpTimeout(mcpConfig)
		pingCtx, cancel := context.WithTimeout(ctx, timeout)

		err := info.Client.Ping(pingCtx, nil)
		cancel()

		if err != nil {
			slog.Warn("MCP health check failed", "name", name, "error", err)
			hm.handleFailure(ctx, name, err)
		} else {
			// Reset restart state on successful ping
			hm.resetRestartState(name)
		}
	}
}

// handleFailure handles a failed health check
func (hm *HealthMonitor) handleFailure(ctx context.Context, name string, err error) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	state, exists := hm.restart[name]
	if !exists {
		state = &restartState{
			backoff: InitialRestartDelay,
		}
		hm.restart[name] = state
	}

	// Update state
	state.lastError = err
	state.lastAttempt = time.Now()

	// Check if we've exceeded max attempts
	if state.attempts >= MaxRestartAttempts {
		slog.Error("MCP max restart attempts exceeded",
			"name", name,
			"attempts", state.attempts,
			"error", err)
		updateState(name, StateError,
			fmt.Errorf("max restart attempts (%d) exceeded: %w", MaxRestartAttempts, err),
			nil, Counts{})
		return
	}

	// Check if we're in backoff period
	if time.Since(state.lastAttempt) < state.backoff {
		slog.Debug("MCP in restart backoff",
			"name", name,
			"backoff", state.backoff,
			"time_remaining", state.backoff-time.Since(state.lastAttempt))
		return
	}

	// Attempt restart
	state.attempts++
	slog.Info("Attempting MCP restart",
		"name", name,
		"attempt", state.attempts,
		"backoff", state.backoff)

	// Get current state for counts
	currentState, _ := GetState(name)

	// Update state to starting
	updateState(name, StateStarting, nil, nil, currentState.Counts)

	// Get config
	cfg := hm.store.Config()
	mcpConfig, exists := cfg.MCP[name]
	if !exists {
		updateState(name, StateError, fmt.Errorf("config not found"), nil, currentState.Counts)
		return
	}

	// Tear down the failed session and rebuild it through the chassis
	// connect path so generation guards, tool/prompt registration, and
	// state transitions behave exactly like a config-change restart.
	teardown(name)
	_, err = connectAndRegister(
		ctx,
		hm.store,
		name,
		mcpConfig,
		currentGen(name),
		hm.store.Resolver(),
		channelEnabled(hm.store.Overrides().EnabledChannels, name),
	)
	if err != nil {
		// Increase backoff for next attempt
		state.backoff = min(state.backoff*2, MaxRestartDelay)
		updateState(name, StateError, err, nil, currentState.Counts)
		return
	}

	// Reset backoff on successful restart
	state.backoff = InitialRestartDelay
	state.attempts = 0
	state.lastError = nil

	slog.Info("MCP restart successful", "name", name)
}

// resetRestartState clears the restart state for a server
func (hm *HealthMonitor) resetRestartState(name string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if state, exists := hm.restart[name]; exists {
		state.attempts = 0
		state.backoff = InitialRestartDelay
		state.lastError = nil
	}
}

// GetRestartState returns the current restart state for a server
func (hm *HealthMonitor) GetRestartState(name string) (attempts int, lastError error) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if state, exists := hm.restart[name]; exists {
		return state.attempts, state.lastError
	}
	return 0, nil
}

// Global health monitor instance
var (
	healthMonitor     *HealthMonitor
	healthMonitorOnce sync.Once
)

// GetHealthMonitor returns the singleton health monitor for the given
// config store. The first caller's store wins; later calls with a different
// store keep the original monitor.
func GetHealthMonitor(store *config.ConfigStore) *HealthMonitor {
	healthMonitorOnce.Do(func() {
		healthMonitor = NewHealthMonitor(store)
	})
	return healthMonitor
}
