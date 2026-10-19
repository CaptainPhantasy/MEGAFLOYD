package megafloydauth

import (
	"context"
	"fmt"
	"time"
)

// LoginOptions controls one device-code login attempt. Zero values select
// the donor defaults: a 15-minute budget, the wall clock, and a
// context-aware sleep.
type LoginOptions struct {
	// OnDeviceCode is called once per device authorization, including a
	// second authorization after expired_token.
	OnDeviceCode func(DeviceAuthorization)
	// Timeout bounds the whole attempt.
	Timeout time.Duration
	// Now overrides the clock (tests).
	Now func() time.Time
	// Sleep overrides the sleeper (tests).
	Sleep func(context.Context, time.Duration) error
}

// Login drives a full device-code login: request a device code, poll the
// token endpoint with interval and slow_down handling, and return the issued
// token. Persistence is the caller's job, so a test flow can be used
// without touching disk.
//
// Policy mirrors the donor client: authorization_pending and slow_down keep
// polling, slow_down adds five seconds to the interval for the rest of the
// attempt, access_denied ends the login, and expired_token requests a fresh
// device code until the local deadline.
func Login(ctx context.Context, flow DeviceFlow, opts LoginOptions) (Token, error) {
	if flow == nil {
		return Token{}, fmt.Errorf("%w: device flow is nil", ErrNotConfigured)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DeviceCodeTimeout
	}
	deadline := now().Add(timeout)

	for {
		auth, err := flow.RequestDeviceAuthorization(ctx)
		if err != nil {
			return Token{}, err
		}
		if opts.OnDeviceCode != nil {
			opts.OnDeviceCode(auth)
		}

		interval := auth.Interval
		if interval < time.Second {
			// RFC 8628 §3.5 and the donor both floor the interval at 1s.
			interval = time.Second
		}
		deviceExpired := false
		for {
			if err := ctx.Err(); err != nil {
				return Token{}, err
			}
			if !now().Before(deadline) {
				return Token{}, fmt.Errorf("%w after %s", ErrLoginTimeout, timeout)
			}

			result, err := flow.PollDeviceToken(ctx, auth.DeviceCode)
			if err != nil {
				return Token{}, err
			}
			switch result.Status {
			case PollSuccess:
				return result.Token, nil
			case PollDenied:
				if result.Description != "" {
					return Token{}, fmt.Errorf("%w: %s", ErrAccessDenied, result.Description)
				}
				return Token{}, ErrAccessDenied
			case PollExpired:
				deviceExpired = true
			case PollPending:
				if result.ErrorCode == "slow_down" {
					interval += 5 * time.Second
				}
			default:
				return Token{}, fmt.Errorf(
					"%w: unexpected poll status %d", ErrInvalidResponse, result.Status,
				)
			}
			if deviceExpired {
				break
			}
			if err := sleep(ctx, interval); err != nil {
				return Token{}, err
			}
		}

		if !now().Before(deadline) {
			return Token{}, fmt.Errorf("%w after %s", ErrLoginTimeout, timeout)
		}
	}
}
