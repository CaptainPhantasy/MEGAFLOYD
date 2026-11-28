package deletionguard

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeletionClassificationNestedShell(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		`rm -f important.txt`,
		`if true; then rm -f important.txt; fi`,
		`if false; then echo safe; elif true; then rm -f important.txt; else echo safe; fi`,
		`if rm -f important.txt; then echo done; fi`,
		`for item in important.txt; do rm "$item"; done`,
		`while true; do rm -f important.txt; break; done`,
		`until rm -f important.txt; do break; done`,
		`for ((i=0; i<1; i++)); do rm -f important.txt; done`,
		`case x in x) rm -f important.txt;; esac`,
		`if true; then for item in important.txt; do { (rm "$item"); }; done; fi`,
		`echo safe | { rm -f important.txt; }`,
		`false || rm -f important.txt`,
		`printf '%s' "$(rm -f important.txt)"`,
		"echo `rm -f important.txt`",
		`value=$(rm -f important.txt)`,
		`cat <(rm -f important.txt)`,
		`cat <<EOF2
$(rm -f important.txt)
EOF2`,
		`[[ $(rm -f important.txt) == x ]]`,
		`local value=$(rm -f important.txt)`,
		`time rm -f important.txt`,
		`coproc rm -f important.txt`,
		`function remove() { rm -f important.txt; }; remove`,
		`"$command" important.txt`,
		`if true; then echo data > "$target"; fi`,
		`bash -c "$script"`,
		`eval "$script"`,
		`eval 'if true; then rm -f important.txt; fi'`,
		`bash -lc 'if true; then rm -f important.txt; fi'`,
		`bash -c 'rm -f important.txt' --help`,
		`command rm -f important.txt`,
		`builtin eval 'rm -f important.txt'`,
	} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			classification := Classify(command, t.TempDir())
			require.True(t, classification.IsDestructive(), "%q was classified harmless: %+v", command, classification)
			guard := testGuard(t.TempDir(), nil)
			require.False(t, guard.Check(context.Background(), "session", "call", classification).Allowed)
		})
	}
}

func TestDeletionClassificationHarmlessNestedShell(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		`if true; then echo safe; else printf safe; fi`,
		`for item in a b; do echo "$item"; done`,
		`while false; do echo safe; done`,
		`case x in x) echo safe;; esac`,
		`echo "$(printf safe)"`,
		`value=$(printf safe)`,
		`[[ x == x ]]`,
		`((1 + 1))`,
		`printf '%s' 'rm -f important.txt'`,
		`go build ./...`,
		`command -v rm`,
		`bash -c 'echo safe'`,
		`eval 'echo safe'`,
	} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			classification := Classify(command, t.TempDir())
			require.False(t, classification.IsDestructive(), "%q: %+v", command, classification)
		})
	}
}

func TestDeletionClassificationDynamicNestedTargetsFailClosed(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		`if true; then rm "$target"; fi`,
		`for item in important.txt; do rm "$item"; done`,
		`case x in x) rm *.txt;; esac`,
		`echo "$(rm "$target")"`,
		`if true; then cd other; fi; rm -f important.txt`,
		`if true; then cd "$directory"; rm -f important.txt; fi`,
		`command cd other; rm -f important.txt`,
		`eval 'cd other'; rm -f important.txt`,
		`if true; then echo data > "$target"; fi`,
		`bash -c "$script"`,
		`eval "$script"`,
		`"$command" important.txt`,
	} {
		classification := Classify(command, t.TempDir())
		require.True(t, classification.IsDestructive(), command)
		require.NotEmpty(t, classification.Unresolved, command)
	}
}

func TestDeletionOperationIdentityBindsOptionsAndMixedEffects(t *testing.T) {
	dir := t.TempDir()
	target, err := ResolveTarget(dir, "b")
	require.NoError(t, err)
	original := Classify("truncate -s 5 a b", dir)
	retry := Classify("truncate -s 5 b", dir)
	require.NotEmpty(t, original.Operations[target])
	require.Equal(t, original.Operations[target], retry.Operations[target])
	require.NotEqual(t, original.Operations[target], Classify("truncate -s 0 b", dir).Operations[target])
	rm := Classify("rm -f a b", dir)
	require.Equal(t, rm.Operations[target], Classify("rm -f b", dir).Operations[target])
	require.Equal(t, rm.Operations[target], ClassifyAll([]string{"rm -f a", "rm -f b"}, dir).Operations[target])
	require.NotEqual(t, rm.Operations[target], Classify("rm -f b; shred b", dir).Operations[target])
	require.NotEqual(t, rm.Operations[target], Classify("rm -f b; rm -f b", dir).Operations[target])
	require.NotEqual(t, rm.Operations[target], Classify("/bin/rm -f b", dir).Operations[target])
	require.NotEqual(t, Classify("rsync --delete a b", dir).Operations[target], Classify("rsync --delete b a", dir).Operations[target])
	require.True(t, rm.RemovesTarget(target))
	require.False(t, original.RemovesTarget(target))
	require.False(t, Classify("rm -f b; shred b", dir).RemovesTarget(target))
	require.False(t, (Classification{}).RemovesTarget(target))
}

func TestDeletionOperationIdentityBindsRedirectProducer(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("original"), 0o600))
	}
	target := filepath.Join(dir, "b")
	original := Classify("echo first >a >b", dir)
	require.NotEmpty(t, original.Operations[target])
	require.Equal(t, original.Operations[target], Classify("echo first >b", dir).Operations[target])
	require.NotEqual(t, original.Operations[target], Classify("echo second >b", dir).Operations[target])
	require.NotEqual(t, Classify("cat <input >b", dir).Operations[target], Classify("cat <other >b", dir).Operations[target])
}

func TestDeletionAbsoluteRemovalMustActuallyRemoveTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a")
	require.NoError(t, os.WriteFile(target, []byte("original"), 0o600))
	confirmer := &recoveryConfirmer{}
	guard := testGuard(t.TempDir(), confirmer)
	classification := Classify("/bin/rm -f a", dir)
	require.True(t, classification.RemovesTarget(target))
	require.True(t, guard.Check(t.Context(), "absolute-rm", "one", classification).Allowed)
	require.NoError(t, guard.BeginExecution("absolute-rm", classification, "one"))
	require.NoError(t, os.WriteFile(target, []byte("changed but remains"), 0o600))
	require.NoError(t, guard.RecordOutcome("absolute-rm", classification, "one", classification.Targets, false))
	record, err := guard.Ledger().Read("absolute-rm")
	require.NoError(t, err)
	require.Empty(t, record.Executions, "a changed file is not a proven completed unlink")
}

func TestDeletionExactTargetPreservesWhitespace(t *testing.T) {
	dir := t.TempDir()
	spaced := Classify("rm -f 'a '", dir)
	plain := Classify("rm -f a", dir)
	require.Equal(t, []string{filepath.Join(dir, "a ")}, spaced.Targets)
	require.Equal(t, []string{filepath.Join(dir, "a")}, plain.Targets)
	require.NotEqual(t, spaced.Digest(), plain.Digest())
	require.Equal(t, []string{" ", "/a "}, CanonicalTargets([]string{"", " ", "/a "}))
}

func TestDeletionClassificationAsynchronousStatementsFailClosed(t *testing.T) {
	for _, command := range []string{`rm -f important.txt &`, `{ rm -f important.txt; } &`, `if true; then rm -f important.txt; fi &`} {
		classification := Classify(command, t.TempDir())
		require.True(t, classification.IsDestructive())
		require.NotEmpty(t, classification.Unresolved)
		require.False(t, testGuard(t.TempDir(), nil).Check(t.Context(), "s", "call", classification).Allowed)
	}
	require.False(t, Classify(`echo safe &`, t.TempDir()).IsDestructive())
}
