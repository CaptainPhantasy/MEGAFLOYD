package terminal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
)

func TestTerminalSessionExecutesInWorkspaceAndResizes(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("SHELL", "/bin/sh")
	s, err := NewSession(10, 40, root)
	require.NoError(t, err)
	defer s.Close()
	_, err = s.Write([]byte("printf '%s\n' \"$PWD\"; printf 'TERMINAL-%s\n' 'READY'\n"))
	require.NoError(t, err)
	done := make(chan []byte, 1)
	go func() {
		var b bytes.Buffer
		buf := make([]byte, 1024)
		for {
			n, err := s.Read(buf)
			b.Write(buf[:n])
			if bytes.Contains(b.Bytes(), []byte("TERMINAL-READY")) || err != nil {
				done <- b.Bytes()
				return
			}
		}
	}()
	select {
	case output := <-done:
		require.Contains(t, string(output), root)
		require.Contains(t, string(output), "TERMINAL-READY")
	case <-time.After(3 * time.Second):
		t.Fatal("shell did not execute input")
	}
	s.Resize(20, 80)
	size, err := pty.GetsizeFull(s.ptyFile)
	require.NoError(t, err)
	require.Equal(t, uint16(20), size.Rows)
	require.Equal(t, uint16(80), size.Cols)
	require.NoError(t, s.Close())
	require.True(t, s.Closed())
	require.NoError(t, s.Close())
}

func TestTerminalComponentRoutesFocusAndRendersOutput(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("SHELL", "/bin/sh")
	c, cmd, err := New("term-test", 10, 40, root)
	require.NoError(t, err)
	require.NotNil(t, cmd)
	defer c.Close()
	handled, _ := c.Update(OutputMsg{ID: "different", Data: []byte("wrong")})
	require.False(t, handled)
	handled, _ = c.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.False(t, handled)
	c.Focus()
	require.True(t, c.Focused())
	c.Blur()
	require.False(t, c.Focused())
	handled, next := c.Update(OutputMsg{ID: c.ID, Data: []byte("visible output")})
	require.True(t, handled)
	require.NotNil(t, next)
	scr := uv.NewScreenBuffer(40, 10)
	c.Draw(scr, uv.Rect(0, 0, 40, 10))
	require.Contains(t, scr.Render(), "visible output")
	c.Close()
	require.False(t, c.Active())
	_, err = NewSession(10, 40, filepath.Join(root, "missing"))
	require.Error(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "preserved"), []byte("safe"), 0o600))
}
