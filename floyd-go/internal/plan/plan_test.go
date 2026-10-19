package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnterStatusAndPaths(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	svc := NewService(dataDir)

	status, err := svc.Status("session-1")
	require.NoError(t, err)
	require.False(t, status.Active)
	require.Empty(t, status.Path)

	entered, err := svc.Enter("session-1")
	require.NoError(t, err)
	require.True(t, entered.Active)
	require.NotEmpty(t, entered.ID)
	require.Equal(
		t,
		filepath.Join(dataDir, "plans", "session-1", entered.ID+".md"),
		entered.Path,
	)

	status, err = svc.Status("session-1")
	require.NoError(t, err)
	require.True(t, status.Active)
	require.Equal(t, entered.ID, status.ID)
	require.Equal(t, entered.Path, status.Path)
	require.Empty(t, status.Content, "the plan file is not created on enter")

	_, err = svc.Enter("session-1")
	require.ErrorIs(t, err, ErrAlreadyActive)
}

func TestStatusMissingPlanFileReturnsPathWithEmptyContent(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir())
	entered, err := svc.Enter("session-1")
	require.NoError(t, err)

	status, err := svc.Status("session-1")
	require.NoError(t, err)
	require.Equal(t, entered.Path, status.Path)
	require.Empty(t, status.Content)
}

func TestClearWritesEmptyPlanFile(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir())
	entered, err := svc.Enter("session-1")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(entered.Path, []byte("# Plan"), 0o644))

	require.NoError(t, svc.Clear("session-1"))

	content, err := os.ReadFile(entered.Path)
	require.NoError(t, err)
	require.Empty(t, content)

	status, err := svc.Status("session-1")
	require.NoError(t, err)
	require.True(t, status.Active)
}

func TestCancelAndExitDeactivate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		leave func(*Service) error
	}{
		{"cancel", func(s *Service) error { return s.Cancel("session-1") }},
		{"exit", func(s *Service) error { return s.Exit("session-1") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := NewService(t.TempDir())
			_, err := svc.Enter("session-1")
			require.NoError(t, err)

			require.NoError(t, tc.leave(svc))

			status, err := svc.Status("session-1")
			require.NoError(t, err)
			require.False(t, status.Active)
			require.Empty(t, status.ID)
		})
	}
}

func TestSessionsAreIsolated(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir())
	first, err := svc.Enter("session-1")
	require.NoError(t, err)

	second, err := svc.Enter("session-2")
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)

	_, err = svc.Enter("session-1")
	require.ErrorIs(t, err, ErrAlreadyActive)

	require.NoError(t, svc.Exit("session-1"))
	status, err := svc.Status("session-2")
	require.NoError(t, err)
	require.True(t, status.Active, "exiting one session must not exit another")
}

func TestRecordRevisionWritesHashAndBytes(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir())
	entered, err := svc.Enter("session-1")
	require.NoError(t, err)

	content := []byte("# Plan\n\n1. Do the thing.\n")
	require.NoError(t, os.WriteFile(entered.Path, content, 0o644))

	revision, err := svc.RecordRevision("session-1")
	require.NoError(t, err)
	require.Equal(t, 1, revision.Version)
	require.Equal(t, len(content), revision.Bytes)

	sum := sha256.Sum256(content)
	require.Equal(t, hex.EncodeToString(sum[:]), revision.SHA256)
	require.Equal(t, filepath.Join(svc.RevisionsDir("session-1", entered.ID), "v1.md"), revision.Path)

	saved, err := os.ReadFile(revision.Path)
	require.NoError(t, err)
	require.Equal(t, content, saved)

	metaData, err := os.ReadFile(filepath.Join(svc.RevisionsDir("session-1", entered.ID), "v1.json"))
	require.NoError(t, err)
	var meta struct {
		Version int    `json:"version"`
		SHA256  string `json:"sha256"`
		Bytes   int    `json:"bytes"`
	}
	require.NoError(t, json.Unmarshal(metaData, &meta))
	require.Equal(t, revision.Version, meta.Version)
	require.Equal(t, revision.SHA256, meta.SHA256)
	require.Equal(t, revision.Bytes, meta.Bytes)

	second, err := svc.RecordRevision("session-1")
	require.NoError(t, err)
	require.Equal(t, 2, second.Version)
}

func TestRecordRevisionInactiveIsNoop(t *testing.T) {
	t.Parallel()

	revision, err := NewService(t.TempDir()).RecordRevision("session-1")
	require.NoError(t, err)
	require.Zero(t, revision.Version)
}

func TestReminderLifecycle(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir())
	require.Empty(t, svc.Reminder("session-1"))

	entered, err := svc.Enter("session-1")
	require.NoError(t, err)

	full := svc.Reminder("session-1")
	require.Contains(t, full, "Plan mode is active")
	require.Contains(t, full, "Plan file: "+entered.Path)

	sparse := svc.Reminder("session-1")
	require.Contains(t, sparse, "Plan mode still active")
	require.NotEqual(t, full, sparse)

	require.NoError(t, svc.Exit("session-1"))
	require.Equal(t, ExitReminder, svc.Reminder("session-1"))
	require.Empty(t, svc.Reminder("session-1"))
}

func TestReminderReentryAfterContextReset(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir())
	entered, err := svc.Enter("session-1")
	require.NoError(t, err)
	require.Contains(t, svc.Reminder("session-1"), "Plan mode is active")
	require.NoError(t, os.WriteFile(entered.Path, []byte("# Existing Plan"), 0o644))

	svc.MarkContextReset("session-1")
	reentry := svc.Reminder("session-1")
	require.True(t, strings.Contains(reentry, "Re-entering Plan Mode"), reentry)
	require.Contains(t, reentry, "Plan file: "+entered.Path)
}

func TestReminderReentryForPreexistingPlan(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	first := NewService(dataDir)
	entered, err := first.Enter("session-1")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(entered.Path, []byte("# Existing Plan"), 0o644))

	restarted := NewService(dataDir)
	require.True(t, strings.Contains(restarted.Reminder("session-1"), "Re-entering Plan Mode"))
}

func TestSanitizesPathComponents(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir())
	entered, err := svc.Enter("../session/one")
	require.NoError(t, err)
	require.NotContains(t, entered.Path, ".."+string(filepath.Separator))
	require.True(t, strings.HasPrefix(entered.Path, filepath.Join(svc.dataDir, "plans")))
}
