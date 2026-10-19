package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func providerError(status int, message string) *fantasy.ProviderError {
	return &fantasy.ProviderError{StatusCode: status, Message: message}
}

func TestRunRetryBudgetAllowsAttemptsWithinLimit(t *testing.T) {
	t.Parallel()

	budget := newRunRetryBudget(3, 99)
	require.Equal(t, 3, budget.maxRetries())

	require.NoError(t, budget.Record(providerError(500, "first")))
	require.NoError(t, budget.Record(providerError(502, "second")))
	require.False(t, budget.Tripped())
}

func TestRunRetryBudgetExhaustion(t *testing.T) {
	t.Parallel()

	budget := newRunRetryBudget(3, 99)

	require.NoError(t, budget.Record(providerError(500, "first")))
	require.NoError(t, budget.Record(providerError(502, "second")))
	err := budget.Record(providerError(503, "third"))
	require.Error(t, err)

	require.True(t, budget.Tripped())
	require.Equal(t, err, budget.Err())
	require.Contains(t, err.Error(), "run retry budget exhausted after 3 attempts (limit 3)")
	require.Contains(t, err.Error(), "stopping instead of looping")
	require.Contains(t, err.Error(), "evidence: retry_budget attempts=3 max_attempts=3 failure=")

	// Once tripped the same evidence error is returned without changing the
	// count, so a late retry callback cannot extend the run.
	require.Equal(t, err, budget.Record(providerError(500, "fourth")))
	require.Contains(t, budget.Err().Error(), "attempts=3")
}

func TestRunRetryBudgetCircuitBreakerTripsOnRepeatedFailure(t *testing.T) {
	t.Parallel()

	budget := newRunRetryBudget(10, 2)
	require.NoError(t, budget.Record(providerError(503, "unavailable")))
	err := budget.Record(providerError(503, "unavailable"))
	require.Error(t, err)

	require.True(t, budget.Tripped())
	require.Contains(t, err.Error(), "run circuit breaker tripped")
	require.Contains(t, err.Error(), "repeated 2 times")
	require.Contains(t, err.Error(), "evidence: circuit_breaker")
	require.Contains(t, err.Error(), "hits=2 attempts=2")
}

func TestRunRetryBudgetCircuitBreakerIgnoresDifferentFailures(t *testing.T) {
	t.Parallel()

	budget := newRunRetryBudget(10, 2)
	require.NoError(t, budget.Record(providerError(500, "first")))
	require.NoError(t, budget.Record(providerError(502, "second")))
	require.NoError(t, budget.Record(providerError(503, "third")))
	require.False(t, budget.Tripped())
}

func TestRunRetryHandlerCancelsAndSkipsNextWhenTripped(t *testing.T) {
	t.Parallel()

	budget := newRunRetryBudget(3, 99)
	cancelled := false
	cancel := func() { cancelled = true }
	nextCalls := 0
	handler := runRetryHandler(budget, cancel, func(*fantasy.ProviderError, time.Duration) {
		nextCalls++
	})

	handler(providerError(500, "first"), time.Millisecond)
	handler(providerError(502, "second"), time.Millisecond)
	require.False(t, cancelled)
	require.Equal(t, 2, nextCalls)

	handler(providerError(503, "third"), time.Millisecond)
	require.True(t, cancelled, "budget trip must cancel the run")
	require.Equal(t, 2, nextCalls, "next callback must not run after the trip")
	require.True(t, budget.Tripped())
}

func TestRunRetryHandlerWithoutBudgetDelegates(t *testing.T) {
	t.Parallel()

	nextCalls := 0
	handler := runRetryHandler(nil, nil, func(*fantasy.ProviderError, time.Duration) {
		nextCalls++
	})
	handler(providerError(500, "first"), time.Millisecond)
	require.Equal(t, 1, nextCalls)
}

func TestRetryBudgetErrorUsesTrippedEvidence(t *testing.T) {
	t.Parallel()

	budget := newRunRetryBudget(1, 99)
	require.Error(t, budget.Record(providerError(500, "boom")))

	err := retryBudgetError(context.Canceled, budget)
	require.Equal(t, budget.Err(), err)
	require.Contains(t, err.Error(), "evidence: retry_budget")
}

func TestRetryBudgetErrorWrapsExhaustedRetryError(t *testing.T) {
	t.Parallel()

	budget := newRunRetryBudget(10, 99)
	streamErr := &fantasy.RetryError{Errors: []error{fmt.Errorf("provider failed")}}
	err := retryBudgetError(streamErr, budget)
	require.Error(t, err)
	require.Contains(t, err.Error(), "run retry budget ended")
	require.Contains(t, err.Error(), "stopping instead of looping")
	require.Contains(t, err.Error(), "retry error: provider failed")
}

func TestRetryBudgetErrorLeavesOtherErrorsAlone(t *testing.T) {
	t.Parallel()

	budget := newRunRetryBudget(10, 99)
	original := fmt.Errorf("ordinary failure")
	require.Equal(t, original, retryBudgetError(original, budget))
	require.NoError(t, retryBudgetError(nil, budget))
}
