package hooks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/legacy-ai/floyd/internal/config"
)

func TestModernRequiredFailureBlocks(t *testing.T) {
	var h config.HookConfig
	if err := json.Unmarshal([]byte(`{"command":"exit 7","required":true}`), &h); err != nil {
		t.Fatal(err)
	}
	got, err := NewRunner([]config.HookConfig{h}, t.TempDir(), t.TempDir()).Run(context.Background(), EventPreToolUse, "required-session", "bash", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Decision != DecisionDeny {
		t.Fatalf("required command failure must block; got %s", got.Decision)
	}
}

func TestModernPayloadHasNativeIdentity(t *testing.T) {
	var p map[string]any
	if err := json.Unmarshal(BuildPayload(EventPreToolUse, "sid", "/tmp", "bash", `{"event_id":"untrusted"}`), &p); err != nil {
		t.Fatal(err)
	}
	if p["version"] != float64(1) || p["event_id"] == nil || p["timestamp"] == nil {
		t.Fatalf("versioned native event identity missing: version=%v event_id=%v timestamp=%v", p["version"], p["event_id"], p["timestamp"])
	}
	if p["event_id"] == "untrusted" {
		t.Fatal("tool input replaced trusted event identity")
	}
}
