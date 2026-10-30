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

// manifestName records which command files this installer owns. It lives in
// the commands directory so a moved or copied data directory keeps its claim
// list with the files it describes; the loader ignores it because it is not
// markdown.
const manifestName = ".megafloyd-skills-manifest"

// manifestHeader explains the file to anyone who finds it.
const manifestHeader = "# Files installed by MegaFloyd. Retired entries are removed on the next install."

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

	desired := make(map[string][]byte, len(commands)+len(aliases))
	for name, content := range commands {
		desired[name] = content
	}
	for name, target := range aliases {
		content, ok := commands[target]
		if !ok {
			return fmt.Errorf("skills: alias %s targets missing file %s", name, target)
		}
		desired[name] = content
	}

	return installFiles(dataDir, desired)
}

// installFiles makes <dataDir>/commands contain desired, then removes the
// files a previous install claimed and this one no longer provides. Removing
// only manifest-claimed names is what keeps user-authored and foreign files
// safe: a file this installer never wrote is never touched.
func installFiles(dataDir string, desired map[string][]byte) error {
	commandsDir := filepath.Join(dataDir, "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		return fmt.Errorf("skills: create commands directory: %w", err)
	}

	previous, err := readManifest(commandsDir)
	if err != nil {
		return err
	}

	// Write before pruning so a failed install never leaves the manifest
	// claiming files that were never written.
	for _, name := range sortedKeys(desired) {
		if err := writeIfChanged(filepath.Join(commandsDir, name), desired[name]); err != nil {
			return fmt.Errorf("skills: install %s: %w", name, err)
		}
	}

	for _, name := range previous {
		if _, current := desired[name]; current {
			continue
		}
		if err := removeInstalled(commandsDir, name); err != nil {
			return fmt.Errorf("skills: prune %s: %w", name, err)
		}
	}

	return writeManifest(commandsDir, sortedKeys(desired))
}

// readManifest returns the names the previous install claimed. A missing
// manifest means nothing is claimed yet, so nothing may be pruned.
func readManifest(commandsDir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(commandsDir, manifestName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skills: read manifest: %w", err)
	}

	var names []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}
		if !validCommandName(name) {
			// A corrupt or hand-edited entry must never become a delete
			// path outside the commands directory.
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}

// writeManifest records the current claim list, sorted and unchanged-aware so
// repeated installs do not rewrite the file.
func writeManifest(commandsDir string, names []string) error {
	var b strings.Builder
	b.WriteString(manifestHeader)
	b.WriteString("\n")
	for _, name := range names {
		b.WriteString(name)
		b.WriteString("\n")
	}
	if err := writeIfChanged(filepath.Join(commandsDir, manifestName), []byte(b.String())); err != nil {
		return fmt.Errorf("skills: write manifest: %w", err)
	}
	return nil
}

// removeInstalled deletes one claimed command file. Only regular files are
// removed: a directory (or symlink) that appeared at a claimed name belongs to
// someone else and is left alone. A file that is already gone is not an error.
func removeInstalled(commandsDir, name string) error {
	path := filepath.Join(commandsDir, name)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// validCommandName accepts a plain file name with no path separators.
func validCommandName(name string) bool {
	if name == "." || name == ".." || name == manifestName {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
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
