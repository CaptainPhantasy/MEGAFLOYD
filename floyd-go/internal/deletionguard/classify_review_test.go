package deletionguard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeletionMoveEffectiveDirectoryDestinations(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"source", "other", "quarantine"} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o700))
	}
	for _, name := range []string{"source/report.txt", "other/notes.txt", "quarantine/report.txt", "quarantine/notes.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	for _, tc := range []struct {
		command string
		targets []string
	}{
		{`mv source/report.txt quarantine/`, []string{filepath.Join(dir, "quarantine/report.txt")}},
		{`mv source/report.txt other/notes.txt quarantine/`, []string{filepath.Join(dir, "quarantine/notes.txt"), filepath.Join(dir, "quarantine/report.txt")}},
		{`mv -t quarantine source/report.txt`, []string{filepath.Join(dir, "quarantine/report.txt")}},
		{`mv --target-directory=quarantine source/report.txt`, []string{filepath.Join(dir, "quarantine/report.txt")}},
		{`mv -T source/report.txt quarantine/report.txt`, []string{filepath.Join(dir, "quarantine/report.txt")}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			classification := Classify(tc.command, dir)
			require.True(t, classification.IsDestructive(), tc.command)
			require.Equal(t, tc.targets, classification.Targets)
			require.False(t, NewGuard(t.TempDir(), nil).Check(t.Context(), "s", "move", classification).Allowed)
		})
	}
	for _, command := range []string{`mv -n source/report.txt quarantine/`, `mv --no-clobber source/report.txt quarantine/`, `mv source/report.txt empty-new-destination`} {
		require.False(t, Classify(command, dir).IsDestructive(), command)
	}
	for _, command := range []string{`mv --unknown source/report.txt quarantine/`, `mv source/report.txt "$destination"`, `mv -t "$destination" source/report.txt`} {
		classification := Classify(command, dir)
		require.True(t, classification.IsDestructive(), command)
		require.NotEmpty(t, classification.Unresolved, command)
	}
}

func TestDeletionWrapperOptionsCannotHideCommand(t *testing.T) {
	for _, command := range []string{
		`sudo -n rm important.txt`,
		`sudo -u root rm important.txt`,
		`sudo --user=root -- rm important.txt`,
		`sudo -n -u root bash -c 'if true; then rm important.txt; fi'`,
		`doas -n -u root rm important.txt`,
		`nohup -- rm important.txt`,
		`sudo --unknown rm important.txt`,
		`sudo -D /elsewhere rm important.txt`,
		`sudo --host elsewhere rm important.txt`,
		`/usr/bin/time -o important.txt echo safe`,
		`sudo "$command" important.txt`,
		`sudo PATH=/different rm important.txt`,
		`sudo -- PATH=/different rm important.txt`,
		`sudo -nEu root rm important.txt`,
		`sudo -uroot rm important.txt`,
		`doas -n -uroot rm important.txt`,
		`/usr/bin/time -p rm important.txt`,
	} {
		t.Run(command, func(t *testing.T) {
			classification := Classify(command, t.TempDir())
			require.True(t, classification.IsDestructive(), command)
			require.False(t, NewGuard(t.TempDir(), nil).Check(t.Context(), "s", "wrapper", classification).Allowed)
		})
	}
	for _, command := range []string{`sudo -n echo safe`, `sudo -u root printf safe`, `doas -n echo safe`, `nohup -- echo safe`} {
		require.False(t, Classify(command, t.TempDir()).IsDestructive(), command)
	}
}

func TestDeletionMoveSymlinkAndDuplicateDestinations(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one", "two", "destination"} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o700))
	}
	for _, name := range []string{"one/report", "two/report", "destination/report"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	require.NoError(t, os.Symlink("destination", filepath.Join(dir, "linked")))
	target := filepath.Join(dir, "linked/report")
	classification := Classify("mv one/report linked/", dir)
	require.True(t, classification.IsDestructive())
	require.Equal(t, []string{target}, classification.Targets)
	duplicate := Classify("mv one/report two/report destination/", dir)
	require.True(t, duplicate.IsDestructive())
	require.NotEmpty(t, duplicate.Unresolved)
	require.False(t, Classify("mv -n one/report two/report destination/", dir).IsDestructive())
}

func TestDeletionLaunchWrappersCannotHideCommand(t *testing.T) {
	for _, command := range []string{
		`env rm important.txt`, `env -- rm important.txt`,
		`xargs env rm`, `xargs -I {} nice -n 5 rm '{}'`, `xargs --max-args=1 timeout 5 rm`, `xargs sh -c 'rm important.txt' --help`, `/usr/bin/env nice -n 5 rm important.txt`,
		`nice rm important.txt`, `nice -n 5 rm important.txt`, `nice --adjustment=-5 rm important.txt`, `nice -10 rm important.txt`,
		`timeout 5 rm important.txt`, `timeout --signal=KILL --kill-after=2 5s rm important.txt`, `timeout -s TERM -k 2 5 rm important.txt`,
		`setsid -fw rm important.txt`, `stdbuf -o L rm important.txt`, `stdbuf --output=L timeout 5 env rm important.txt`,
		`env PATH=/different rm important.txt`, `env FOO=value echo safe`, `env -i rm important.txt`,
		`env -u PATH rm important.txt`, `env -C /elsewhere rm important.txt`, `env -S 'rm important.txt'`,
		`timeout --unknown 5 rm important.txt`, `nice --unknown rm important.txt`, `env --unknown rm important.txt`,
		`env "$command" important.txt`, `xargs "$command"`, `timeout "$duration" rm important.txt`,
	} {
		t.Run(command, func(t *testing.T) {
			classification := Classify(command, t.TempDir())
			require.True(t, classification.IsDestructive(), command)
			require.False(t, NewGuard(t.TempDir(), nil).Check(t.Context(), "s", "launch-wrapper", classification).Allowed)
		})
	}
	for _, command := range []string{`env echo safe`, `env -- echo safe`, `xargs -n 1 echo`, `nice -n 5 echo safe`, `timeout 2 printf safe`, `setsid -f echo safe`, `stdbuf -o L echo safe`} {
		require.False(t, Classify(command, t.TempDir()).IsDestructive(), command)
	}
	for _, command := range []string{`env PATH=/different echo safe`, `env -C /different echo safe`, `env -S 'rm important.txt'`, `timeout "$duration" rm important.txt`, `nice -n "$priority" rm important.txt`} {
		classification := Classify(command, t.TempDir())
		require.True(t, classification.IsDestructive(), command)
		require.NotEmpty(t, classification.Unresolved, command)
	}
}
