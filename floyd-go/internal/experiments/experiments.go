// Package experiments defines the config-backed experimental feature
// registry behind /experiments. Flags resolve with the same precedence as
// the floyd7 harness: per-flag environment override, then the
// [experimental] config section, then the master environment switch, then
// the flag's built-in default.
package experiments

import (
	"os"
	"slices"
	"strings"
)

const (
	// MasterEnv enables every flag when truthy.
	MasterEnv = "FLOYD_EXPERIMENTAL_FLAG"
	// EnvPrefix is prepended to the upper-cased flag id.
	EnvPrefix = "FLOYD_EXPERIMENTAL_"

	// GhostText gates inline AI suggestions and prompt-history completions.
	GhostText = "ghost_text"
)

// Flag is one registered experimental feature.
type Flag struct {
	ID          string
	Title       string
	Description string
	Default     bool
}

// Source reports where a flag's effective value came from.
type Source string

const (
	SourceEnv     Source = "env"
	SourceConfig  Source = "config"
	SourceMaster  Source = "master-env"
	SourceDefault Source = "default"
)

// State is a flag plus its resolved value and provenance.
type State struct {
	Flag
	Enabled bool
	Source  Source
}

var flags = []Flag{
	{
		ID:          GhostText,
		Title:       "Ghost-text suggestions",
		Description: "Suggest a follow-up with the small model after replies; use history while typing",
		Default:     true,
	},
}

// Flags returns a copy of the registry.
func Flags() []Flag {
	return slices.Clone(flags)
}

// Lookup resolves a flag id.
func Lookup(id string) (Flag, bool) {
	for _, flag := range flags {
		if flag.ID == id {
			return flag, true
		}
	}
	return Flag{}, false
}

// EnvName returns the per-flag environment variable for id.
func EnvName(id string) string {
	return EnvPrefix + strings.ToUpper(id)
}

// Enabled reports whether id is enabled under cfg.
func Enabled(cfg map[string]bool, id string) bool {
	_, enabled := resolve(cfg, id, os.Getenv)
	return enabled
}

// List returns every registered flag with its resolved value.
func List(cfg map[string]bool) []State {
	states := make([]State, 0, len(flags))
	for _, flag := range flags {
		source, enabled := resolve(cfg, flag.ID, os.Getenv)
		states = append(states, State{Flag: flag, Enabled: enabled, Source: source})
	}
	return states
}

// Set returns a copy of cfg with id set to enabled. It reports false for
// an unknown flag id.
func Set(cfg map[string]bool, id string, enabled bool) (map[string]bool, bool) {
	if _, ok := Lookup(id); !ok {
		return cfg, false
	}
	out := make(map[string]bool, len(cfg)+1)
	for key, value := range cfg {
		out[key] = value
	}
	out[id] = enabled
	return out, true
}

func resolve(cfg map[string]bool, id string, getenv func(string) string) (Source, bool) {
	flag, ok := Lookup(id)
	if !ok {
		return SourceDefault, false
	}
	if value, set := parseEnv(getenv(EnvName(id))); set {
		return SourceEnv, value
	}
	if cfg != nil {
		if value, set := cfg[id]; set {
			return SourceConfig, value
		}
	}
	if value, set := parseEnv(getenv(MasterEnv)); set && value {
		return SourceMaster, true
	}
	return SourceDefault, flag.Default
}

func parseEnv(value string) (bool, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false, false
	}
	switch value {
	case "0", "false", "no", "off", "disable", "disabled":
		return false, true
	default:
		return true, true
	}
}
