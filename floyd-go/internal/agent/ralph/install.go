package ralph

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed commands/ralph.md
var ralphCommandMD []byte

//go:embed commands/ralph-cancel.md
var ralphCancelMD []byte

//go:embed commands/ralph-status.md
var ralphStatusMD []byte

// InstallCommands writes the ralph loop slash commands into the .floyd/commands
// directory so they appear in the command palette. Idempotent — overwrites
// existing files to keep them up to date.
func InstallCommands(dataDir string) error {
	commandsDir := filepath.Join(dataDir, "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		return err
	}

	commands := map[string][]byte{
		"ralph.md":        ralphCommandMD,
		"ralph-cancel.md": ralphCancelMD,
		"ralph-status.md": ralphStatusMD,
	}

	for name, content := range commands {
		if err := os.WriteFile(filepath.Join(commandsDir, name), content, 0o644); err != nil {
			return err
		}
	}

	return nil
}
