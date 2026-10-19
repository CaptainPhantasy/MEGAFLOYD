// Package skills embeds the megafloyd slash-command skills and installs them
// into the data directory's commands folder so the command loader can find
// them at runtime.
package skills

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed data/*.md
var commandData embed.FS

//go:embed data/skills.json
var registryData []byte

type registry struct {
	Skills []registryEntry `json:"skills"`
}

type registryEntry struct {
	Name    string   `json:"name"`
	File    string   `json:"file"`
	Aliases []string `json:"aliases"`
	AliasOf string   `json:"alias_of"`
}

// InstallCommands writes every embedded skill command into
// <dataDir>/commands. Files are rewritten only when their content differs, so
// repeated calls are idempotent. Aliases declared in skills.json are written
// as copies of their target file because the markdown loader resolves
// commands by file name only.
func InstallCommands(dataDir string) error {
	if dataDir == "" {
		return errors.New("skills: data directory is empty")
	}

	commands, err := commandFiles()
	if err != nil {
		return err
	}
	aliases, err := aliasFiles(commands)
	if err != nil {
		return err
	}

	commandsDir := filepath.Join(dataDir, "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		return fmt.Errorf("skills: create commands directory: %w", err)
	}

	for _, name := range sortedKeys(commands) {
		if err := writeIfChanged(filepath.Join(commandsDir, name), commands[name]); err != nil {
			return fmt.Errorf("skills: install %s: %w", name, err)
		}
	}
	for _, name := range sortedKeys(aliases) {
		content, ok := commands[aliases[name]]
		if !ok {
			return fmt.Errorf("skills: alias %s targets missing file %s", name, aliases[name])
		}
		if err := writeIfChanged(filepath.Join(commandsDir, name), content); err != nil {
			return fmt.Errorf("skills: install alias %s: %w", name, err)
		}
	}

	return nil
}

// commandFiles returns the embedded command markdown keyed by file name.
func commandFiles() (map[string][]byte, error) {
	entries, err := fs.ReadDir(commandData, "data")
	if err != nil {
		return nil, fmt.Errorf("skills: read embedded commands: %w", err)
	}

	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := commandData.ReadFile("data/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("skills: read embedded command %s: %w", entry.Name(), err)
		}
		files[entry.Name()] = content
	}

	return files, nil
}

// aliasFiles maps alias file names to the command file they copy. Aliases
// come from both the "aliases" lists and the "alias_of" entries in
// skills.json; duplicates collapse to one alias file.
func aliasFiles(commands map[string][]byte) (map[string]string, error) {
	var reg registry
	if err := json.Unmarshal(registryData, &reg); err != nil {
		return nil, fmt.Errorf("skills: parse skills.json: %w", err)
	}

	aliases := make(map[string]string)
	add := func(name, file string) error {
		if name == "" || file == "" {
			return nil
		}
		if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
			return fmt.Errorf("skills: invalid alias name %q", name)
		}
		if _, ok := commands[file]; !ok {
			return fmt.Errorf("skills: alias %s targets unknown file %s", name, file)
		}

		aliasFile := name + ".md"
		if _, ok := commands[aliasFile]; ok {
			// The alias already exists as a real skill file.
			return nil
		}
		aliases[aliasFile] = file
		return nil
	}

	for _, entry := range reg.Skills {
		for _, name := range entry.Aliases {
			if err := add(name, entry.File); err != nil {
				return nil, err
			}
		}
		if entry.AliasOf != "" {
			if err := add(entry.Name, entry.File); err != nil {
				return nil, err
			}
		}
	}

	return aliases, nil
}

// writeIfChanged writes content only when the destination is missing or has
// different bytes.
func writeIfChanged(path string, content []byte) error {
	current, err := os.ReadFile(path)
	if err == nil && bytes.Equal(current, content) {
		return nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
