package megafloydauth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// managerFlow is a DeviceFlow with scripted login and refresh responses.
type managerFlow struct {
	scriptedFlow

	mu            sync.Mutex
	refreshCalls  int
	refreshTokens []string
	refreshResult Token
	refreshErr    error
	refreshDelay  time.Duration
}

func (f *managerFlow) RefreshAccessToken(ctx context.Context, refreshToken string) (Token, error) {
	f.mu.Lock()
	f.refreshCalls++
	f.refreshTokens = append(f.refreshTokens, refreshToken)
	result, err, delay := f.refreshResult, f.refreshErr, f.refreshDelay
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return Token{}, ctx.Err()
		}
	}
	if err != nil {
		return Token{}, err
	}
	return result, nil
}

func (f *managerFlow) refreshCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refreshCalls
}

func (f *managerFlow) tokensSeen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.refreshTokens...)
}

func newManagerForTest(
	t *testing.T, flow DeviceFlow, clock func() time.Time,
) (*Manager, FileStore, string) {
	t.Helper()
	store := FileStore{Dir: t.TempDir()}
	const name = "floyd-code-env-test"
	manager := &Manager{Flow: flow, Store: store, Name: name, Now: clock}
	return manager, store, name
}

func fixedClock() time.Time {
	return time.Unix(testNowUnix, 0)
}

func TestManagerLoginSavesToken(t *testing.T) {
	t.Parallel()

	flow := &managerFlow{scriptedFlow: scriptedFlow{
		auths: []DeviceAuthorization{{DeviceCode: "device-1", Interval: time.Second}},
		polls: []PollResult{{
			Status: PollSuccess,
			Token: Token{
				AccessToken:  "access",
				RefreshToken: "refresh",
				ExpiresAt:    testNowUnix + 3600,
				ExpiresIn:    3600,
			},
		}},
	}}
	manager, store, name := newManagerForTest(t, flow, fixedClock)
	manager.Sleep = (&sleepRecorder{}).sleep

	token, err := manager.Login(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, "access", token.AccessToken)

	stored, ok, err := store.Load(name)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, token, stored)
}

func TestManagerTokenFresh(t *testing.T) {
	t.Parallel()

	flow := &managerFlow{}
	manager, store, name := newManagerForTest(t, flow, fixedClock)
	require.NoError(t, store.Save(name, Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    testNowUnix + 3600,
		ExpiresIn:    3600,
	}))

	token, err := manager.Token(t.Context())
	require.NoError(t, err)
	require.Equal(t, "access", token.AccessToken)
	require.Zero(t, flow.refreshCount())
}

func TestManagerTokenRefreshesWhenDue(t *testing.T) {
	t.Parallel()

	flow := &managerFlow{refreshResult: Token{
		AccessToken:  "access-new",
		RefreshToken: "refresh-new",
		ExpiresAt:    testNowUnix + 7200,
		ExpiresIn:    7200,
	}}
	manager, store, name := newManagerForTest(t, flow, fixedClock)
	// A one-hour lifetime refreshes inside the last 30 minutes.
	require.NoError(t, store.Save(name, Token{
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		ExpiresAt:    testNowUnix + 60,
		ExpiresIn:    3600,
	}))

	token, err := manager.Token(t.Context())
	require.NoError(t, err)
	require.Equal(t, "access-new", token.AccessToken)
	require.Equal(t, []string{"refresh-old"}, flow.tokensSeen())

	stored, ok, err := store.Load(name)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "access-new", stored.AccessToken)
	require.Equal(t, int64(testNowUnix+7200), stored.ExpiresAt)
}

func TestManagerTokenSingleFlight(t *testing.T) {
	t.Parallel()

	flow := &managerFlow{
		refreshResult: Token{
			AccessToken:  "access-new",
			RefreshToken: "refresh-new",
			ExpiresAt:    testNowUnix + 7200,
			ExpiresIn:    7200,
		},
		refreshDelay: 10 * time.Millisecond,
	}
	manager, store, name := newManagerForTest(t, flow, fixedClock)
	require.NoError(t, store.Save(name, Token{
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		ExpiresAt:    testNowUnix + 60,
		ExpiresIn:    3600,
	}))

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := manager.Token(t.Context())
			if err != nil {
				errs <- err
				return
			}
			if token.AccessToken != "access-new" {
				errs <- fmt.Errorf("unexpected token %q", token.AccessToken)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 1, flow.refreshCount())
}

func TestManagerTokenUnauthorizedWritesTombstone(t *testing.T) {
	t.Parallel()

	flow := &managerFlow{
		refreshErr: fmt.Errorf("%w: rejected", ErrUnauthorized),
	}
	manager, store, name := newManagerForTest(t, flow, fixedClock)
	require.NoError(t, store.Save(name, Token{
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		ExpiresAt:    testNowUnix + 60,
		ExpiresIn:    3600,
		Scope:        "floyd-code",
		TokenType:    "Bearer",
	}))

	_, err := manager.Token(t.Context())
	require.ErrorIs(t, err, ErrUnauthorized)

	tombstone, ok, err := store.Load(name)
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, tombstone.AccessToken)
	require.Empty(t, tombstone.RefreshToken)
	require.Equal(t, "floyd-code", tombstone.Scope)

	status, err := manager.Status()
	require.NoError(t, err)
	require.True(t, status.Revoked)
	require.False(t, status.LoggedIn)

	_, err = manager.Token(t.Context())
	require.ErrorIs(t, err, ErrLoginRevoked)
	require.Equal(t, 1, flow.refreshCount())
}

func TestManagerTokenMissing(t *testing.T) {
	t.Parallel()

	manager, _, _ := newManagerForTest(t, &managerFlow{}, fixedClock)
	_, err := manager.Token(t.Context())
	require.ErrorIs(t, err, ErrNotLoggedIn)

	status, err := manager.Status()
	require.NoError(t, err)
	require.False(t, status.LoggedIn)
	require.False(t, status.Revoked)
}

func TestManagerTokenRevokedRecord(t *testing.T) {
	t.Parallel()

	manager, store, name := newManagerForTest(t, &managerFlow{}, fixedClock)
	require.NoError(t, store.Save(name, Token{Scope: "floyd-code"}))

	_, err := manager.Token(t.Context())
	require.ErrorIs(t, err, ErrLoginRevoked)
}

func TestManagerTokenWithoutRefreshToken(t *testing.T) {
	t.Parallel()

	manager, store, name := newManagerForTest(t, &managerFlow{}, fixedClock)
	require.NoError(t, store.Save(name, Token{
		AccessToken: "access",
		ExpiresAt:   testNowUnix - 60,
		ExpiresIn:   3600,
	}))

	_, err := manager.Token(t.Context())
	require.ErrorIs(t, err, ErrLoginRevoked)
}

func TestManagerAccessToken(t *testing.T) {
	t.Parallel()

	manager, store, name := newManagerForTest(t, &managerFlow{}, fixedClock)
	require.NoError(t, store.Save(name, Token{
		AccessToken: "access",
		ExpiresAt:   testNowUnix + 3600,
		ExpiresIn:   3600,
	}))

	token, err := manager.AccessToken(t.Context())
	require.NoError(t, err)
	require.Equal(t, "access", token)
}

func TestManagerStatus(t *testing.T) {
	t.Parallel()

	manager, store, name := newManagerForTest(t, &managerFlow{}, fixedClock)
	require.NoError(t, store.Save(name, Token{
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		ExpiresAt:    testNowUnix + 3600,
		ExpiresIn:    3600,
		Scope:        "floyd-code",
		TokenType:    "Bearer",
	}))

	status, err := manager.Status()
	require.NoError(t, err)
	require.True(t, status.LoggedIn)
	require.False(t, status.Revoked)
	require.False(t, status.RefreshDue)
	require.Equal(t, time.Unix(testNowUnix+3600, 0), status.ExpiresAt)
	require.Equal(t, time.Hour, status.Remaining)
	require.Equal(t, "floyd-code", status.Scope)

	rendered := fmt.Sprintf("%+v", status)
	require.NotContains(t, rendered, "access-secret")
	require.NotContains(t, rendered, "refresh-secret")
}

func TestManagerStatusRefreshDue(t *testing.T) {
	t.Parallel()

	manager, store, name := newManagerForTest(t, &managerFlow{}, fixedClock)
	require.NoError(t, store.Save(name, Token{
		AccessToken: "access",
		ExpiresAt:   testNowUnix + 600,
		ExpiresIn:   3600,
	}))

	status, err := manager.Status()
	require.NoError(t, err)
	require.True(t, status.LoggedIn)
	require.True(t, status.RefreshDue)
	require.Equal(t, 10*time.Minute, status.Remaining)
}

func TestManagerStatusUnknownExpiry(t *testing.T) {
	t.Parallel()

	manager, store, name := newManagerForTest(t, &managerFlow{}, fixedClock)
	require.NoError(t, store.Save(name, Token{AccessToken: "access"}))

	status, err := manager.Status()
	require.NoError(t, err)
	require.True(t, status.LoggedIn)
	require.True(t, status.ExpiresAt.IsZero())
	require.Zero(t, status.Remaining)
	require.False(t, status.RefreshDue)
}

func TestManagerLogout(t *testing.T) {
	t.Parallel()

	manager, store, name := newManagerForTest(t, &managerFlow{}, fixedClock)
	require.NoError(t, store.Save(name, Token{AccessToken: "access"}))

	require.NoError(t, manager.Logout())
	_, ok, err := store.Load(name)
	require.NoError(t, err)
	require.False(t, ok)

	// Logging out twice is not an error.
	require.NoError(t, manager.Logout())
}

func TestManagerMisconfigured(t *testing.T) {
	t.Parallel()

	manager := &Manager{}
	_, err := manager.Token(t.Context())
	require.ErrorIs(t, err, ErrNotConfigured)
	_, err = manager.Status()
	require.ErrorIs(t, err, ErrNotConfigured)
	require.ErrorIs(t, manager.Logout(), ErrNotConfigured)
}

func TestManagerTokenPropagatesNonUnauthorizedRefreshError(t *testing.T) {
	t.Parallel()

	networkErr := errors.New("network down")
	flow := &managerFlow{refreshErr: networkErr}
	manager, store, name := newManagerForTest(t, flow, fixedClock)
	require.NoError(t, store.Save(name, Token{
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		ExpiresAt:    testNowUnix + 60,
		ExpiresIn:    3600,
	}))

	_, err := manager.Token(t.Context())
	require.ErrorIs(t, err, networkErr)

	// The original token survives a transient failure.
	stored, ok, err := store.Load(name)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "access-old", stored.AccessToken)
}
