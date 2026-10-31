package config

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tidwall/sjson"
)

// EditField validates and writes one configuration field with the same atomic
// lock used by the runtime, without contacting providers or starting tools.
func EditField(ctx context.Context, cwd, dataDir string, scope Scope, key string, value any) (string, error) {
	if key == "" || strings.ContainsAny(key, "#*?[]") {
		return "", fmt.Errorf("expected a dotted configuration key")
	}
	cfg, _, err := Inspect(ctx, cwd, dataDir)
	if err != nil {
		return "", err
	}
	store := &ConfigStore{config: cfg, workingDir: cwd, globalDataPath: GlobalConfigData(), workspacePath: filepath.Join(cfg.Options.DataDirectory, appName+".json")}
	path, err := store.configPath(scope)
	if err != nil {
		return "", err
	}
	err = store.atomicWrite(scope, func(data []byte) ([]byte, error) {
		updated, err := sjson.SetBytes(data, key, value)
		if err != nil {
			return nil, err
		}
		var validated Config
		decoder := json.NewDecoder(strings.NewReader(string(updated)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&validated); err != nil {
			return nil, fmt.Errorf("invalid configuration: %w", err)
		}
		if err := validated.ValidateHooks(); err != nil {
			return nil, err
		}
		return updated, nil
	})
	return path, err
}
