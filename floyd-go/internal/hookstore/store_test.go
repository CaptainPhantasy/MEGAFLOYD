package hookstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/stretchr/testify/require"
)

func TestTracePaginationFiltersAndReadOnlyRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(dir)) })
	store := New(conn)
	for _, record := range []Record{
		{ID: "one", EventID: "event1", Event: "PreToolUse", SessionID: "session1", ToolCallID: "tool1", State: "started"},
		{ID: "two", EventID: "event1", Event: "PreToolUse", SessionID: "session1", ToolCallID: "tool1", State: "succeeded"},
		{ID: "three", EventID: "event2", Event: "PreToolUse", SessionID: "session2", ToolCallID: "tool2", State: "failed"},
	} {
		require.NoError(t, store.Append(t.Context(), record))
	}
	first, err := store.List(t.Context(), Query{SessionID: "session1", Limit: 1})
	require.NoError(t, err)
	require.Len(t, first.Records, 1)
	require.Equal(t, "one", first.Records[0].ID)
	require.True(t, first.HasMore)
	second, err := store.List(t.Context(), Query{SessionID: "session1", After: first.Next, Limit: 1})
	require.NoError(t, err)
	require.Len(t, second.Records, 1)
	require.Equal(t, "two", second.Records[0].ID)
	require.False(t, second.HasMore)
	require.Greater(t, second.Next, first.Next)
	reader, closer, err := OpenReader(dir)
	require.NoError(t, err)
	defer closer.Close()
	page, err := reader.List(t.Context(), Query{ToolCallID: "tool2"})
	require.NoError(t, err)
	require.Len(t, page.Records, 1)
	require.Equal(t, "failed", page.Records[0].State)
	for _, query := range []Query{{After: -1}, {Limit: -1}, {Limit: MaxLimit + 1}} {
		_, err := store.List(t.Context(), query)
		require.Error(t, err)
	}
	require.Error(t, store.Append(t.Context(), Record{ID: "invalid"}))
}

func TestMissingTraceDoesNotCreateDatabaseAndCorruptionIsReported(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reader, closer, err := OpenReader(dir)
	require.NoError(t, err)
	defer closer.Close()
	page, err := reader.List(t.Context(), Query{})
	require.NoError(t, err)
	require.Empty(t, page.Records)
	require.NoFileExists(t, filepath.Join(dir, "floyd.db"))
	require.Error(t, New(nil).Append(t.Context(), Record{ID: "one", EventID: "e", Event: "PreToolUse"}))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "floyd.db"), []byte("not a sqlite database"), 0o600))
	reader, closer, err = OpenReader(dir)
	if err == nil {
		defer closer.Close()
		_, err = reader.List(t.Context(), Query{})
	}
	require.Error(t, err)
}
