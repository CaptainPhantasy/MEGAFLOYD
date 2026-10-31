package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestWebFlagsRegisteredOnRootAndRun(t *testing.T) {
	t.Parallel()

	for _, cmd := range []*cobra.Command{rootCmd, runCmd} {
		web := cmd.Flags().Lookup(webFlag)
		require.NotNil(t, web, "%s --web", cmd.Name())
		require.Equal(t, "false", web.DefValue)

		resume := cmd.Flags().Lookup(resumeFlag)
		require.NotNil(t, resume, "%s --resume", cmd.Name())
		require.Equal(t, "r", resume.Shorthand)
	}
}

func TestResumeWithoutWebUsesTerminalHandler(t *testing.T) {
	t.Parallel()

	called := false
	cmd := &cobra.Command{
		Use: "test",
		RunE: func(*cobra.Command, []string) error {
			called = true
			return nil
		},
	}
	installWebHandoff(cmd)
	cmd.SetArgs([]string{"-r", "session-1"})

	err := cmd.Execute()

	require.NoError(t, err)
	require.True(t, called)
}

func TestInstallWebHandoffPreservesExistingRun(t *testing.T) {
	t.Parallel()

	called := false
	cmd := &cobra.Command{
		Use: "test",
		RunE: func(*cobra.Command, []string) error {
			called = true
			return nil
		},
	}
	installWebHandoff(cmd)

	require.NoError(t, cmd.Execute())
	require.True(t, called, "existing RunE must stay in place without --web")
}

func TestInstallWebHandoffIsAdditive(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error {
		return nil
	}}
	cmd.Flags().BoolP("quiet", "q", false, "Hide spinner")

	installWebHandoff(cmd)

	require.NotNil(t, cmd.Flags().Lookup("quiet"))
	require.NotNil(t, cmd.Flags().Lookup(webFlag))
	require.NotNil(t, cmd.Flags().Lookup(resumeFlag))
}
