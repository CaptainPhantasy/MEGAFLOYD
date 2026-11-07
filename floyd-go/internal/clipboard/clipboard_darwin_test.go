//go:build darwin && !ios

package clipboard

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMacTextFallbackPreservesBytesAndReportsFailures(t *testing.T) {
	init, command := nativeClipboardInit, clipboardTextCommand
	t.Cleanup(func() { nativeClipboardInit = init; clipboardTextCommand = command })
	nativeClipboardInit = func() error { return errors.New("native clipboard unavailable") }
	var stored string
	var programs []string
	clipboardTextCommand = func(ctx context.Context, program, input string) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 3*time.Second)
		programs = append(programs, program)
		if program == "/usr/bin/pbcopy" {
			stored = input
			return nil, nil
		}
		require.Equal(t, "/usr/bin/pbpaste", program)
		return []byte(stored), nil
	}
	want := "Complete answer — including Unicode\nand its last line"
	require.NoError(t, WriteText(want))
	got, err := Read(FormatText)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
	require.Equal(t, []string{"/usr/bin/pbcopy", "/usr/bin/pbpaste"}, programs)
	clipboardTextCommand = func(context.Context, string, string) ([]byte, error) { return nil, errors.New("service unavailable") }
	require.ErrorIs(t, WriteText(want), ErrWrite)
	_, err = Read(FormatText)
	require.ErrorContains(t, err, "service unavailable")
}

func TestMacClipboardRealRoundTrip(t *testing.T) {
	fixture := os.Getenv("MEGAFLOYD_CLIPBOARD_FIXTURE")
	if fixture == "" {
		t.Skip("run with the private all-types pasteboard preservation harness")
	}
	require.NoError(t, WriteText(fixture))
	got, err := Read(FormatText)
	require.NoError(t, err)
	// Do not include clipboard contents in assertion output.
	if string(got) != fixture {
		t.Fatal("clipboard roundtrip bytes did not match the isolated fixture")
	}
}
