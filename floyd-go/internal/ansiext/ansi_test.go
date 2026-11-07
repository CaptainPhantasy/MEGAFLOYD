package ansiext

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEscapeMakesControlSequencesVisible(t *testing.T) {
	t.Parallel()
	require.Equal(t, "␛[31mred␛[0m␊␉␀␍␡", Escape("\x1b[31mred\x1b[0m\n\t\x00\r\x7f"))
	require.Equal(t, "café 雪 👩‍💻", Escape("café 雪 👩‍💻"))
	require.Empty(t, Escape(""))
}
