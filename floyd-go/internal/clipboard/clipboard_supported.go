//go:build (darwin || linux || windows || freebsd || openbsd || netbsd) && !ios && !android

package clipboard

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.design/x/clipboard"
)

var (
	nativeClipboardInit  = clipboard.Init
	nativeClipboardWrite = clipboard.Write
	nativeClipboardRead  = clipboard.Read
	clipboardTextCommand = runClipboardTextCommand
)

func runClipboardTextCommand(ctx context.Context, program, input string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, program)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	return cmd.Output()
}

func initClipboard() error {
	return nativeClipboardInit()
}

func writeText(text string) error {
	if err := nativeClipboardInit(); err == nil {
		if nativeClipboardWrite(clipboard.FmtText, []byte(text)) != nil {
			return nil
		}
	}
	// The native library needs CGO on macOS; release builds without CGO
	// can still use the OS clipboard service through its bundled client.
	if runtime.GOOS == "darwin" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := clipboardTextCommand(ctx, "/usr/bin/pbcopy", text); err != nil {
			return fmt.Errorf("%w: %v", ErrWrite, err)
		}
		return nil
	}
	return ErrWrite
}

func read(f Format) ([]byte, error) {
	var format clipboard.Format
	switch f {
	case FormatText:
		format = clipboard.FmtText
	case FormatImage:
		format = clipboard.FmtImage
	default:
		return nil, ErrEmpty
	}
	initErr := nativeClipboardInit()
	if initErr == nil {
		if data := nativeClipboardRead(format); data != nil {
			return data, nil
		}
	}
	if runtime.GOOS == "darwin" && f == FormatText {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		data, err := clipboardTextCommand(ctx, "/usr/bin/pbpaste", "")
		if err != nil {
			return nil, fmt.Errorf("clipboard read failed: %w", err)
		}
		if len(data) == 0 {
			return nil, ErrEmpty
		}
		return data, nil
	}
	if initErr != nil {
		return nil, initErr
	}
	return nil, ErrEmpty
}
