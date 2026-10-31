package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/event"
	"github.com/legacy-ai/floyd/internal/ui/util"
)

type editorPreferencesState struct {
	normal  bool
	pending string
	undo    []string
	yank    string
}
type preferenceResultMsg struct {
	title, body, changed string
	err                  error
}

func (m *UI) preferencesConfig() *config.Config {
	if m == nil || m.com == nil || m.com.Workspace == nil {
		return nil
	}
	return m.com.Config()
}

func (m *UI) outputStyle() string {
	cfg := m.preferencesConfig()
	if cfg != nil && cfg.Options != nil && cfg.Options.TUI != nil && cfg.Options.TUI.OutputStyle == "plain" {
		return "plain"
	}
	return "markdown"
}

func (m *UI) vimEnabled() bool {
	cfg := m.preferencesConfig()
	return cfg != nil && cfg.Options != nil && cfg.Options.TUI != nil && cfg.Options.TUI.VimMode
}

func (m *UI) vimModeLabel() string {
	if !m.vimEnabled() {
		return ""
	}
	if m.editorPreferences.normal {
		return "VIM NORMAL · i insert · hjkl move · x delete · dd line · u undo"
	}
	return "VIM INSERT · Esc normal"
}

func (m *UI) applyTerminalPreferences() {
	profile := "standard"
	if cfg := m.preferencesConfig(); cfg != nil && cfg.Options != nil && cfg.Options.TUI != nil && cfg.Options.TUI.TerminalProfile == "multiline" {
		profile = "multiline"
	}
	if profile == "multiline" {
		m.keyMap.Editor.SendMessage = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "send"))
		m.keyMap.Editor.Newline = key.NewBinding(key.WithKeys("enter", "shift+enter", "ctrl+j"), key.WithHelp("enter", "newline"))
	} else {
		m.keyMap.Editor.SendMessage = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send"))
		m.keyMap.Editor.Newline = key.NewBinding(key.WithKeys("shift+enter", "ctrl+j"), key.WithHelp("ctrl+j", "newline"))
	}
}

func (m *UI) preferenceCommand(name, args string) tea.Cmd {
	args = strings.TrimSpace(args)
	ws := m.com.Workspace
	if name == "context" || name == "cost" {
		if m.session == nil {
			return m.showInfo(strings.Title(name), "No active session yet.")
		}
		id := m.session.ID
		return func() tea.Msg {
			s, err := ws.GetSession(context.Background(), id)
			if err != nil {
				return preferenceResultMsg{err: err}
			}
			cfg := ws.Config()
			selected := cfg.Models[config.SelectedModelTypeLarge]
			body := fmt.Sprintf("Session: %s\nPrompt tokens recorded: %d\nCompletion tokens recorded: %d\nRecorded cost: $%.6f\n", s.ID, s.PromptTokens, s.CompletionTokens, s.Cost)
			if s.EstimatedUsage {
				body += "Usage is estimated.\n"
			}
			if name == "context" {
				model := cfg.GetModelByType(config.SelectedModelTypeLarge)
				window := int64(0)
				if model != nil {
					window = int64(model.ContextWindow)
				}
				body += fmt.Sprintf("\nSelected model: %s/%s\nConfigured context window: %d tokens\nMessages recorded: %d\nInstruction files: %s\nAdditional directories: %s\n", selected.Provider, selected.Model, window, s.MessageCount, strings.Join(cfg.Options.ContextPaths, ", "), strings.Join(ws.AdditionalDirs(), ", "))
				body += "Recorded token totals are cumulative usage, not the remaining context window.\n"
			}
			return preferenceResultMsg{title: strings.Title(name), body: body}
		}
	}
	scope := config.ScopeWorkspace
	var field string
	var value any
	var body string
	switch name {
	case "vim":
		enabled := m.vimEnabled()
		switch args {
		case "":
			enabled = !enabled
		case "on":
			enabled = true
		case "off":
			enabled = false
		case "status":
			return m.showInfo("Vim", fmt.Sprintf("Enabled: %t\n%s", enabled, m.vimModeLabel()))
		default:
			return util.ReportWarn("Usage: /vim [on|off|status]")
		}
		field = "options.tui.vim_mode"
		value = enabled
		body = fmt.Sprintf("Vim editing: %t. Insert mode starts immediately. Escape enters normal mode; i/a/o insert, hjkl move, 0/$ line edges, x delete, dd delete line, u undo, p paste deleted line.", enabled)
	case "output-style":
		if args == "" || args == "status" {
			return m.showInfo("Output style", "Current: "+m.outputStyle()+"\nUse /output-style markdown or /output-style plain.")
		}
		if args != "plain" && args != "markdown" {
			return util.ReportWarn("Usage: /output-style markdown|plain")
		}
		field = "options.tui.output_style"
		value = args
		body = "Assistant text presentation: " + args
	case "terminal-setup":
		if args == "" {
			args = "standard"
		}
		if args != "standard" && args != "multiline" {
			return util.ReportWarn("Usage: /terminal-setup [standard|multiline]")
		}
		field = "options.tui.terminal_profile"
		value = args
		body = "Terminal profile: " + args + ". "
		if args == "multiline" {
			body += "Enter inserts a newline; Ctrl+D sends."
		} else {
			body += "Enter sends; Ctrl+J inserts a newline. Shift+Enter also inserts a newline in terminals that report it."
		}
		body += fmt.Sprintf("\nDetected terminal: %s\nSize: %d × %d", m.caps.TerminalVersion, m.width, m.height)
	case "privacy-settings":
		if args == "" || args == "status" {
			return m.showInfo("Privacy settings", fmt.Sprintf("Analytics disabled in configuration: %t\nAnalytics additionally requires an operator-provided telemetry key; DO_NOT_TRACK remains effective.\nModel requests use your selected provider. Session history stays in the configured workspace data directory (on the server in client/server mode); telemetry export requires an explicit command.\nUse /privacy-settings off to block new analytics or /privacy-settings on to permit configured analytics.", ws.Config().Options.DisableMetrics))
		}
		if args != "on" && args != "off" {
			return util.ReportWarn("Usage: /privacy-settings [status|on|off]")
		}
		scope = config.ScopeGlobal
		field = "options.disable_metrics"
		value = args == "off"
		body = "Analytics preference saved: " + args + ". An operator-provided telemetry key is still required; DO_NOT_TRACK remains effective."
	default:
		return util.ReportWarn("Unknown preference: " + name)
	}
	return func() tea.Msg {
		err := ws.SetConfigField(scope, field, value)
		if name == "privacy-settings" {
			// A workspace setting outranks global defaults. Persist the explicit
			// choice in both scopes so it remains effective after restarting.
			workspaceErr := ws.SetConfigField(config.ScopeWorkspace, field, value)
			err = errors.Join(err, workspaceErr)
			if workspaceErr == nil {
				event.SetPrivacyDisabled(value.(bool))
				if !value.(bool) {
					event.Init()
				}
			}
		}
		return preferenceResultMsg{title: strings.Title(name), body: body, changed: name, err: err}
	}
}

func (m *UI) applyPreferenceResult(msg preferenceResultMsg) tea.Cmd {
	if msg.err != nil {
		return util.ReportError(msg.err)
	}
	if msg.changed == "vim" {
		m.editorPreferences = editorPreferencesState{}
	}
	if msg.changed == "terminal-setup" {
		m.applyTerminalPreferences()
	}
	info := m.showInfo(msg.title, msg.body)
	if msg.changed == "output-style" && m.session != nil {
		return tea.Batch(info, m.loadSession(m.session.ID))
	}
	return info
}

func (m *UI) handleVimKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.vimEnabled() || m.focus != uiFocusEditor || m.activeInline != nil || (m.dialog != nil && m.dialog.HasDialogs()) {
		return false, nil
	}
	text := msg.String()
	if !m.editorPreferences.normal {
		if text == "esc" {
			m.editorPreferences.normal = true
			m.editorPreferences.pending = ""
			return true, nil
		}
		return false, nil
	}
	if strings.HasPrefix(text, "ctrl+") || strings.HasPrefix(text, "alt+") || text == "tab" || text == "enter" || text == "esc" {
		return false, nil
	}
	save := func() {
		m.editorPreferences.undo = append(m.editorPreferences.undo, m.textarea.Value())
		if len(m.editorPreferences.undo) > 100 {
			m.editorPreferences.undo = m.editorPreferences.undo[1:]
		}
	}
	update := func(code rune) { m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: code}) }
	prior := m.editorPreferences.pending
	m.editorPreferences.pending = ""
	switch text {
	case "i":
		m.editorPreferences.normal = false
	case "a":
		update(tea.KeyRight)
		m.editorPreferences.normal = false
	case "o":
		save()
		m.textarea.CursorEnd()
		m.textarea.InsertRune('\n')
		m.editorPreferences.normal = false
	case "h", "left":
		update(tea.KeyLeft)
	case "l", "right":
		update(tea.KeyRight)
	case "j", "down":
		m.textarea.CursorDown()
	case "k", "up":
		m.textarea.CursorUp()
	case "0", "home":
		m.textarea.CursorStart()
	case "$", "end":
		m.textarea.CursorEnd()
	case "x", "delete":
		save()
		update(tea.KeyDelete)
	case "d":
		if prior != "d" {
			m.editorPreferences.pending = "d"
			return true, nil
		}
		save()
		lines := strings.Split(m.textarea.Value(), "\n")
		row := m.textarea.Line()
		m.editorPreferences.yank = lines[row]
		lines = append(lines[:row], lines[row+1:]...)
		m.textarea.SetValue(strings.Join(lines, "\n"))
		m.textarea.MoveToBegin()
		for i := 0; i < row && i < len(lines); i++ {
			m.textarea.CursorDown()
		}
	case "u":
		n := len(m.editorPreferences.undo)
		if n > 0 {
			m.textarea.SetValue(m.editorPreferences.undo[n-1])
			m.editorPreferences.undo = m.editorPreferences.undo[:n-1]
		}
	case "p":
		save()
		m.textarea.CursorEnd()
		m.textarea.InsertString("\n" + m.editorPreferences.yank)
	case "esc":
	default:
		return true, nil
	}
	m.updateLayoutAndSize()
	return true, nil
}
