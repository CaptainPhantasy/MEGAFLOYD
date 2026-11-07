package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/legacy-ai/floyd/internal/megafloyd"
)

// Inspect reads the configured workspace without starting an agent, opening a
// database, refreshing provider catalogs, resolving credentials, or migrating
// files. Trusted shell configuration is evaluated, but hooks are never run.
func Inspect(ctx context.Context, workingDir, dataDir string) (*Config, []string, error) {
	paths := lookupConfigs(workingDir)
	cfg, loaded, err := loadFromConfigPaths(ctx, paths)
	if err != nil {
		return nil, nil, err
	}
	if megafloyd.IsActive() && dataDir == "" {
		dataDir, err = megafloyd.WorkspaceDataDir(workingDir)
		if err != nil {
			return nil, nil, err
		}
	}
	cfg.setDefaults(workingDir, dataDir)
	workspacePath := filepath.Join(cfg.Options.DataDirectory, appName+".json")
	if data, err := os.ReadFile(workspacePath); err == nil && len(data) > 0 {
		merged, err := loadFromBytes([][]byte{mustMarshalConfig(cfg), data})
		if err != nil {
			return nil, nil, fmt.Errorf("read workspace configuration %s: %w", workspacePath, err)
		}
		dataDir = cfg.Options.DataDirectory
		cfg = merged
		cfg.setDefaults(workingDir, dataDir)
		loaded = append(loaded, workspacePath)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err := cfg.ValidateHooks(); err != nil {
		return nil, nil, err
	}
	return cfg, loaded, nil
}
