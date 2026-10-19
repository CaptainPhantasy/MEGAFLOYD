package model

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

func testStashUI() *UI {
	ta := textarea.New()
	ta.Focus()
	return &UI{textarea: ta, focus: uiFocusEditor, localUserMessages: make(map[string]int64)}
}

func TestStashShelvesText(t *testing.T) {
	m := testStashUI()
	m.textarea.SetValue("draft command here")
	m.stashRecallPrompt()
	if got := m.textarea.Value(); got != "" {
		t.Fatalf("box not cleared, got %q", got)
	}
	if m.stashedPrompt != "draft command here" {
		t.Fatalf("stash missing, got %q", m.stashedPrompt)
	}
}

func TestStashRecallsIntoEmptyBox(t *testing.T) {
	m := testStashUI()
	m.stashedPrompt = "stashed command here"
	m.stashRecallPrompt()
	if got := m.textarea.Value(); got != "stashed command here" {
		t.Fatalf("not recalled, got %q", got)
	}
}

func TestStashEmptyBoxNothingStashedIsNoop(t *testing.T) {
	m := testStashUI()
	if cmd := m.stashRecallPrompt(); cmd == nil {
		t.Fatalf("expected info message, got nil cmd")
	}
	if got := m.textarea.Value(); got != "" {
		t.Fatalf("box changed, got %q", got)
	}
}

func TestStashKeyMatches(t *testing.T) {
	km := DefaultKeyMap()
	altS := tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt}
	if !key.Matches(altS, km.Editor.StashPrompt) {
		t.Fatalf("alt+s does not match StashPrompt binding")
	}
	// alt+s must not fire suggestion accept or any other editor action.
	if key.Matches(altS, km.Editor.AcceptSuggestion) {
		t.Fatalf("alt+s collides with AcceptSuggestion")
	}
}

func TestAcceptSuggestionBindingIsModified(t *testing.T) {
	km := DefaultKeyMap()
	// The accept binding must stay a modified (non-printable-alone) key:
	// a bare printable character would leak into the prompt text whenever
	// no suggestion is present.
	for _, k := range km.Editor.AcceptSuggestion.Keys() {
		if len(k) == 1 {
			t.Fatalf("AcceptSuggestion must not bind a bare printable key, got %q", k)
		}
	}
}
