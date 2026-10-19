package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
)

// The reference harness defaults to 10 attempts per step. The Go harness
// bounds the whole run instead: every retry in any step draws from the same
// budget, so a run cannot spin forever by resetting a per-step counter.
const (
	defaultRunMaxAttempts       = 10
	defaultRunCircuitBreakerHit = 2
)

// runRetryBudget bounds provider retries for one agent run and trips a
// circuit breaker when the same provider failure repeats. Record is called
// from fantasy's OnRetry callback for each retry it is about to schedule.
// Once tripped, the handler cancels the run so no further model attempt
// starts, and Run returns the recorded error as explicit evidence.
type runRetryBudget struct {
	maxAttempts int
	circuitHits int

	mu       sync.Mutex
	retries  int
	hashHits map[string]int
	tripErr  error
}

func newRunRetryBudget(maxAttempts, circuitHits int) *runRetryBudget {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if circuitHits < 1 {
		circuitHits = 1
	}
	return &runRetryBudget{
		maxAttempts: maxAttempts,
		circuitHits: circuitHits,
		hashHits:    make(map[string]int),
	}
}

// maxRetries returns the value to install on fantasy's AgentStreamCall. The
// budget callback trips on the retry that would exceed maxAttempts, so the
// provider setting must allow that callback to fire rather than silently
// returning a bare RetryError first.
func (b *runRetryBudget) maxRetries() int {
	if b == nil {
		return 0
	}
	return b.maxAttempts
}

// Record observes one failed provider attempt. It returns a non-nil error
// when the run must stop; the caller cancels the run and surfaces that error.
func (b *runRetryBudget) Record(providerErr *fantasy.ProviderError) error {
	if b == nil {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tripErr != nil {
		return b.tripErr
	}

	b.retries++
	failure := providerFailureSummary(providerErr)
	hash := runFailureHash(failure)
	b.hashHits[hash]++
	if b.hashHits[hash] >= b.circuitHits {
		b.tripErr = fmt.Errorf(
			"run circuit breaker tripped: failure %s repeated %d times within the retry budget; stopping instead of looping. Last failure: %s (evidence: circuit_breaker failure=%s hits=%d attempts=%d)",
			hash, b.hashHits[hash], failure, hash, b.hashHits[hash], b.retries,
		)
		return b.tripErr
	}

	if b.retries >= b.maxAttempts {
		b.tripErr = fmt.Errorf(
			"run retry budget exhausted after %d attempts (limit %d); stopping instead of looping. Last failure: %s (evidence: retry_budget attempts=%d max_attempts=%d failure=%s)",
			b.retries, b.maxAttempts, failure, b.retries, b.maxAttempts, hash,
		)
		return b.tripErr
	}
	return nil
}

// Tripped reports whether the run must stop.
func (b *runRetryBudget) Tripped() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tripErr != nil
}

// Err returns the evidence error recorded when the budget tripped.
func (b *runRetryBudget) Err() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tripErr
}

// runRetryHandler wraps an existing OnRetry callback. When the budget trips
// it cancels the run and skips next, so the stream stops before scheduling
// another model attempt and the partial-retry bookkeeping does not run.
func runRetryHandler(
	budget *runRetryBudget,
	cancel context.CancelFunc,
	next fantasy.OnRetryCallback,
) fantasy.OnRetryCallback {
	return func(err *fantasy.ProviderError, delay time.Duration) {
		if budget != nil {
			if tripErr := budget.Record(err); tripErr != nil {
				slog.Error("Run retry budget tripped", "error", tripErr)
				if cancel != nil {
					cancel()
				}
				return
			}
		}
		if next != nil {
			next(err, delay)
		}
	}
}

func providerFailureSummary(err *fantasy.ProviderError) string {
	if err == nil {
		return "provider error"
	}
	parts := make([]string, 0, 3)
	if err.Title != "" {
		parts = append(parts, err.Title)
	}
	if err.Message != "" {
		parts = append(parts, err.Message)
	}
	if err.StatusCode != 0 {
		parts = append(parts, fmt.Sprintf("status=%d", err.StatusCode))
	}
	if len(parts) == 0 {
		return "provider error"
	}
	return strings.Join(parts, ": ")
}

func runFailureHash(failure string) string {
	sum := sha256.Sum256([]byte(failure))
	return hex.EncodeToString(sum[:])[:12]
}

// retryBudgetError replaces a stream error caused by a tripped budget. A
// fantasy RetryError without a trip still means the provider's retry pass
// ended; wrap it with the same evidence vocabulary so callers never see a
// silent loop stop.
func retryBudgetError(err error, budget *runRetryBudget) error {
	if err == nil {
		return nil
	}
	if budget.Tripped() {
		return budget.Err()
	}
	var retryErr *fantasy.RetryError
	if errors.As(err, &retryErr) {
		return fmt.Errorf(
			"run retry budget ended: provider retries exhausted without a successful attempt; stopping instead of looping. Last error: %s (evidence: retry_budget attempts=unknown exhausted=true)",
			err,
		)
	}
	return err
}
