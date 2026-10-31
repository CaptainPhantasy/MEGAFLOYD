package deletionguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeletionRecognizedOverwriteAndDiscardCommands(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		`cp /dev/null report.txt`, `cp -f /dev/null report.txt`,
		`cp /dev/null destination/`, `cp -t destination /dev/null`,
		`cp --target-directory=destination /dev/null`,
		`cp --unknown /dev/null report.txt`, `cp "$input" report.txt`,
		`git restore report.txt`, `git restore .`, `git checkout -- report.txt`,
		`git checkout HEAD -- report.txt`, `git -C . restore report.txt`,
	} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, "destination"), 0o700))
			for _, name := range []string{"report.txt", "destination/null"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("original"), 0o600))
			}
			c := Classify(command, dir)
			require.True(t, c.IsDestructive(), command)
			require.False(t, NewGuard(t.TempDir(), nil).Check(t.Context(), "s", "discard", c).Allowed)
		})
	}
	for _, command := range []string{`cp /dev/null new.txt`, `cp -n /dev/null report.txt`, `git status`, `git log -1`, `printf safe`} {
		require.False(t, Classify(command, t.TempDir()).IsDestructive(), command)
	}
}

func TestDeletionCombinedOutputRedirect(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "report.txt")
	require.NoError(t, os.WriteFile(target, []byte("original"), 0o600))
	c := Classify(`printf replacement &> report.txt`, dir)
	require.True(t, c.IsDestructive())
	require.Equal(t, []string{target}, c.Targets)
	var operations [][]string
	require.NoError(t, json.Unmarshal([]byte(c.Operations[target]), &operations))
	require.Len(t, operations, 1)
	require.Equal(t, "&>", operations[0][0])
	require.Contains(t, operations[0][1], "printf replacement")
	require.False(t, Classify(`printf replacement &>> report.txt`, dir).IsDestructive())
}

func TestDeletionDynamicArgumentsCannotHideLaterEffects(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		`find "$target" -delete`, `find "$target" -exec rm '{}' ';'`,
		`dd if="$input" of=report.txt`, `dd "$options" of=report.txt`,
		`env "$command" rm report.txt`, `sudo -u "$user" rm report.txt`,
		`xargs -I "$marker" rm '{}'`, `git "$option" restore report.txt`,
	} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			c := Classify(command, t.TempDir())
			require.True(t, c.IsDestructive())
			require.NotEmpty(t, c.Unresolved)
			require.NotContains(t, c.Targets, "$target")
		})
	}
	require.False(t, Classify(`printf '%s' "$value"`, t.TempDir()).IsDestructive())
}

func TestDeletionMixedQuarantineTargetsAreNotApprovedForDestruction(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	for _, c := range []Classification{
		Classify(`mv a saved; rm b`, dir),
		ClassifyAll([]string{`mv a saved`, `rm b`}, dir),
	} {
		require.True(t, c.IsDestructive())
		require.Equal(t, []string{filepath.Join(dir, "b")}, c.Targets)
		require.Len(t, c.Operations, 1)
		confirmer := &recoveryConfirmer{}
		guard := testGuard(t.TempDir(), confirmer)
		decision := guard.Check(t.Context(), "mixed", "call", c)
		require.True(t, decision.Allowed, decision.Reason)
		require.Equal(t, 2, confirmer.calls)
		require.NoError(t, guard.BeginExecution("mixed", c, "call"))
		record, err := guard.Ledger().Read("mixed")
		require.NoError(t, err)
		require.Len(t, record.Approvals, 1)
		require.Equal(t, []string{filepath.Join(dir, "b")}, record.Approvals[0].Targets)
		require.Len(t, record.Confirmations, 2)
		for _, confirmation := range record.Confirmations {
			require.Equal(t, []string{filepath.Join(dir, "b")}, confirmation.Targets)
		}
		require.Len(t, record.Attempts, 1)
		require.Equal(t, "started", record.Attempts[0].State)
		require.Empty(t, record.Executions)
	}
	for _, name := range []string{"a", "b"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		require.Equal(t, name, string(data))
	}
	_, err := os.Lstat(filepath.Join(dir, "saved"))
	require.ErrorIs(t, err, os.ErrNotExist)
	c := Classify(`mv a saved; mv b other`, dir)
	require.Equal(t, EffectQuarantine, c.Effect)
	require.Equal(t, []string{filepath.Join(dir, "a"), filepath.Join(dir, "b")}, c.Targets)
}

func TestDeletionCopyDestinationSemantics(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"source", "destination", "destination/source"} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o700))
	}
	for _, name := range []string{"report", "destination/report", "source/report", "destination/source/report"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("preserved"), 0o600))
	}
	require.NoError(t, os.Symlink("report", filepath.Join(dir, "linked")))
	for _, tc := range []struct {
		command string
		target  string
	}{
		{`cp /dev/null report`, "report"},
		{`cp report destination/`, "destination/report"},
		{`cp -t destination report`, "destination/report"},
		{`cp -T report linked`, "linked"},
		{`cp -n -f /dev/null report`, "report"},
		{`cp --no-clobber --interactive /dev/null report`, "report"},
	} {
		c := Classify(tc.command, dir)
		require.True(t, c.IsDestructive(), tc.command)
		require.Equal(t, []string{filepath.Join(dir, tc.target)}, c.Targets, tc.command)
	}
	for _, command := range []string{`cp -f -n /dev/null report`, `cp /dev/null new-report`, `cp -R source new-directory`} {
		require.False(t, Classify(command, dir).IsDestructive(), command)
	}
	for _, command := range []string{`cp -R source destination`, `cp a new-report; cp b new-report`, `cp a new-report; rm new-report`} {
		c := Classify(command, dir)
		require.True(t, c.IsDestructive(), command)
		require.NotEmpty(t, c.Unresolved, command)
	}
	for _, command := range []string{`sudo -u "$user" echo safe`, `env "$command" echo safe`, `xargs -I "$marker" echo safe`} {
		c := Classify(command, dir)
		require.True(t, c.IsDestructive(), command)
		require.NotEmpty(t, c.Unresolved, command)
	}
}

func TestDeletionInformationalOperandsCannotSuppressEffects(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"--help", "--version", "report", "source"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("original"), 0o600))
	}
	for _, command := range []string{
		`rm -- --help report`, `rm -- --version report`,
		`cp -- source --help`, `cp -- source --version`,
		`mv -- source --help`, `truncate -s 0 -- --version`,
		`find . -name --help -delete`, `find . -name --version -delete`,
		`git checkout -- --help`, `git restore -- --version`,
		`git clean -f -- --dry-run report`, `git clean -f -- -n report`,
	} {
		c := Classify(command, dir)
		require.True(t, c.IsDestructive(), command)
		require.False(t, NewGuard(t.TempDir(), nil).Check(t.Context(), "s", "help-operand", c).Allowed)
	}
	for _, command := range []string{`rm --help`, `cp --help`, `mv --version`, `git --version`, `find --help`, `git clean -n -- report`} {
		require.False(t, Classify(command, dir).IsDestructive(), command)
	}
}

func TestDeletionCopyDescriptorsBindEachCanonicalSource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "destination"), 0o700))
	for _, name := range []string{"a", "b", "target", "destination/a", "destination/b"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	for _, tc := range []struct {
		command string
		target  string
		source  string
		options []string
	}{
		{`cp ./a target`, "target", "a", nil},
		{`/bin/cp -p -f a target`, "target", "a", []string{"-p", "-f"}},
		{`cp -f -p a target`, "target", "a", []string{"-f", "-p"}},
		{`cp -- a target`, "target", "a", []string{"--"}},
		{`cp -t destination a`, "destination/a", "a", []string{"-t", "destination"}},
		{`cp --target-directory=destination b`, "destination/b", "b", []string{"--target-directory=destination"}},
		{`cp a b destination/`, "destination/a", "a", nil},
		{`cp a b destination/`, "destination/b", "b", nil},
	} {
		c := Classify(tc.command, dir)
		target := filepath.Join(dir, tc.target)
		var operations [][]string
		require.NoError(t, json.Unmarshal([]byte(c.Operations[target]), &operations), tc.command)
		require.Len(t, operations, 1, tc.command)
		expected := append([]string{operations[0][0]}, tc.options...)
		expected = append(expected, filepath.Join(dir, tc.source), target)
		require.Equal(t, expected, operations[0], tc.command)
	}
	all := Classify(`cp a b destination/`, dir)
	retry := Classify(`cp b destination/`, dir)
	target := filepath.Join(dir, "destination/b")
	require.Equal(t, all.Operations[target], retry.Operations[target])
	require.NotEqual(t, Classify(`cp -p -f a target`, dir).Operations[filepath.Join(dir, "target")], Classify(`cp -f -p a target`, dir).Operations[filepath.Join(dir, "target")])
}
