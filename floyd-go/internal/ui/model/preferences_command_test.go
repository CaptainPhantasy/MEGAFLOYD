package model

import (
	"context"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/event"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/stretchr/testify/require"
)

type preferencesWorkspace struct {
	*builtinTestWorkspace
	storedSession session.Session
	scopes        []config.Scope
}

func (w *preferencesWorkspace) GetSession(context.Context, string) (session.Session, error) {
	return w.storedSession, nil
}

func (w *preferencesWorkspace) SetConfigField(scope config.Scope, key string, value any) error {
	w.scopes = append(w.scopes, scope)
	if err := w.builtinTestWorkspace.SetConfigField(scope, key, value); err != nil {
		return err
	}
	switch key {
	case "options.disable_metrics":
		w.cfg.Options.DisableMetrics = value.(bool)
	case "options.tui.vim_mode":
		w.cfg.Options.TUI.VimMode = value.(bool)
	case "options.tui.output_style":
		w.cfg.Options.TUI.OutputStyle = value.(string)
	case "options.tui.terminal_profile":
		w.cfg.Options.TUI.TerminalProfile = value.(string)
	}
	return nil
}

func TestPreferencesPersistAndApplyEditorProfiles(t *testing.T) {
	cfg := testConfigWithModel(4096)
	cfg.Options.TUI = &config.TUIOptions{}
	ui, _ := newBuiltinTestUI(cfg)
	ws := &preferencesWorkspace{builtinTestWorkspace: ui.com.Workspace.(*builtinTestWorkspace)}
	ui.com = common.DefaultCommon(ws)
	ui.textarea = textarea.New()
	ui.keyMap = DefaultKeyMap()
	cmd := ui.preferenceCommand("terminal-setup", "multiline")
	msg := cmd().(preferenceResultMsg)
	require.NoError(t, msg.err)
	ui.applyPreferenceResult(msg)
	require.Equal(t, "multiline", ws.setFields["options.tui.terminal_profile"])
	require.True(t, key.Matches(tea.KeyPressMsg{Code: tea.KeyEnter}, ui.keyMap.Editor.Newline))
	require.True(t, key.Matches(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, ui.keyMap.Editor.SendMessage))
	cmd = ui.preferenceCommand("vim", "on")
	msg = cmd().(preferenceResultMsg)
	require.NoError(t, msg.err)
	ui.applyPreferenceResult(msg)
	require.True(t, ui.vimEnabled())
	require.Contains(t, ui.vimModeLabel(), "INSERT")
	cmd = ui.preferenceCommand("output-style", "plain")
	msg = cmd().(preferenceResultMsg)
	require.NoError(t, msg.err)
	require.Equal(t, "plain", ui.outputStyle())
}

func TestVimEditsActualTextareaAndUndo(t *testing.T) {
	cfg := testConfigWithModel(4096)
	cfg.Options.TUI = &config.TUIOptions{VimMode: true}
	ui := newBusyUI(&countingWorkspace{})
	ui.com.Workspace = &testWorkspace{cfg: cfg}
	ui.textarea = textarea.New()
	ui.textarea.SetWidth(60)
	ui.textarea.Focus()
	ui.textarea.SetValue("abcd\nsecond")
	ui.textarea.MoveToBegin()
	ui.focus = uiFocusEditor
	press := func(code rune) { handled, _ := ui.handleVimKey(tea.KeyPressMsg{Code: code}); require.True(t, handled) }
	press(tea.KeyEscape)
	require.Contains(t, ui.vimModeLabel(), "NORMAL")
	press('l')
	press('x')
	require.Equal(t, "acd\nsecond", ui.textarea.Value())
	press('u')
	require.Equal(t, "abcd\nsecond", ui.textarea.Value())
	ui.textarea.MoveToBegin()
	press('d')
	press('d')
	require.Equal(t, "second", ui.textarea.Value())
	press('u')
	require.Equal(t, "abcd\nsecond", ui.textarea.Value())
	press('i')
	handled, _ := ui.handleVimKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	require.False(t, handled)
}

func TestContextAndCostUseLiveSessionEvidence(t *testing.T) {
	cfg := testConfigWithModel(4096)
	ui, overlay := newBuiltinTestUI(cfg)
	ws := &preferencesWorkspace{builtinTestWorkspace: ui.com.Workspace.(*builtinTestWorkspace), storedSession: session.Session{ID: "actual", PromptTokens: 123, CompletionTokens: 45, Cost: .25, MessageCount: 7}}
	ui.com = common.DefaultCommon(ws)
	ui.session = &session.Session{ID: "actual"}
	msg := ui.preferenceCommand("cost", "")().(preferenceResultMsg)
	require.NoError(t, msg.err)
	ui.applyPreferenceResult(msg)
	info := overlay.Dialog(dialog.InfoID).(*dialog.Info)
	require.Contains(t, info.Body(), "123")
	require.Contains(t, info.Body(), "0.250000")
}

func TestPreferencesSafeWithoutWorkspace(t *testing.T) {
	for _, ui := range []*UI{{}, {com: &common.Common{}}} {
		require.False(t, ui.vimEnabled())
		require.Equal(t, "markdown", ui.outputStyle())
		ui.applyTerminalPreferences()
		require.True(t, key.Matches(tea.KeyPressMsg{Code: tea.KeyEnter}, ui.keyMap.Editor.SendMessage))
	}
}

func TestPrivacyPreferenceOverridesWorkspaceAndPersistsDefault(t *testing.T) {
	previous := event.PrivacyDisabled()
	defer event.SetPrivacyDisabled(previous)
	cfg := testConfigWithModel(4096)
	ui, _ := newBuiltinTestUI(cfg)
	ws := &preferencesWorkspace{builtinTestWorkspace: ui.com.Workspace.(*builtinTestWorkspace)}
	ui.com = common.DefaultCommon(ws)
	msg := ui.preferenceCommand("privacy-settings", "off")().(preferenceResultMsg)
	require.NoError(t, msg.err)
	require.Equal(t, []config.Scope{config.ScopeGlobal, config.ScopeWorkspace}, ws.scopes)
	require.True(t, cfg.Options.DisableMetrics)
	require.True(t, event.PrivacyDisabled())
}
