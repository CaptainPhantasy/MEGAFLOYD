package config

import (
	"charm.land/catwalk/pkg/catwalk"
	"encoding/json"
	"github.com/qjebbs/go-jsons"
	"slices"
	"strings"
)

// persistedModelSelection keeps explicit resets when workspace configuration
// overlays a different globally selected model.
type persistedModelSelection struct {
	SelectedModel
	MaxTokens       int64          `json:"max_tokens"`
	ReasoningEffort string         `json:"reasoning_effort"`
	Think           bool           `json:"think"`
	ProviderOptions map[string]any `json:"provider_options"`
}

func modelSelectionForChange(c *Config, kind SelectedModelType, selected SelectedModel) SelectedModel {
	if c.Providers == nil {
		return selected
	}
	model := c.GetModel(selected.Provider, selected.Model)
	if model == nil {
		return selected
	}
	previous := c.Models[kind]
	if previous.Provider != selected.Provider || previous.Model != selected.Model {
		selected.ProviderOptions = nil
		selected.MaxTokens = model.DefaultMaxTokens
		selected.ReasoningEffort = ""
		selected.Think = model.CanReason && len(model.ReasoningLevels) == 0
		if model.CanReason {
			selected.ReasoningEffort = preferredModelReasoning(model.ReasoningLevels, model.DefaultReasoningEffort)
		}
	} else if model.CanReason && len(model.ReasoningLevels) == 0 && previous.Think != selected.Think {
		// A native boolean toggle needs an explicit off marker across reloads.
		selected.ReasoningEffort = ""
		if !selected.Think {
			selected.ReasoningEffort = "none"
		}
	}
	if !model.CanReason {
		selected.ReasoningEffort = ""
		selected.Think = false
	}
	return selected
}

func preferredModelReasoning(levels []string, fallback string) string {
	// High is the requested default. If unavailable, use the greatest
	// advertised named level; never invent an unsupported provider value.
	for _, level := range []string{"high", "max", "xhigh", "medium", "low", "minimal"} {
		if slices.Contains(levels, level) {
			return level
		}
	}
	if fallback != "none" && fallback != "off" && slices.Contains(levels, fallback) {
		return fallback
	}
	for i := len(levels) - 1; i >= 0; i-- {
		if levels[i] != "none" && levels[i] != "off" && levels[i] != "disabled" {
			return levels[i]
		}
	}
	return ""
}

// DefaultReasoningEffort selects only an effort advertised by this model.
func DefaultReasoningEffort(model catwalk.Model) string {
	if !model.CanReason {
		return ""
	}
	return preferredModelReasoning(model.ReasoningLevels, model.DefaultReasoningEffort)
}

// The general JSON merger treats null object values as absent. A model's
// explicit provider_options:null means reset, including across restarts.
func restoreModelOptionResets(c *Config, configs [][]byte) error {
	type layer struct {
		Models map[SelectedModelType]struct {
			Options json.RawMessage `json:"provider_options"`
		} `json:"models"`
	}
	layers := make([]layer, len(configs))
	resets := map[SelectedModelType]int{}
	for i, data := range configs {
		if err := json.Unmarshal(data, &layers[i]); err != nil {
			return err
		}
		for kind, model := range layers[i].Models {
			if strings.TrimSpace(string(model.Options)) == "null" {
				resets[kind] = i
			}
		}
	}
	for kind, index := range resets {
		pieces := [][]byte{[]byte("{}")}
		for _, layer := range layers[index+1:] {
			if value := layer.Models[kind].Options; len(value) > 0 {
				pieces = append(pieces, value)
			}
		}
		var options map[string]any
		if len(pieces) > 1 {
			data, err := jsons.Merge(pieces)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(data, &options); err != nil {
				return err
			}
		}
		selected := c.Models[kind]
		selected.ProviderOptions = options
		c.Models[kind] = selected
	}
	return nil
}
