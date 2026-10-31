package util

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusCommandsPreserveSeverityAndError(t *testing.T) {
	t.Parallel()
	require.Equal(t, InfoMsg{Type: InfoTypeInfo, Msg: "Saved"}, ReportInfo("Saved")())
	require.Equal(t, InfoMsg{Type: InfoTypeWarn, Msg: "Try again"}, ReportWarn("Try again")())
	require.Equal(t, InfoMsg{Type: InfoTypeError, Msg: "disk unavailable"}, ReportError(errors.New("disk unavailable"))())
	require.True(t, (InfoMsg{}).IsEmpty())
	require.False(t, NewWarnMsg("warning").IsEmpty())
}

func TestExecShellRejectsMalformedAndEmptyCommands(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"", `editor "unterminated`} {
		cmd := ExecShell(context.Background(), input, nil)
		msg, ok := cmd().(InfoMsg)
		require.True(t, ok)
		require.Equal(t, InfoTypeError, msg.Type)
		require.NotEmpty(t, msg.Msg)
	}
}
