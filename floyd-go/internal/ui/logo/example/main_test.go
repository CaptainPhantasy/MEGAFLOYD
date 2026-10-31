package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
)

func TestLogoExampleRendersInTerminal(t *testing.T) {
	if os.Getenv("MEGAFLOYD_LOGO_TEST_CHILD") == "1" {
		main()
		return
	}
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLogoExampleRendersInTerminal$")
	cmd.Env = append(os.Environ(), "MEGAFLOYD_LOGO_TEST_CHILD=1", "TERM=xterm-256color")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 60, Cols: 140})
	require.NoError(t, err)
	defer f.Close()
	var output bytes.Buffer
	_, _ = io.Copy(&output, f)
	require.NoError(t, cmd.Wait())
	require.Contains(t, output.String(), "v1.0.0")
	require.NotContains(t, output.String(), "Could not get terminal size")
	require.NotContains(t, output.String(), "panic:")
}
