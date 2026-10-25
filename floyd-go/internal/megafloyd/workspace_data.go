package megafloyd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WorkspaceDataDir keeps conversation state in the caller's own home. The
// private registry preserves deliberately selected existing stores without
// discovering another user's state in a shared project directory.
func WorkspaceDataDir(workingDir string) (string, error) {
	home, err := HomeDir()
	if err != nil {
		return "", err
	}
	cwd, err := filepath.Abs(workingDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve workspace %s: %w", workingDir, err)
	}
	registryPath := filepath.Join(home, "state", "workspace-data.json")
	data, err := os.ReadFile(registryPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read private workspace registry: %w", err)
	}
	if err == nil {
		var registry struct {
			Version    int               `json:"version"`
			Workspaces map[string]string `json:"workspaces"`
		}
		if err := json.Unmarshal(data, &registry); err != nil {
			return "", fmt.Errorf("read private workspace registry: %w", err)
		}
		if registry.Version != 1 {
			return "", fmt.Errorf("unsupported private workspace registry version: %d", registry.Version)
		}
		if selected := registry.Workspaces[cwd]; selected != "" {
			if !filepath.IsAbs(selected) {
				return "", fmt.Errorf("private workspace data path must be absolute: %s", selected)
			}
			return filepath.Clean(selected), nil
		}
	}
	digest := sha256.Sum256([]byte(cwd))
	dataDir := filepath.Join(DataDir(home), "workspaces", hex.EncodeToString(digest[:]))
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("create personal workspace data: %w", err)
	}
	return dataDir, nil
}
