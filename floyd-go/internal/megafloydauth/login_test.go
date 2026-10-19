package megafloydauth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// scriptedFlow is a DeviceFlow driven by fixed responses.
type scriptedFlow struct {
	mu        sync.Mutex
	auths     []DeviceAuthorization
	polls     []PollResult
	authCalls int
	pollCalls int
	pollCodes []string
	pollErr   error
}

func (f *scriptedFlow) RequestDeviceAuthorization(context.Context) (DeviceAuthorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.authCalls >= len(f.auths) {
		return DeviceAuthorization{}, errors.New("scriptedFlow: no authorization left")
	}
	auth := f.auths[f.authCalls]
	f.authCalls++
	return auth, nil
}

func (f *scriptedFlow) PollDeviceToken(_ context.Context, deviceCode string) (PollResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pollCodes = append(f.pollCodes, deviceCode)
	if f.pollErr != nil {
		return PollResult{}, f.pollErr
	}
	if f.pollCalls >= len(f.polls) {
		return PollResult{Status: PollPending, ErrorCode: "authorization_pending"}, nil
	}
	result := f.polls[f.pollCalls]
	f.pollCalls++
	return result, nil
}

func (f *scriptedFlow) RefreshAccessToken(context.Context, string) (Token, error) {
	return Token{}, errors.New("scriptedFlow: refresh not scripted")
}

func (f *scriptedFlow) counts() (auth, poll int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authCalls, f.pollCalls
}

func (f *scriptedFlow) codes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.pollCodes...)
}

// fakeClock is a clock that tests advance explicitly.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(testNowUnix, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestLoginPendingThenSuccess(t *testing.T) {
	t.Parallel()

	flow := &scriptedFlow{
		auths: []DeviceAuthorization{{
			UserCode:                "ABCD",
			DeviceCode:              "device-1",
			VerificationURIComplete: "https://example.test/approve",
			Interval:                2 * time.Second,
		}},
		polls: []PollResult{
			{Status: PollPending, ErrorCode: "authorization_pending"},
			{Status: PollSuccess, Token: Token{AccessToken: "access", RefreshToken: "refresh"}},
		},
	}
	recorder := &sleepRecorder{}
	var displayed []DeviceAuthorization

	token, err := Login(t.Context(), flow, LoginOptions{
		OnDeviceCode: func(auth DeviceAuthorization) { displayed = append(displayed, auth) },
		Sleep:        recorder.record,
	})
	require.NoError(t, err)
	require.Equal(t, "access", token.AccessToken)
	require.Equal(t, []time.Duration{2 * time.Second}, recorder.recorded())
	require.Len(t, displayed, 1)
	require.Equal(t, "ABCD", displayed[0].UserCode)
}

func TestLoginSlowDownAddsFiveSeconds(t *testing.T) {
	t.Parallel()

	flow := &scriptedFlow{
		auths: []DeviceAuthorization{{
			DeviceCode: "device-1",
			Interval:   time.Second,
		}},
		polls: []PollResult{
			{Status: PollPending, ErrorCode: "authorization_pending"},
			{Status: PollPending, ErrorCode: "slow_down"},
			{Status: PollSuccess, Token: Token{AccessToken: "access"}},
		},
	}
	recorder := &sleepRecorder{}

	_, err := Login(t.Context(), flow, LoginOptions{Sleep: recorder.record})
	require.NoError(t, err)
	require.Equal(t, []time.Duration{time.Second, 6 * time.Second}, recorder.recorded())
}

func TestLoginExpiredTokenRequestsNewCode(t *testing.T) {
	t.Parallel()

	flow := &scriptedFlow{
		auths: []DeviceAuthorization{
			{DeviceCode: "device-1", Interval: time.Second},
			{DeviceCode: "device-2", Interval: time.Second},
		},
		polls: []PollResult{
			{Status: PollExpired, ErrorCode: "expired_token"},
			{Status: PollSuccess, Token: Token{AccessToken: "access"}},
		},
	}
	recorder := &sleepRecorder{}
	var shown int

	token, err := Login(t.Context(), flow, LoginOptions{
		OnDeviceCode: func(DeviceAuthorization) { shown++ },
		Sleep:        recorder.record,
	})
	require.NoError(t, err)
	require.Equal(t, "access", token.AccessToken)
	require.Equal(t, 2, shown)
	require.Equal(t, []string{"device-1", "device-2"}, flow.codes())
}

func TestLoginTimesOut(t *testing.T) {
	t.Parallel()

	flow := &scriptedFlow{
		auths: []DeviceAuthorization{{DeviceCode: "device-1", Interval: time.Second}},
		polls: []PollResult{
			{Status: PollPending, ErrorCode: "authorization_pending"},
			{Status: PollPending, ErrorCode: "authorization_pending"},
			{Status: PollPending, ErrorCode: "authorization_pending"},
		},
	}
	clock := newFakeClock()
	sleep := func(_ context.Context, _ time.Duration) error {
		clock.Advance(2 * time.Second)
		return nil
	}

	_, err := Login(t.Context(), flow, LoginOptions{
		Timeout: 3 * time.Second,
		Now:     clock.Now,
		Sleep:   sleep,
	})
	require.ErrorIs(t, err, ErrLoginTimeout)
}

func TestLoginAccessDenied(t *testing.T) {
	t.Parallel()

	flow := &scriptedFlow{
		auths: []DeviceAuthorization{{DeviceCode: "device-1", Interval: time.Second}},
		polls: []PollResult{
			{Status: PollDenied, ErrorCode: "access_denied", Description: "user refused"},
		},
	}

	_, err := Login(t.Context(), flow, LoginOptions{Sleep: (&sleepRecorder{}).sleep})
	require.ErrorIs(t, err, ErrAccessDenied)
	require.Contains(t, err.Error(), "user refused")
}

func TestLoginUnexpectedPollStatus(t *testing.T) {
	t.Parallel()

	flow := &scriptedFlow{
		auths: []DeviceAuthorization{{DeviceCode: "device-1", Interval: time.Second}},
		polls: []PollResult{{Status: PollStatus(99)}},
	}

	_, err := Login(t.Context(), flow, LoginOptions{Sleep: (&sleepRecorder{}).sleep})
	require.ErrorIs(t, err, ErrInvalidResponse)
}

func TestLoginPropagatesPollError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("poll exploded")
	flow := &scriptedFlow{
		auths:   []DeviceAuthorization{{DeviceCode: "device-1", Interval: time.Second}},
		pollErr: wantErr,
	}

	_, err := Login(t.Context(), flow, LoginOptions{Sleep: (&sleepRecorder{}).sleep})
	require.ErrorIs(t, err, wantErr)
}

func TestLoginHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	flow := &scriptedFlow{
		auths: []DeviceAuthorization{{DeviceCode: "device-1", Interval: time.Second}},
	}

	_, err := Login(ctx, flow, LoginOptions{Sleep: (&sleepRecorder{}).sleep})
	require.ErrorIs(t, err, context.Canceled)
	_, polls := flow.counts()
	require.Zero(t, polls)
}

func TestLoginRequiresFlow(t *testing.T) {
	t.Parallel()

	_, err := Login(t.Context(), nil, LoginOptions{})
	require.ErrorIs(t, err, ErrNotConfigured)
}
