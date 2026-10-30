package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeletionGuardNestedCommandsNeverReachToolWithoutAuthorization(t *testing.T) {
	for _, command := range []string{
		`if true; then rm widget; fi`,
		`for item in widget; do rm "$item"; done`,
		`case x in x) { (rm widget); };; esac`,
		`echo "$(rm widget)"`,
		`if true; then echo data > "$target"; fi`,
		`bash -c "$script"`,
		`sudo -n rm widget`,
		`sudo -u root rm widget`,
		`nohup -- rm widget`,
		`env rm widget`,
		`xargs env rm widget`,
		`nice -n 5 rm widget`,
		`timeout 2 rm widget`,
		`setsid -fw rm widget`,
		`stdbuf -o L env rm widget`,
	} {
		t.Run(command, func(t *testing.T) {
			for _, answers := range [][]bool{nil, {true, false}} {
				f := newDeletionFixture(t)
				f.answers = answers
				response, ran := f.run("session-nested", command)
				require.True(t, response.IsError, response.Content)
				require.False(t, ran, "unauthorized nested commands cannot enter the execution tool")
				require.Empty(t, f.record("session-nested").Executions)
			}
		})
	}
}

func TestDeletionGuardDirectoryMoveCannotOverwriteWithoutAuthorization(t *testing.T) {
	f := newDeletionFixture(t)
	for _, name := range []string{"source", "quarantine"} {
		require.NoError(t, os.Mkdir(filepath.Join(f.workingDir, name), 0o700))
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.workingDir, "source/report"), []byte("incoming"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(f.workingDir, "quarantine/report"), []byte("preserved"), 0o600))
	response, ran := f.run("move-overwrite", "mv source/report quarantine/")
	require.True(t, response.IsError)
	require.False(t, ran)
	original, err := os.ReadFile(filepath.Join(f.workingDir, "quarantine/report"))
	require.NoError(t, err)
	require.Equal(t, "preserved", string(original))
	require.Empty(t, f.record("move-overwrite").Executions)
}
