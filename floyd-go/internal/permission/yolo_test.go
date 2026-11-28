package permission

import "testing"

// TestYoloSelections pins which selections count as yolo for a check that needs
// a human answer. Manual is the only one that keeps asking. The selection is
// read each time, because the owner can change it.
func TestYoloSelections(t *testing.T) {
	if Yolo(nil) {
		t.Fatal("no permission service is not yolo")
	}
	manual := NewPermissionService(t.TempDir(), false, nil)
	if Yolo(manual) {
		t.Fatal("manual is not yolo")
	}
	// Skip requests is what --yolo, --dangerously-skip-permissions, ctrl+y and
	// "Never Ask" select.
	skipped := NewPermissionService(t.TempDir(), true, nil)
	if !Yolo(skipped) {
		t.Fatal("skip requests is yolo")
	}

	service := NewPermissionService(t.TempDir(), false, nil)
	mode := service.(ModeService)
	for _, tc := range []struct {
		mode string
		yolo bool
	}{
		{ModeYolo, true},
		{ModeManual, false},
		{ModeAuto, true},
		{ModeManual, false},
	} {
		if err := mode.SetMode(tc.mode); err != nil {
			t.Fatal(err)
		}
		if got := Yolo(service); got != tc.yolo {
			t.Errorf("after selecting %s: Yolo=%v, want %v", tc.mode, got, tc.yolo)
		}
	}
}
