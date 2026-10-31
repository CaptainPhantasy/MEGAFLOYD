package main

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthorityEntryHelper(t *testing.T) {
	if os.Getenv("MEGAFLOYD_AUTHORITY_TEST") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"megafloyd-approval-authority"}, os.Args[i+1:]...)
			break
		}
	}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	main()
}

func TestAuthorityCLIRejectsInvalidIdentityBeforeStartup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		args    []string
		want    string
		success bool
	}{
		{"help", []string{"--help"}, "allow-executable", true},
		{"bad pin", []string{"--allow-executable", "no-pin"}, "invalid executable pin", false},
		{"bad uid", []string{"--allow-uid", "not-a-number"}, "invalid syntax", false},
		{"negative uid", []string{"--allow-uid", "-1"}, "invalid syntax", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestAuthorityEntryHelper$", "--"}, tc.args...)...)
			child.Dir = t.TempDir()
			child.Env = []string{"PATH=" + os.Getenv("PATH"), "MEGAFLOYD_AUTHORITY_TEST=1"}
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

func TestRepeatedIdentityFlagsRetainAllPins(t *testing.T) {
	t.Parallel()
	var values stringsFlag
	flags := flag.NewFlagSet("fixture", flag.ContinueOnError)
	flags.Var(&values, "pin", "test pin")
	require.NoError(t, flags.Parse([]string{"-pin", "first=aa", "-pin", "second=bb"}))
	require.Equal(t, stringsFlag{"first=aa", "second=bb"}, values)
	require.Equal(t, "first=aa,second=bb", values.String())
}
