package megafloydauth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Manager owns one token slot and the flow that refreshes it.
//
// It is the entry point for the auth commands. Token and AccessToken are
// also the helper the provider layer can consume later; nothing in the
// chassis calls them yet. Refreshes are serialized inside one process (the
// donor's cross-process lock is not ported).
type Manager struct {
	// Flow performs the network calls.
	Flow DeviceFlow
	// Store persists the token bundle.
	Store Store
	// Name is the storage name (see StorageName).
	Name string
	// Now overrides the clock (tests).
	Now func() time.Time
	// Sleep overrides the sleeper used by Login (tests).
	Sleep func(context.Context, time.Duration) error

	mu sync.Mutex
}

// Status is a secret-free snapshot of the stored credential.
type Status struct {
	// LoggedIn reports a usable access token.
	LoggedIn bool
	// Revoked reports a tombstone: the previous login was rejected.
	Revoked bool
	// ExpiresAt is the token expiry in local time; zero means unknown.
	ExpiresAt time.Time
	// Remaining is the time until ExpiresAt at the manager's clock.
	Remaining time.Duration
	// Scope and TokenType mirror the stored bundle.
	Scope     string
	TokenType string
	// RefreshDue reports that a lazy refresh would run on the next Token
	// call.
	RefreshDue bool
}

type tokenState int

const (
	tokenMissing tokenState = iota
	tokenRevoked
	tokenValid
)

// Login runs the device flow and persists the issued token.
func (m *Manager) Login(
	ctx context.Context, onDeviceCode func(DeviceAuthorization),
) (Token, error) {
	if err := m.validate(); err != nil {
		return Token{}, err
	}
	token, err := Login(ctx, m.Flow, LoginOptions{
		OnDeviceCode: onDeviceCode,
		Now:          m.Now,
		Sleep:        m.Sleep,
	})
	if err != nil {
		return Token{}, err
	}
	if err := m.Store.Save(m.Name, token); err != nil {
		return Token{}, err
	}
	return token, nil
}

// Token returns a usable token, refreshing lazily when the stored token is
// inside its refresh window. An unauthorized refresh writes a tombstone and
// returns ErrUnauthorized.
func (m *Manager) Token(ctx context.Context) (Token, error) {
	if err := m.validate(); err != nil {
		return Token{}, err
	}
	token, state, err := m.load()
	if err != nil {
		return Token{}, err
	}
	switch state {
	case tokenMissing:
		return Token{}, ErrNotLoggedIn
	case tokenRevoked:
		return Token{}, ErrLoginRevoked
	}
	if !token.Expired(m.now()) {
		return token, nil
	}

	// Serialize refreshes in this process. Re-read after the lock so a
	// refresh performed by another goroutine is noticed.
	m.mu.Lock()
	defer m.mu.Unlock()

	token, state, err = m.load()
	if err != nil {
		return Token{}, err
	}
	switch state {
	case tokenMissing:
		return Token{}, ErrNotLoggedIn
	case tokenRevoked:
		return Token{}, ErrLoginRevoked
	}
	if !token.Expired(m.now()) {
		return token, nil
	}
	if token.RefreshToken == "" {
		return Token{}, ErrLoginRevoked
	}

	refreshed, err := m.Flow.RefreshAccessToken(ctx, token.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			tombstone := Token{Scope: token.Scope, TokenType: token.TokenType}
			_ = m.Store.Save(m.Name, tombstone)
		}
		return Token{}, err
	}
	if err := m.Store.Save(m.Name, refreshed); err != nil {
		return Token{}, err
	}
	return refreshed, nil
}

// AccessToken returns a usable access token. It implements TokenSource for
// future provider wiring; nothing consumes it yet.
func (m *Manager) AccessToken(ctx context.Context) (string, error) {
	token, err := m.Token(ctx)
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

// Status reports whether a token is stored and when it expires. It never
// returns token material.
func (m *Manager) Status() (Status, error) {
	if m == nil || m.Store == nil || m.Name == "" {
		return Status{}, fmt.Errorf("%w: manager has no token store", ErrNotConfigured)
	}
	token, state, err := m.load()
	if err != nil {
		return Status{}, err
	}
	status := Status{Scope: token.Scope, TokenType: token.TokenType}
	switch state {
	case tokenMissing:
		return Status{}, nil
	case tokenRevoked:
		status.Revoked = true
		return status, nil
	}

	now := m.now()
	status.LoggedIn = true
	if token.ExpiresAt != 0 {
		status.ExpiresAt = time.Unix(token.ExpiresAt, 0)
		status.Remaining = status.ExpiresAt.Sub(now)
	}
	status.RefreshDue = token.Expired(now)
	return status, nil
}

// Logout removes the stored token. A missing file is not an error.
func (m *Manager) Logout() error {
	if m == nil || m.Store == nil || m.Name == "" {
		return fmt.Errorf("%w: manager has no token store", ErrNotConfigured)
	}
	return m.Store.Remove(m.Name)
}

var _ TokenSource = (*Manager)(nil)

func (m *Manager) validate() error {
	if m == nil {
		return fmt.Errorf("%w: manager is nil", ErrNotConfigured)
	}
	if m.Flow == nil {
		return fmt.Errorf("%w: manager has no device flow", ErrNotConfigured)
	}
	if m.Store == nil {
		return fmt.Errorf("%w: manager has no token store", ErrNotConfigured)
	}
	if m.Name == "" {
		return fmt.Errorf("%w: manager has no token name", ErrNotConfigured)
	}
	return nil
}

func (m *Manager) load() (Token, tokenState, error) {
	token, ok, err := m.Store.Load(m.Name)
	if err != nil {
		return Token{}, tokenMissing, err
	}
	if !ok {
		return Token{}, tokenMissing, nil
	}
	if token.AccessToken == "" {
		return token, tokenRevoked, nil
	}
	return token, tokenValid, nil
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}
