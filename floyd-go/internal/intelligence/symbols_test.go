package intelligence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSymbolIndexFindsCaseInsensitiveExactMatchBeforePrefixes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "example.go")
	require.NoError(t, os.WriteFile(path, []byte("package sample\n\nfunc Greet() {}\nfunc Greeter() {}\n"), 0o600))
	index := NewSymbolIndex(root)
	require.NoError(t, index.IndexFile(t.Context(), path))
	symbols := index.Query("greet", 1)
	require.Len(t, symbols, 1)
	require.Equal(t, "Greet", symbols[0].Name)
	require.Equal(t, 3, symbols[0].Line)
	require.True(t, symbols[0].Exported)
	all := index.QueryByFile(path)
	require.Len(t, all, 2)
	all[0].Name = "changed-copy"
	require.NotEqual(t, "changed-copy", index.QueryByFile(path)[0].Name)
	require.NoError(t, os.WriteFile(path, []byte("package sample\nfunc Farewell() {}\n"), 0o600))
	future := time.Now().Add(time.Second)
	require.NoError(t, os.Chtimes(path, future, future))
	require.NoError(t, index.IndexFile(t.Context(), path))
	require.Empty(t, index.Query("Greet", 0))
	require.Len(t, index.QueryByKind(SymbolKindFunction, 0), 1)
	require.Equal(t, "Farewell", index.Query("fare", 0)[0].Name)
	index.Clear()
	require.Empty(t, index.QueryByFile(path))
	require.Error(t, index.IndexFile(t.Context(), filepath.Join(root, "missing.go")))
}
