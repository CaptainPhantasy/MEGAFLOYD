package config

import (
	"encoding/json"
	"testing"
)

func TestModernRejectsAsyncPreTool(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"hooks":{"PreToolUse":[{"command":"true","async":true}]}}`), &c); err != nil {
		t.Fatal(err)
	}
	if err := c.ValidateHooks(); err == nil {
		t.Fatal("async pre-tool hook must not be accepted as a decision check")
	}
}

func TestModernHookAliasesAndDeterministicOrder(t *testing.T) {
	c := Config{Hooks: map[string][]HookConfig{
		"pre_tool_use": {{Command: "lower"}}, "PRETOOLUSE": {{Command: "upper"}}, "PreToolUse": {{Command: "canonical"}},
		"tool_failure": {{Command: "failure"}}, "tool_canceled": {{Command: "cancel"}},
	}}
	if err := c.ValidateHooks(); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"canonical", "upper", "lower"} {
		if c.Hooks["PreToolUse"][i].Command != want {
			t.Fatalf("wrong order: %+v", c.Hooks)
		}
	}
	if len(c.Hooks["PostToolUseFailure"]) != 1 || len(c.Hooks["ToolCancelled"]) != 1 {
		t.Fatal(c.Hooks)
	}
	if err := c.ValidateHooks(); err != nil {
		t.Fatal(err)
	}
	if len(c.Hooks["PreToolUse"]) != 3 {
		t.Fatal("validation duplicated aliases")
	}
}

func TestModernHookModeValidation(t *testing.T) {
	for _, tc := range []struct {
		event string
		h     HookConfig
	}{
		{"PostToolUse", HookConfig{Required: true, Async: true}},
		{"PostToolUse", HookConfig{Sequential: true, Async: true}},
		{"Stop", HookConfig{Required: true}},
		{"SessionStart", HookConfig{Async: true}},
		{"Unknown", HookConfig{Sequential: true}},
	} {
		tc.h.Command = "true"
		c := Config{Hooks: map[string][]HookConfig{tc.event: {tc.h}}}
		if c.ValidateHooks() == nil {
			t.Fatalf("accepted invalid configuration: %+v", tc)
		}
	}
}
