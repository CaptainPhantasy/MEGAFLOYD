package megafloydweb

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerminalResetSequenceIsExact(t *testing.T) {
	t.Parallel()

	const want = "\x1b[<u\x1b[>4;0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?25h"
	require.Equal(t, want, TerminalResetSequence)
}

func TestWriteTerminalReset(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	require.NoError(t, WriteTerminalReset(&out))
	require.Equal(t, TerminalResetSequence, out.String())
	require.Equal(t, 1, bytes.Count(out.Bytes(), []byte("\x1b[?25h")))
}
