package diff

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateDiffReportsActualLineChanges(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, before, after string
		added, removed      int
	}{
		{"unchanged", "hello\n", "hello\n", 0, 0},
		{"replacement", "first\nold\n", "first\nnew\n", 1, 1},
		{"content resembling headers", "--old\n", "++new\n", 1, 1},
		{"missing final newline", "old", "new", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, added, removed := GenerateDiff(tc.before, tc.after, "/src/file.go")
			require.Equal(t, tc.added, added, output)
			require.Equal(t, tc.removed, removed, output)
			if tc.before == tc.after {
				require.Empty(t, output)
			} else {
				require.Contains(t, output, "--- a/src/file.go\n+++ b/src/file.go\n")
			}
		})
	}
}
