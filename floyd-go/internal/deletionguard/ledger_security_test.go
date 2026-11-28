package deletionguard

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/legacy-ai/floyd/internal/approvalauthority"
	"github.com/stretchr/testify/require"
)

func TestProtectedLedgerRejectsForgedWorkspaceHistory(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	c := Classify("rm -f target", dir)
	record := Record{Schema: RecordSchema, SessionID: "forged-session"}
	for stage := 1; stage <= 2; stage++ {
		record.Confirmations = append(record.Confirmations, Confirmation{ID: string(rune('0' + stage)), Stage: stage, TargetSet: c.Digest(), Targets: c.Targets, Source: "human", RecordedAt: "2026-10-30T00:00:00Z"})
	}
	bytes, err := json.Marshal(record)
	require.NoError(t, err)
	guard := NewGuard(data, nil)
	require.IsType(t, &approvalauthority.Client{}, guard.ledger.authority, "default runtime must use the protected broker")
	require.NoError(t, os.MkdirAll(guard.Ledger().Dir("forged-session"), 0o700))
	require.NoError(t, os.WriteFile(guard.Ledger().Path("forged-session"), bytes, 0o600))
	decision := guard.Check(t.Context(), "forged-session", "call", c)
	require.False(t, decision.Allowed, "workspace source=human JSON cannot authorize deletion")
	// A trusted authority with an empty record also ignores the forged local file.
	injected := NewGuardWithAuthority(data, nil, fixtureBackend(data))
	require.False(t, injected.Check(t.Context(), "forged-session", "call", c).Allowed)
	trusted, err := injected.Ledger().Read("forged-session")
	require.NoError(t, err)
	require.Empty(t, trusted.Confirmations)
}

func TestProtectedLedgerRejectsStaleRevisionAndLocalRollback(t *testing.T) {
	guard, dir, all, _ := recoveryFixture(t)
	before, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.NoError(t, guard.BeginExecution("s", all, "one"))
	require.NoError(t, os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "quarantined-a")))
	require.NoError(t, guard.RecordOutcome("s", all, "one", nil, false))
	require.Error(t, guard.Ledger().write("s", before), "an old approval snapshot cannot overwrite later execution records")
	bytes, err := json.Marshal(before)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(guard.Ledger().Dir("s"), 0o700))
	require.NoError(t, os.WriteFile(guard.Ledger().Path("s"), bytes, 0o600))
	restarted := testGuard(guard.ledger.dataDir, nil)
	require.False(t, restarted.Check(t.Context(), "s", "repeat", Classify("rm -f a", dir)).Allowed)
	require.True(t, restarted.Check(t.Context(), "s", "retry", Classify("rm -f b", dir)).Allowed)
}

type unavailableAuthority struct{}

func (unavailableAuthority) Read(context.Context, string, string) ([]byte, uint64, error) {
	return nil, 0, errors.New("service unavailable")
}

func (unavailableAuthority) CompareAndSwap(context.Context, string, string, uint64, []byte) error {
	return errors.New("service unavailable")
}

func TestProtectedAuthorityUnavailableFailsClosedWithoutAffectingHarmlessCalls(t *testing.T) {
	confirmer := &recoveryConfirmer{}
	guard := NewGuardWithAuthority(t.TempDir(), confirmer, unavailableAuthority{})
	denied := guard.Check(t.Context(), "s", "delete", Classify("rm -f target", t.TempDir()))
	require.False(t, denied.Allowed)
	require.Contains(t, denied.Reason, "administrator-owned approval service")
	require.Equal(t, 0, confirmer.calls, "answers cannot be collected into an unprotected fallback")
	require.True(t, guard.Check(t.Context(), "s", "harmless", Classify("printf hello", t.TempDir())).Allowed)
}

// unsupportedTransportAuthority models a platform whose approval service has
// no authenticated transport at all.
type unsupportedTransportAuthority struct{}

func (unsupportedTransportAuthority) Read(context.Context, string, string) ([]byte, uint64, error) {
	return nil, 0, approvalauthority.ErrTransportUnavailable
}

func (unsupportedTransportAuthority) CompareAndSwap(context.Context, string, string, uint64, []byte) error {
	return approvalauthority.ErrTransportUnavailable
}

// TestUnsupportedTransportIsDistinctFromAnUnreachableService is what lets the
// Bash gate step aside on a platform that cannot host the approval service
// while it keeps failing closed where starting the service would help.
func TestUnsupportedTransportIsDistinctFromAnUnreachableService(t *testing.T) {
	confirmer := &recoveryConfirmer{}
	platform := NewGuardWithAuthority(t.TempDir(), confirmer, unsupportedTransportAuthority{})
	require.True(t, platform.UnsupportedTransport())
	_, err := platform.Ledger().Read("s")
	require.ErrorIs(t, err, ErrSessionRecordUnavailable)
	require.ErrorIs(t, err, ErrUnsupportedApproval)
	require.Contains(t, err.Error(), "Move the items to a quarantine directory instead")
	require.NotContains(t, err.Error(), "start the administrator-owned approval service")
	// A direct check has no gate to step aside from, so it still fails closed.
	denied := platform.Check(t.Context(), "s", "delete", Classify("rm -f target", t.TempDir()))
	require.False(t, denied.Allowed)
	require.Equal(t, 0, confirmer.calls)
	require.NotContains(t, denied.Reason, "start the administrator-owned approval service")

	service := NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	require.False(t, service.UnsupportedTransport(), "a service that can be started is not reported")

	working := testGuard(t.TempDir(), nil)
	require.False(t, working.UnsupportedTransport())
	require.Empty(t, fixtureBackend(working.ledger.dataDir).data, "the probe must store nothing")
}

// countingAuthority counts reads so a test can see how often the guard asks.
type countingAuthority struct {
	fixtureAuthority
	reads int
}

func (a *countingAuthority) Read(ctx context.Context, namespace, session string) ([]byte, uint64, error) {
	a.mu.Lock()
	a.reads++
	a.mu.Unlock()
	return a.fixtureAuthority.Read(ctx, namespace, session)
}

// TestUnsupportedTransportIsReadOncePerGuard: whether this platform has an
// approval transport cannot change while the process runs, and the probe is a
// read through the authority, so it is not repeated on every gated call.
func TestUnsupportedTransportIsReadOncePerGuard(t *testing.T) {
	authority := &countingAuthority{fixtureAuthority: fixtureAuthority{data: map[string][]byte{}, revisions: map[string]uint64{}}}
	guard := NewGuardWithAuthority(t.TempDir(), nil, authority)
	for range 3 {
		require.False(t, guard.UnsupportedTransport())
	}
	require.Equal(t, 1, authority.reads)
}

// TestDefaultRuntimeReportsItsRealPlatform checks the guard the runtime builds,
// not a fixture: only Linux has an authenticated transport.
func TestDefaultRuntimeReportsItsRealPlatform(t *testing.T) {
	require.Equal(t, runtime.GOOS != "linux", NewGuard(t.TempDir(), nil).UnsupportedTransport())
}

// This test runs explicitly in the isolated, separately privileged service test.
// Its environment inputs exist only in the test binary, never in the runtime.
func TestProtectedAuthorityProcessIntegration(t *testing.T) {
	socket := os.Getenv("MEGAFLOYD_APPROVAL_TEST_SOCKET")
	if socket == "" {
		t.Skip("requires the isolated root-owned authority integration fixture")
	}
	require.NotEqual(t, 0, os.Getuid(), "the client must run as the ordinary engine account")
	if phase := os.Getenv("MEGAFLOYD_APPROVAL_TEST_PHASE"); phase != "" {
		runProtectedAuthorityPhase(t, socket, phase, os.Getenv("MEGAFLOYD_APPROVAL_TEST_FIXTURE"))
		return
	}
	dir, data := t.TempDir(), t.TempDir()
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	client := approvalauthority.NewClient(socket, 0)
	confirmer := &recoveryConfirmer{}
	guard := NewGuardWithAuthority(data, confirmer, client)
	all := Classify("rm -f a b", dir)
	require.True(t, guard.Check(t.Context(), "protected-process", "one", all).Allowed)
	require.Equal(t, 2, confirmer.calls)
	original, err := guard.Ledger().Read("protected-process")
	require.NoError(t, err)
	require.NoError(t, guard.BeginExecution("protected-process", all, "one"))
	require.NoError(t, os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "quarantined-a")))
	require.NoError(t, guard.RecordOutcome("protected-process", all, "one", nil, false))
	require.Error(t, guard.Ledger().write("protected-process", original), "broker CAS rejects replay of an earlier approval version")
	restarted := NewGuardWithAuthority(data, nil, client)
	require.True(t, restarted.Check(t.Context(), "protected-process", "retry", Classify("rm -f b", dir)).Allowed)
	require.False(t, restarted.Check(t.Context(), "protected-process", "repeat", Classify("rm -f a", dir)).Allowed)
	bytes, err := json.Marshal(original)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(restarted.Ledger().Dir("protected-process"), 0o700))
	require.NoError(t, os.WriteFile(restarted.Ledger().Path("protected-process"), bytes, 0o600))
	require.False(t, restarted.Check(t.Context(), "protected-process", "forged-repeat", Classify("rm -f a", dir)).Allowed)
	history, err := restarted.Ledger().Read("protected-process")
	require.NoError(t, err)
	require.Len(t, history.Confirmations, 2)
	require.Len(t, history.Executions, 1)
}

type protectedProcessFixture struct {
	Schema     string `json:"schema"`
	Workspace  string `json:"workspace"`
	Data       string `json:"data"`
	PreparePID int    `json:"prepare_pid"`
}

// The two phases are executed as separate OS processes using the same pinned
// test executable. The broker may also restart between these invocations.
func runProtectedAuthorityPhase(t *testing.T, socket, phase, fixtureDir string) {
	t.Helper()
	require.True(t, filepath.IsAbs(fixtureDir), "supply an absolute isolated test fixture directory")
	dir, data := filepath.Join(fixtureDir, "workspace"), filepath.Join(fixtureDir, "data")
	manifest := filepath.Join(fixtureDir, "fixture.json")
	client := approvalauthority.NewClient(socket, 0)
	const session = "protected-process-restart"
	switch phase {
	case "prepare":
		require.NoError(t, os.Mkdir(dir, 0o700), "prepare must not overwrite an existing workspace")
		require.NoError(t, os.Mkdir(data, 0o700), "prepare must not overwrite existing runtime data")
		for _, name := range []string{"a", "b", "c"} {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
		}
		confirmer := &recoveryConfirmer{}
		guard := NewGuardWithAuthority(data, confirmer, client)
		all := Classify("rm -f a b", dir)
		decision := guard.Check(t.Context(), session, "first", all)
		require.True(t, decision.Allowed, decision.Reason)
		require.Equal(t, 2, confirmer.calls)
		require.NoError(t, guard.BeginExecution(session, all, "first"))
		require.NoError(t, os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "quarantined-a")))
		require.NoError(t, guard.RecordOutcome(session, all, "first", nil, false))
		history, err := guard.Ledger().Read(session)
		require.NoError(t, err)
		require.Len(t, history.Confirmations, 2)
		require.Len(t, history.Executions, 1)
		bytes, err := json.Marshal(protectedProcessFixture{Schema: "protected-process-fixture/v1", Workspace: dir, Data: data, PreparePID: os.Getpid()})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(manifest, bytes, 0o600))
		t.Logf("prepare client_pid=%d protected_revision=%d confirmations=%d executions=%d", os.Getpid(), history.revision, len(history.Confirmations), len(history.Executions))
	case "retry":
		bytes, err := os.ReadFile(manifest)
		require.NoError(t, err)
		var info protectedProcessFixture
		require.NoError(t, json.Unmarshal(bytes, &info))
		require.Equal(t, "protected-process-fixture/v1", info.Schema)
		require.Equal(t, dir, info.Workspace)
		require.Equal(t, data, info.Data)
		require.Positive(t, info.PreparePID)
		require.NotEqual(t, info.PreparePID, os.Getpid(), "retry must run in a new OS process")
		guard := NewGuardWithAuthority(data, nil, client)
		remaining := Classify("rm -f b", dir)
		decision := guard.Check(t.Context(), session, "retry", remaining)
		require.True(t, decision.Allowed, decision.Reason)
		require.False(t, guard.Check(t.Context(), session, "repeat-a", Classify("rm -f a", dir)).Allowed)
		require.False(t, guard.Check(t.Context(), session, "extra", Classify("rm -f b c", dir)).Allowed)
		// Replacing the remaining pathname must invalidate the old answers.
		require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "saved-original-b")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("substituted"), 0o600))
		require.False(t, guard.Check(t.Context(), session, "substitution", remaining).Allowed)
		// Restore the original unchanged inode; preserve the substitute in quarantine.
		require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-substitute-b")))
		require.NoError(t, os.Rename(filepath.Join(dir, "saved-original-b"), filepath.Join(dir, "b")))
		require.True(t, guard.Check(t.Context(), session, "retry", remaining).Allowed)
		history, err := guard.Ledger().Read(session)
		require.NoError(t, err)
		require.Len(t, history.Confirmations, 2)
		require.Len(t, history.Executions, 1)
		stale := history
		stale.revision--
		require.Error(t, guard.Ledger().write(session, stale), "broker refuses a stale revision after process restart")
		forged := history
		forged.Executions = nil
		forged.Attempts = nil
		forgedBytes, err := json.Marshal(forged)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(guard.Ledger().Dir(session), 0o700))
		require.NoError(t, os.WriteFile(guard.Ledger().Path(session), forgedBytes, 0o600))
		require.False(t, guard.Check(t.Context(), session, "forged-repeat", Classify("rm -f a", dir)).Allowed)
		require.NoError(t, guard.BeginExecution(session, remaining, "retry"))
		require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-b")))
		require.NoError(t, guard.RecordOutcome(session, remaining, "retry", nil, false))
		require.False(t, guard.Check(t.Context(), session, "repeat-b", remaining).Allowed)
		require.False(t, guard.Check(t.Context(), session, "repeat-original", Classify("rm -f a b", dir)).Allowed)
		history, err = guard.Ledger().Read(session)
		require.NoError(t, err)
		require.Len(t, history.Confirmations, 2)
		require.Len(t, history.Executions, 2)
		require.Len(t, history.Attempts, 4)
		t.Logf("retry client_pid=%d prepare_pid=%d protected_revision=%d confirmations=%d executions=%d", os.Getpid(), info.PreparePID, history.revision, len(history.Confirmations), len(history.Executions))
	default:
		t.Fatalf("unsupported protected-authority test phase %q", phase)
	}
}
