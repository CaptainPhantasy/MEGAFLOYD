package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMainEntryHelper(t *testing.T) {
	if os.Getenv("MEGAFLOYD_ENTRY_TEST") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"megafloyd"}, os.Args[i+1:]...)
			break
		}
	}
	main()
}

func TestMainEntrypointHelpAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		want    string
		success bool
	}{
		{"help", []string{"--help"}, "MegaFloyd", true},
		{"unknown flag", []string{"--definitely-unknown-flag"}, "Unknown flag", false},
		{"dry run", []string{"ai", "dry-run", "--model", "fixture-model", "--user", "hello", "--pretty=false"}, `"model":"fixture-model"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestMainEntryHelper$", "--"}, tc.args...)...)
			child.Dir = root
			child.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "MEGAFLOYD_HOME=" + filepath.Join(root, "home"), "MEGAFLOYD_BINARY_NAME=megafloyd", "MEGAFLOYD_ENTRY_TEST=1", "FLOYD_DISABLE_PROVIDER_AUTO_UPDATE=1", "NO_COLOR=1"}
			output, err := child.CombinedOutput()
			if tc.success {
				require.NoError(t, err, string(output))
			} else {
				require.Error(t, err, string(output))
			}
			require.Contains(t, string(output), tc.want)
			require.NoError(t, ctx.Err())
		})
	}
}
