// Package cmd provides CLI commands for Floyd.
package cmd

// GLMModelShortcut defines a shortcut flag that maps to a specific GLM model.
type GLMModelShortcut struct {
	Flag         string  // CLI flag (e.g., "47" for -47)
	ModelID      string  // Exact model ID for API (e.g., "glm-4.7")
	Description  string  // Help text
	Temperature  float64 // Default temperature (0 = use provider default)
	Reasoning    string  // "enabled", "disabled", or "auto" (empty = don't set)
	ClearThink   *bool   // Whether to clear reasoning content between turns
	SupportsTool bool    // Whether this model supports tool calling well
}

// GLMModelShortcuts maps CLI flags to GLM model configurations.
// Model IDs are EXACT as specified in Z.AI API documentation.
// See: https://z.ai/model-api
var GLMModelShortcuts = map[string]GLMModelShortcut{
	// ═══════════════════════════════════════════════════════════════════════════
	// GLM-5 Series (Latest flagship with thinking enabled by default)
	// ═══════════════════════════════════════════════════════════════════════════
	"5": {
		Flag:         "5",
		ModelID:      "glm-5",
		Description:  "GLM-5 flagship model (thinking enabled by default)",
		Temperature:  0.1,
		Reasoning:    "enabled", // Default, but explicit
		SupportsTool: true,
	},

	// ═══════════════════════════════════════════════════════════════════════════
	// GLM-4.7 Series (Latest generation)
	// ═══════════════════════════════════════════════════════════════════════════
	"47": {
		Flag:         "47",
		ModelID:      "glm-4.7",
		Description:  "GLM-4.7 (thinking enabled by default)",
		Temperature:  0.1,
		Reasoning:    "enabled",
		SupportsTool: true,
	},
	"47f": {
		Flag:         "47f",
		ModelID:      "glm-4.7-flash",
		Description:  "GLM-4.7 Flash (fast, thinking enabled)",
		Temperature:  0.2, // Slightly higher for creativity with fewer params
		Reasoning:    "enabled",
		SupportsTool: true,
	},
	"47x": {
		Flag:         "47x",
		ModelID:      "glm-4.7-flashx",
		Description:  "GLM-4.7 FlashX (fastest 4.7 variant)",
		Temperature:  0.2,
		Reasoning:    "enabled",
		SupportsTool: true,
	},

	// ═══════════════════════════════════════════════════════════════════════════
	// GLM-4.6 Series (Stable production series)
	// ═══════════════════════════════════════════════════════════════════════════
	"46": {
		Flag:         "46",
		ModelID:      "glm-4.6",
		Description:  "GLM-4.6 (auto-determines thinking)",
		Temperature:  0.1,
		Reasoning:    "", // Auto-determine (don't force)
		SupportsTool: true,
	},
	"46v": {
		Flag:         "46v",
		ModelID:      "glm-4.6v",
		Description:  "GLM-4.6V (vision-capable, auto thinking)",
		Temperature:  0.1,
		Reasoning:    "", // Auto-determine
		SupportsTool: true,
	},
	"46vf": {
		Flag:         "46vf",
		ModelID:      "glm-4.6v-flash",
		Description:  "GLM-4.6V Flash (fast vision model)",
		Temperature:  0.2,
		Reasoning:    "",
		SupportsTool: true,
	},
	"46vx": {
		Flag:         "46vx",
		ModelID:      "glm-4.6v-flashx",
		Description:  "GLM-4.6V FlashX (fastest vision variant)",
		Temperature:  0.2,
		Reasoning:    "",
		SupportsTool: true,
	},

	// ═══════════════════════════════════════════════════════════════════════════
	// GLM-4.5 Series (Mature series with many variants)
	// ═══════════════════════════════════════════════════════════════════════════
	"45": {
		Flag:         "45",
		ModelID:      "glm-4.5",
		Description:  "GLM-4.5 (mature production model)",
		Temperature:  0.1,
		Reasoning:    "", // Auto-determine
		SupportsTool: true,
	},
	"45v": {
		Flag:         "45v",
		ModelID:      "glm-4.5v",
		Description:  "GLM-4.5V (vision-capable)",
		Temperature:  0.1,
		Reasoning:    "",
		SupportsTool: true,
	},
	"45a": {
		Flag:         "45a",
		ModelID:      "glm-4.5-air",
		Description:  "GLM-4.5 Air (lightweight, fast)",
		Temperature:  0.1,
		Reasoning:    "disabled", // Air models typically don't think
		SupportsTool: true,
	},
	"45ax": {
		Flag:         "45ax",
		ModelID:      "glm-4.5-airx",
		Description:  "GLM-4.5 AirX (optimized air variant)",
		Temperature:  0.1,
		Reasoning:    "disabled",
		SupportsTool: true,
	},
	"45f": {
		Flag:         "45f",
		ModelID:      "glm-4.5-flash",
		Description:  "GLM-4.5 Flash (fastest 4.5 variant)",
		Temperature:  0.2,
		Reasoning:    "disabled",
		SupportsTool: true,
	},

	// ═══════════════════════════════════════════════════════════════════════════
	// Legacy / Special Models
	// ═══════════════════════════════════════════════════════════════════════════
	"4p": {
		Flag:         "4p",
		ModelID:      "glm-4-plus",
		Description:  "GLM-4 Plus (enhanced legacy, high concurrency)",
		Temperature:  0.1,
		Reasoning:    "disabled",
		SupportsTool: true,
	},
	"432": {
		Flag:         "432",
		ModelID:      "glm-4-32b-0414-128k",
		Description:  "GLM-4 32B (128K context, legacy)",
		Temperature:  0.1,
		Reasoning:    "disabled",
		SupportsTool: true,
	},
}

// GetGLMModelByFlag returns the GLM model configuration for a given flag.
// Returns empty struct and false if not found.
func GetGLMModelByFlag(flag string) (GLMModelShortcut, bool) {
	shortcut, ok := GLMModelShortcuts[flag]
	return shortcut, ok
}

// ListGLMModelFlags returns all available GLM model shortcut flags.
func ListGLMModelFlags() []string {
	flags := make([]string, 0, len(GLMModelShortcuts))
	for flag := range GLMModelShortcuts {
		flags = append(flags, flag)
	}
	return flags
}
