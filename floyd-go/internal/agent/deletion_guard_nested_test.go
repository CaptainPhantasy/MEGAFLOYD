package agent

import (
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
