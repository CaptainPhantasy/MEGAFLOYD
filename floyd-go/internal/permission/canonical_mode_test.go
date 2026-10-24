package permission

import (
	"context"
	"testing"
	"time"
)

func TestCanonicalThreePermissionModes(t *testing.T) {
	p := NewPermissionService(t.TempDir(), false, nil)
	mode := p.(ModeService)
	if mode.Mode() != ModeManual {
		t.Fatal("initial policy is not manual")
	}
	if err := mode.SetMode(ModeYolo); err != nil {
		t.Fatal(err)
	}
	approved, err := p.Request(context.Background(), CreatePermissionRequest{SessionID: "test", ToolName: "bash", Params: map[string]any{"command": "git status --short"}})
	if err != nil || !approved || p.SkipRequests() {
		t.Fatalf("routine policy: approved=%v error=%v skip=%v", approved, err, p.SkipRequests())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	event := p.Subscribe(ctx)
	result := make(chan bool, 1)
	go func() {
		ok, _ := p.Request(ctx, CreatePermissionRequest{SessionID: "test", ToolName: "bash", Params: map[string]any{"command": "git reset --hard"}})
		result <- ok
	}()
	select {
	case req := <-event:
		if !p.Deny(req.Payload) {
			t.Fatal("pending request not denied")
		}
	case <-ctx.Done():
		t.Fatal("destructive command did not request permission")
	}
	if <-result {
		t.Fatal("denied command approved")
	}
	if err := mode.SetMode(ModeAuto); err != nil {
		t.Fatal(err)
	}
	if !p.SkipRequests() || mode.Mode() != ModeAuto {
		t.Fatal("explicit auto mode did not enable bypass")
	}
	p.SetSkipRequests(false)
	if mode.Mode() != ModeManual {
		t.Fatal("native normal mode did not restore manual policy")
	}
	if err := mode.SetMode("bogus"); err == nil || mode.Mode() != ModeManual {
		t.Fatal("invalid policy changed state")
	}
	for command, want := range map[string]bool{"pwd": true, "git diff --stat": true, "echo 'hello world'": true, "rm -rf .": false, "git diff --output=out": false, "echo $(hostname)": false, "cat ~/.ssh/id_ed25519": false, "pwd; rm file": false, "git branch delete-me": false, "rg --pre sh x": false} {
		if got := routineCommand(command); got != want {
			t.Errorf("%q: got %v want %v", command, got, want)
		}
	}
}
