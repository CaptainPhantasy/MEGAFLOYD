package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/spf13/cobra"
)

func init() {
	var target, sessionID string
	var asJSON, describe bool
	command := &cobra.Command{
		Use: "handoff [web|tui|desktop]", Short: "Open the same workspace and session in another MegaFloyd interface", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selected := target
			if len(args) > 0 {
				if selected != "" && selected != args[0] {
					return fmt.Errorf("target flag and argument disagree")
				}
				selected = args[0]
			}
			if describe {
				selected = ""
			} else if selected != "web" && selected != "tui" && selected != "desktop" {
				return fmt.Errorf("choose handoff target web, tui, or desktop")
			}
			ws, cleanup, err := setupClientServerWorkspace(cmd)
			if err != nil {
				return err
			}
			defer cleanup()
			if sessionID != "" {
				sess, err := resolveWorkspaceSessionID(cmd.Context(), ws, sessionID)
				if err != nil {
					return err
				}
				sessionID = sess.ID
			}
			result, err := workspace.Handoff(cmd.Context(), ws, sessionID, selected)
			if err != nil {
				return err
			}
			if asJSON || describe {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			if result.WebURL != "" {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), result.WebURL)
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Opened %s for %s\n", result.Target, result.CWD)
			}
			return err
		},
	}
	command.Flags().StringVar(&target, "target", "", "Target interface: web, tui, or desktop")
	command.Flags().StringVarP(&sessionID, "session", "s", "", "Existing session to continue")
	command.Flags().BoolVar(&asJSON, "json", false, "Print the handoff descriptor as JSON")
	command.Flags().BoolVar(&describe, "describe", false, "Describe the current session without opening another interface")
	rootCmd.AddCommand(command)
}
