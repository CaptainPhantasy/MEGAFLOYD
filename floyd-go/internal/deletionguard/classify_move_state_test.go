package deletionguard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeletionMoveSequencesFailClosedOnOverlappingState(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "tree"), 0o700))
	require.NoError(t, os.Symlink("tree", filepath.Join(dir, "alias")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tree/child"), []byte("preserved"), 0o600))
	for _, command := range []string{
		"mv a out; mv b out",
		"mv a Out; mv b out",
		"mv a café; mv b cafe\u0301",
		"mv a alias/out; mv b tree/out",
		"mv a alias/out; rm tree/out",
		"mv a out; rm out",
		"mv tree parked; rm parked/child",
		"mv a out; mv out other",
		"rm a; mv b a",
		"bash -c 'mv a out'; mv b out",
		"mv a out; bash -c 'rm out'",
		"if true; then mv a out; fi; mv b out",
	} {
		classification := Classify(command, dir)
		require.True(t, classification.IsDestructive(), command)
		require.NotEmpty(t, classification.Unresolved, command)
		confirmer := &recoveryConfirmer{}
		decision := testGuard(t.TempDir(), confirmer).Check(t.Context(), "move-state", "call", classification)
		require.False(t, decision.Allowed, "confirmations cannot authorize unproven moved data")
		require.Zero(t, confirmer.calls)
	}
	for _, commands := range [][]string{{"mv a out", "mv b out"}, {"mv a out", "rm out"}, {"rm a", "mv b a"}} {
		classification := ClassifyAll(commands, dir)
		require.True(t, classification.IsDestructive(), commands)
		require.NotEmpty(t, classification.Unresolved, commands)
	}
	for _, command := range []string{"mv a out; mv b other", "mv a out; printf usable", "bash -c 'mv a out'; mv b other", "bash -c 'cd tree'; mv a out", "(cd tree); mv a out", "echo $(cd tree); mv a out"} {
		require.False(t, Classify(command, dir).IsDestructive(), command)
	}
	require.False(t, ClassifyAll([]string{"mv a out", "mv b other"}, dir).IsDestructive())
	disjoint := ClassifyAll([]string{"cd tree", "rm a"}, dir)
	require.True(t, disjoint.IsDestructive())
	require.Empty(t, disjoint.Unresolved, "independent command directories must remain independent")
}

func TestDeletionUnprovenDirectoryAndParseStateFailsClosed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o700))
	for _, name := range []string{"important", "out"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", name), []byte("preserved"), 0o600))
	}
	for _, command := range []string{
		"if true; then cd sub; fi; : > important",
		"if true; then cd sub; fi; mv a out",
		"cd sub; : > important",
		"rm important\nif true; then",
		"printf safe\nif true; then",
	} {
		classification := Classify(command, dir)
		require.True(t, classification.IsDestructive(), command)
		require.NotEmpty(t, classification.Unresolved, command)
	}
	require.False(t, Classify("if true; then cd sub; fi; printf safe", dir).IsDestructive())
	require.False(t, Classify("cd sub; printf safe > /dev/null", dir).IsDestructive())
	classification := ClassifyAll([]string{"printf safe", "if true; then"}, dir)
	require.True(t, classification.IsDestructive())
	require.NotEmpty(t, classification.Unresolved)
}
