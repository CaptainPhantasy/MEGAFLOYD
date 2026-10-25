package chat

import "testing"

func TestCanonicalToolDisplayPreservesProtocolNames(t *testing.T) {
	for raw, want := range map[string]string{"floyd_info": "MegaFloyd Info", "floyd_logs": "MegaFloyd Logs", "user_floyd_tool": "User Floyd Tool"} {
		if got := humanizedToolName(raw); got != want {
			t.Fatalf("%s: %q, want %q", raw, got, want)
		}
	}
}
