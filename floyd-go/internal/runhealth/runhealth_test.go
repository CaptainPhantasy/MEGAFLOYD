package runhealth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPersistentTaskCircuitAndRecovery(t *testing.T) {
	t.Setenv("FLOYD_RUNTIME_PROFILE", "megafloyd")
	t.Setenv("SUPERFLOYD_DEGRADATION_CONTROLS", "1")
	dir := t.TempDir()
	ctx := context.Background()
	task := TaskKey("session", "test the canonical build")
	failure := errors.New("provider unavailable")
	for range CircuitHits {
		finish, err := Begin(ctx, dir, task)
		require.NoError(t, err)
		require.ErrorIs(t, finish(failure), failure)
	}
	_, err := Begin(ctx, dir, task)
	require.ErrorContains(t, err, "circuit breaker engaged")
	state, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, state.Failures, 2)

	otherTask := TaskKey("session", "inspect provider configuration")
	finish, err := Begin(ctx, dir, otherTask)
	require.NoError(t, err)
	require.NoError(t, finish(nil))
	_, err = Begin(ctx, dir, task)
	require.ErrorContains(t, err, "circuit breaker engaged")

	for i := range state.Failures {
		state.Failures[i].Occurred = time.Now().Add(-CircuitWindow - time.Second).Unix()
	}
	require.NoError(t, save(dir, state))
	finish, err = Begin(ctx, dir, task)
	require.NoError(t, err)
	require.NoError(t, finish(nil))
	state, err = Load(dir)
	require.NoError(t, err)
	require.Empty(t, state.Failures)
}

func TestPersistentRetryBudgetAndCancellation(t *testing.T) {
	t.Setenv("FLOYD_RUNTIME_PROFILE", "megafloyd")
	t.Setenv("SUPERFLOYD_DEGRADATION_CONTROLS", "1")
	dir := t.TempDir()
	ctx := context.Background()
	finish, err := Begin(ctx, dir, TaskKey("session", "canceled turn"))
	require.NoError(t, err)
	require.ErrorIs(t, finish(context.Canceled), context.Canceled)
	state, err := Load(dir)
	require.NoError(t, err)
	require.Empty(t, state.Failures)

	var wg sync.WaitGroup
	errs := make(chan error, RetryBudgetFailures)
	for i := range RetryBudgetFailures {
		finish, err := Begin(ctx, dir, TaskKey("session", strings.Repeat("x", i+1)))
		require.NoError(t, err)
		wg.Go(func() { errs <- finish(errors.New("one bounded failure")) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.ErrorContains(t, err, "one bounded failure")
	}
	state, err = Load(dir)
	require.NoError(t, err)
	require.Len(t, state.Failures, RetryBudgetFailures)
	_, err = Begin(ctx, dir, TaskKey("session", "next run"))
	require.ErrorContains(t, err, "retry budget exceeded")

	for i := range state.Failures {
		state.Failures[i].Occurred = time.Now().Add(-RetryBudgetWindow - time.Second).Unix()
	}
	require.NoError(t, save(dir, state))
	finish, err = Begin(ctx, dir, TaskKey("session", "recovered run"))
	require.NoError(t, err)
	require.NoError(t, finish(nil))
}

func TestHealthReadErrorIsPreserved(t *testing.T) {
	t.Setenv("FLOYD_RUNTIME_PROFILE", "megafloyd")
	t.Setenv("SUPERFLOYD_DEGRADATION_CONTROLS", "1")
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime_health.json")
	require.NoError(t, os.WriteFile(path, []byte("invalid existing state"), 0o600))
	_, err := Begin(context.Background(), dir, "task")
	require.ErrorContains(t, err, "decode run health")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "invalid existing state", string(data))

	t.Setenv("FLOYD_RUNTIME_PROFILE", "floyd")
	finish, err := Begin(context.Background(), dir, "task")
	require.NoError(t, err)
	require.NoError(t, finish(nil))
}

func TestMegafloydProfileAndExplicitOverride(t *testing.T) {
	require.True(t, IsResilientProfile("megafloyd"))
	require.True(t, IsResilientProfile("megafloyd.test"))
	require.False(t, IsResilientProfile("floyd"))
	t.Setenv("FLOYD_RUNTIME_PROFILE", "megafloyd")
	t.Setenv("SUPERFLOYD_DEGRADATION_CONTROLS", "1")
	require.True(t, Enabled())
	t.Setenv("SUPERFLOYD_DEGRADATION_CONTROLS", "off")
	require.False(t, Enabled())
	t.Setenv("FLOYD_RUNTIME_PROFILE", "floyd")
	t.Setenv("SUPERFLOYD_DEGRADATION_CONTROLS", "1")
	require.False(t, Enabled())
}
