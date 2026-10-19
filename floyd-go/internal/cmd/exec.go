package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/legacy-ai/floyd/internal/execution"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <command>",
	Short: "Execute a shell command with safety checks",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		command := strings.Join(args, " ")
		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		// TODO(port-wave4): the chassis config has no options.execution
		// surface yet; flags are the only source for now.
		execInputs, err := resolveExecutionConfig(cmd, nil)
		if err != nil {
			return err
		}
		captureStderr, _ := cmd.Flags().GetBool("stderr")

		env := execution.NewEnvironment(execution.Config{
			Shell:           execInputs.Shell,
			DefaultTimeout:  execInputs.Timeout,
			MaxBufferSize:   execInputs.MaxBufferBytes,
			AllowedPrefixes: execInputs.AllowedPrefix,
			AllowedPatterns: execInputs.AllowedRegex,
			DeniedPrefixes:  execInputs.DeniedPrefix,
			DeniedPatterns:  execInputs.DeniedRegex,
		})
		if err := env.Initialize(context.Background()); err != nil {
			return err
		}

		options := execution.Options{
			Cwd:           cwd,
			Shell:         execInputs.Shell,
			Timeout:       execInputs.Timeout,
			Env:           execInputs.Env,
			MaxBufferSize: execInputs.MaxBufferBytes,
			CaptureStderr: captureStderr,
		}
		result, err := env.Execute(context.Background(), command, options)
		if err != nil {
			return err
		}

		if result.Output != "" {
			cmd.Print(result.Output)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("command failed with exit code %d", result.ExitCode)
		}
		return nil
	},
}

func init() {
	execCmd.PersistentFlags().Duration("timeout", 0, "Execution timeout (0 uses config default)")
	execCmd.PersistentFlags().String("shell", "", "Shell to use (defaults to system shell)")
	execCmd.PersistentFlags().Int("max-buffer", 0, "Maximum output size in bytes (0 uses config default)")
	execCmd.PersistentFlags().Bool("stderr", false, "Include stderr in output")
	execCmd.PersistentFlags().StringArray("allow-prefix", nil, "Allowed command prefix (repeatable)")
	execCmd.PersistentFlags().StringArray("allow-regex", nil, "Allowed command regex (repeatable)")
	execCmd.PersistentFlags().StringArray("deny-prefix", nil, "Denied command prefix (repeatable)")
	execCmd.PersistentFlags().StringArray("deny-regex", nil, "Denied command regex (repeatable)")
	execCmd.PersistentFlags().StringArray("env", nil, "Environment variable override, e.g. --env KEY=value")
	execCmd.PersistentFlags().String("cwd", "", "Working directory")
}
