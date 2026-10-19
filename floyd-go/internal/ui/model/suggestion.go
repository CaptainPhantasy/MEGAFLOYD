package model

import (
	"strings"

	"charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/clipboard"
	"github.com/legacy-ai/floyd/internal/experiments"
	"github.com/legacy-ai/floyd/internal/ui/util"
)

// TODO(port-wave4): the v5.3 line also sources AI-generated suggestions
// through AgentCoordinator.SuggestFollowup/SuggestPrompt. The chassis
// coordinator does not expose those methods yet, so suggestions here are
// history-based only; wire the AI source when the coordinator grows it.

// updateCommandSuggestion refreshes the inline ghost-text suggestion from
// prompt history. The suggestion is only shown while the editor is
// focused, no completion popup is open, and the editor content is a
// prefix of the candidate (or empty).
func (m *UI) updateCommandSuggestion() {
	if !m.experimentEnabled(experiments.GhostText) {
		m.commandSuggestion = ""
		return
	}
	if m.focus != uiFocusEditor || !m.textarea.Focused() {
		m.commandSuggestion = ""
		return
	}
	if m.completionsOpen {
		m.commandSuggestion = ""
		return
	}
	value := m.textarea.Value()
	// Multi-line suggestions are truncated to their first line while the
	// editor itself is single-line.
	if m.commandSuggestion != "" && strings.Contains(m.commandSuggestion, "\n") {
		firstLine := strings.Split(m.commandSuggestion, "\n")[0]
		if !strings.Contains(value, "\n") {
			m.commandSuggestion = firstLine
		}
		return
	}

	for i := len(m.promptHistory.messages) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(m.promptHistory.messages[i])
		if candidate == "" {
			continue
		}
		if value == "" {
			m.commandSuggestion = candidate
			return
		}
		if strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(value)) && !strings.EqualFold(candidate, value) {
			m.commandSuggestion = candidate
			return
		}
	}
	m.commandSuggestion = ""
}

// commandSuggestionSuffix returns the ghost-text portion of the current
// suggestion: the part extending beyond the editor value, shown only for
// single-line content with the caret on the first line.
func (m *UI) commandSuggestionSuffix() string {
	if m.commandSuggestion == "" || !m.textarea.Focused() {
		return ""
	}
	value := m.textarea.Value()
	if strings.Contains(value, "\n") {
		return ""
	}
	cur := m.textarea.Cursor()
	valueRunes := []rune(value)
	if cur == nil || cur.Y != 0 || cur.X > len(valueRunes) {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(m.commandSuggestion), strings.ToLower(value)) {
		return ""
	}
	suggestionRunes := []rune(m.commandSuggestion)
	if len(suggestionRunes) <= len(valueRunes) {
		return ""
	}
	return string(suggestionRunes[len(valueRunes):])
}

// acceptCommandSuggestion inserts the ghost-text suffix at the caret.
// It reports whether a suggestion was accepted.
func (m *UI) acceptCommandSuggestion() bool {
	if suffix := m.commandSuggestionSuffix(); suffix != "" {
		m.textarea.InsertString(suffix)
		m.textarea.MoveToEnd()
		m.updateCommandSuggestion()
		return true
	}
	return false
}

// stashRecallPrompt implements the stash toggle (alt+s). With text in the
// box, it shelves the draft into the stash (plus a clipboard copy) and
// clears the box so another command can be typed and sent. With an empty
// box and a non-empty stash, it restores the stashed text. With an empty
// box and nothing stashed, it reports that and does nothing.
func (m *UI) stashRecallPrompt() tea.Cmd {
	value := m.textarea.Value()
	if strings.TrimSpace(value) != "" {
		m.stashedPrompt = value
		m.textarea.Reset()
		m.updateCommandSuggestion()
		clipboard.WriteText(value)
		return util.ReportInfo("Prompt stashed — type away, alt+s recalls it.")
	}
	if m.stashedPrompt == "" {
		return util.ReportInfo("Nothing stashed.")
	}
	m.textarea.SetValue(m.stashedPrompt)
	m.textarea.MoveToEnd()
	m.updateCommandSuggestion()
	return nil
}

// renderEditorSuggestion appends the ghost-text suffix to the rendered
// editor view on the caret's line.
func (m *UI) renderEditorSuggestion(view string) string {
	suffix := m.commandSuggestionSuffix()
	if suffix == "" {
		return view
	}
	lines := strings.Split(view, "\n")
	lineIndex := m.textarea.Line()
	if lineIndex < 0 || lineIndex >= len(lines) {
		return view
	}
	lines[lineIndex] = lines[lineIndex] + m.com.Styles.Editor.GhostSuggestion.Render(suffix)
	return strings.Join(lines, "\n")
}
