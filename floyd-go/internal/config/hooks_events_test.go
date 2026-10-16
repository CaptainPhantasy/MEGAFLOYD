package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeHookEventCanonicalizesVariants(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"pretooluse":    "PreToolUse",
		"PreToolUse":    "PreToolUse",
		"pre_tool_use":  "PreToolUse",
		"POSTTOOLUSE":   "PostToolUse",
		"post_tool_use": "PostToolUse",
		"PostToolUse":   "PostToolUse",
		"sessionstart":  "SessionStart",
		"SESSION_START": "SessionStart",
		"SessionStart":  "SessionStart",
		"stop":          "Stop",
		"STOP":          "Stop",
		"runcomplete":   "Stop",
		"run_complete":  "Stop",
		"unknownevent":  "unknownevent", // unknown names pass through untouched
	}
	for in, want := range cases {
		require.Equal(t, want, normalizeHookEvent(in), "input %q", in)
	}
}

func TestValidateHooksMergesAliasEventKeys(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Hooks: map[string][]HookConfig{
			"pre_tool_use": {{Command: "echo pre"}},
			"posttooluse":  {{Command: "echo post"}},
			"run_complete": {{Command: "echo stop"}},
		},
	}
	require.NoError(t, cfg.ValidateHooks())
	require.Len(t, cfg.Hooks["PreToolUse"], 1)
	require.Len(t, cfg.Hooks["PostToolUse"], 1)
	require.Len(t, cfg.Hooks["Stop"], 1)
	_, hasAlias := cfg.Hooks["pre_tool_use"]
	require.False(t, hasAlias, "alias key must be folded into the canonical key")
}

func TestHeartbeatConfigDefaults(t *testing.T) {
	t.Parallel()
	hb := &HeartbeatConfig{Enabled: true}
	require.Equal(t, HeartbeatPrompt, hb.EffectivePrompt())
	require.Equal(t, DefaultMaxConsecutiveHeartbeats, hb.EffectiveMaxConsecutive())
	require.Positive(t, hb.Cooldown())

	custom := &HeartbeatConfig{
		Enabled:         true,
		Prompt:          "keep going",
		MaxConsecutive:  3,
		CooldownSeconds: 5,
	}
	require.Equal(t, "keep going", custom.EffectivePrompt())
	require.Equal(t, 3, custom.EffectiveMaxConsecutive())
	require.Equal(t, 5e9, float64(custom.Cooldown()))
}
