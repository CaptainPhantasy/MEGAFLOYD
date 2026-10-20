package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/legacy-ai/floyd/internal/enginebridge"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
)

var (
	sessionEnginesJSON bool
	sessionImportJSON  bool
)

var sessionEnginesCmd = &cobra.Command{
	Use:     "engines",
	Aliases: []string{"engine", "engine-list"},
	Short:   "List sessions from the desktop/web engine",
	Long:    "List sessions stored by the Floyd Code engine (used by the desktop app). Use --json for machine-readable output.",
	RunE:    runSessionEngines,
}

var sessionImportCmd = &cobra.Command{
	Use:   "import <engine-session-id>",
	Short: "Import a desktop/web engine session into the CLI",
	Long:  "Copy an engine session into the local database so the CLI can show and resume it. The engine's session ID is kept, so importing twice never duplicates a transcript.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSessionImport,
}

func init() {
	sessionEnginesCmd.Flags().BoolVar(&sessionEnginesJSON, "json", false, "output in JSON format")
	sessionImportCmd.Flags().BoolVar(&sessionImportJSON, "json", false, "output in JSON format")
	sessionCmd.AddCommand(sessionEnginesCmd)
	sessionCmd.AddCommand(sessionImportCmd)
}

type engineSessionJSON struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	WorkDir string `json:"workdir"`
	Updated string `json:"updated"`
	Source  string `json:"source"`
}

func runSessionEngines(cmd *cobra.Command, _ []string) error {
	home := enginebridge.Home()
	sessions, err := enginebridge.List(home)
	if err != nil {
		return fmt.Errorf("list engine sessions: %w", err)
	}

	if sessionEnginesJSON {
		output := make([]engineSessionJSON, len(sessions))
		for i, item := range sessions {
			updated := ""
			if !item.UpdatedAt.IsZero() {
				updated = item.UpdatedAt.Format(time.RFC3339)
			}
			output[i] = engineSessionJSON{
				ID:      item.ID,
				Title:   item.Title,
				WorkDir: item.WorkDir,
				Updated: updated,
				Source:  "engine",
			}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetEscapeHTML(false)
		return enc.Encode(output)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Engine home: %s\n", home)
	for _, item := range sessions {
		updated := "-"
		if !item.UpdatedAt.IsZero() {
			updated = item.UpdatedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(out, "%s  %s  %s\n", updated, item.ID, item.Title)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(out, "No engine sessions found.")
	}
	return nil
}

func runSessionImport(cmd *cobra.Command, args []string) error {
	ctx, svc, cleanup, err := sessionSetup(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	home := enginebridge.Home()
	found, err := enginebridge.Find(home, args[0])
	if err != nil {
		return err
	}
	transcript, err := enginebridge.ReadTranscript(found.Dir)
	if err != nil {
		return err
	}
	imported := make([]session.ImportMessage, 0, len(transcript))
	for _, item := range transcript {
		parts, err := message.MarshalParts(item.Parts)
		if err != nil {
			return fmt.Errorf("marshal transcript parts: %w", err)
		}
		imported = append(imported, session.ImportMessage{
			Role:      item.Role,
			Parts:     parts,
			CreatedAt: item.CreatedAt,
		})
	}

	result, err := session.ImportEngine(ctx, svc.sessions, found.ID, found.Title, imported)
	if err != nil {
		return fmt.Errorf("import engine session: %w", err)
	}

	if sessionImportJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetEscapeHTML(false)
		return enc.Encode(map[string]any{
			"id":       result.Session.ID,
			"title":    result.Session.Title,
			"copied":   result.Copied,
			"existing": result.Existing,
		})
	}
	if result.Existing {
		fmt.Fprintf(cmd.OutOrStdout(), "Session already imported: %s\n", result.Session.ID)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Imported %s (%d messages)\n", result.Session.ID, result.Copied)
	return nil
}
