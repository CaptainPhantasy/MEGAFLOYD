package agent

import (
	"fmt"
)

// TokenBudget provides configurable token budget tracking with
// threshold-based warnings and hard caps. It hooks into the
// agent's Stream lifecycle to enforce spending limits.
type TokenBudget struct {
	// MaxTokensUSD is the hard cap in USD. When cumulative cost
	// exceeds this, the agent is stopped. Zero means unlimited.
	MaxTokensUSD float64 `json:"max_tokens_usd"`
	// WarnAtPercent is the percentage (0-100) at which a warning
	// system message is injected. Default: 80.
	WarnAtPercent float64 `json:"warn_at_percent"`
	// spentUSD is the accumulated cost so far.
	spentUSD float64
	// warned indicates whether the warning has already been emitted.
	warned bool
}

// BudgetConfig is the configuration surface exposed via config.go.
type BudgetConfig struct {
	// MaxUSD is the maximum spend in USD for a single session.
	MaxUSD float64 `json:"max_usd,omitempty" jsonschema:"description=Maximum token spend in USD per session,example=5.0,example=10.0"`
	// WarnPercent is the percentage of the budget at which a warning
	// is injected into the conversation. Default: 80.
	WarnPercent float64 `json:"warn_percent,omitempty" jsonschema:"description=Percentage of budget at which a warning is injected,example=80,default=80"`
}

// NewTokenBudget creates a TokenBudget from config.
func NewTokenBudget(cfg *BudgetConfig) *TokenBudget {
	if cfg == nil || cfg.MaxUSD <= 0 {
		return nil // budget disabled
	}
	warnPct := cfg.WarnPercent
	if warnPct <= 0 || warnPct > 100 {
		warnPct = 80
	}
	return &TokenBudget{
		MaxTokensUSD: cfg.MaxUSD,
		WarnAtPercent: warnPct,
	}
}

// Check returns whether the agent should continue, and a warning
// message if the threshold has been crossed. Call before each
// agent.Stream() invocation.
func (b *TokenBudget) Check() (proceed bool, warning string) {
	if b == nil {
		return true, ""
	}

	// Hard cap check
	if b.spentUSD >= b.MaxTokensUSD {
		return false, fmt.Sprintf(
			"Token budget exceeded: $%.4f / $%.4f (100%%). Session stopped.",
			b.spentUSD, b.MaxTokensUSD,
		)
	}

	// Warning threshold check
	threshold := b.MaxTokensUSD * (b.WarnAtPercent / 100)
	if !b.warned && b.spentUSD >= threshold {
		b.warned = true
		remaining := b.MaxTokensUSD - b.spentUSD
		pct := b.spentUSD / b.MaxTokensUSD * 100
		if pct > 100 {
			pct = 100
		}
		return true, fmt.Sprintf(
			"⚠ Token budget warning: $%.4f / $%.4f (%.0f%%). $%.4f remaining.",
			b.spentUSD, b.MaxTokensUSD,
			pct,
			remaining,
		)
	}

	return true, ""
}

// Record adds a cost observation to the budget.
func (b *TokenBudget) Record(costUSD float64) {
	if b == nil {
		return
	}
	b.spentUSD += costUSD
}

// Spent returns the cumulative spend in USD.
func (b *TokenBudget) Spent() float64 {
	if b == nil {
		return 0
	}
	return b.spentUSD
}
