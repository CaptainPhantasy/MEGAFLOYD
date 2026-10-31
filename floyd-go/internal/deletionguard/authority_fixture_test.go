package deletionguard

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Fixture authority lives only in test binaries; production has no local fallback.
type fixtureAuthority struct {
	mu        sync.Mutex
	data      map[string][]byte
	revisions map[string]uint64
}

func (a *fixtureAuthority) Read(_ context.Context, namespace, session string) ([]byte, uint64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := namespace + ":" + session
	return append([]byte(nil), a.data[key]...), a.revisions[key], nil
}

func (a *fixtureAuthority) CompareAndSwap(_ context.Context, namespace, session string, revision uint64, data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := namespace + ":" + session
	if a.revisions[key] != revision {
		return errors.New("protected approval revision conflict")
	}
	a.data[key] = append([]byte(nil), data...)
	a.revisions[key]++
	return nil
}

func (a *fixtureAuthority) corrupt(namespace, session string, data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := namespace + ":" + session
	a.data[key] = append([]byte(nil), data...)
	a.revisions[key]++
}

var fixtureAuthorities sync.Map

func fixtureBackend(dataDir string) *fixtureAuthority {
	a, _ := fixtureAuthorities.LoadOrStore(dataDir, &fixtureAuthority{data: map[string][]byte{}, revisions: map[string]uint64{}})
	return a.(*fixtureAuthority)
}

func testLedger(dataDir string) *Ledger {
	return newLedgerWithAuthority(dataDir, fixtureBackend(dataDir))
}

func testGuard(dataDir string, confirmer Confirmer) *Guard {
	return &Guard{ledger: testLedger(dataDir), confirmer: confirmer, now: time.Now}
}
