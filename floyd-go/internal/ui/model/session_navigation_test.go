package model

import (
	"context"
	"errors"
	"testing"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/stretchr/testify/require"
)

type navigationWorkspace struct {
	*countingWorkspace
	requested []string
	presence  []string
	getError  error
}

func (w *navigationWorkspace) GetSession(_ context.Context, id string) (session.Session, error) {
	w.requested = append(w.requested, id)
	return session.Session{ID: id, Title: id}, w.getError
}

func (*navigationWorkspace) ListSessionHistory(context.Context, string) ([]history.File, error) {
	return nil, nil
}

func (*navigationWorkspace) FileTrackerListReadFiles(context.Context, string) ([]string, error) {
	return nil, nil
}

func (w *navigationWorkspace) SetCurrentSession(_ context.Context, id string) error {
	w.presence = append(w.presence, id)
	return nil
}
func (*navigationWorkspace) Config() *config.Config { return testConfigWithModel(1000) }

func newNavigationUI() (*UI, *navigationWorkspace) {
	w := &navigationWorkspace{countingWorkspace: &countingWorkspace{ready: true}}
	m := newBusyUI(w.countingWorkspace)
	m.com.Workspace = w
	warmCaches(m, false)
	return m, w
}

func TestSessionLoadKeepsOriginalWorkspaceAndRejectsStaleReply(t *testing.T) {
	m, original := newNavigationUI()
	load := m.loadSession("original-session")
	other := &navigationWorkspace{countingWorkspace: &countingWorkspace{ready: true}}
	m.com.Workspace = other
	m.session = &session.Session{ID: "other-session"}
	loaded := load().(loadSessionMsg)
	require.Equal(t, []string{"original-session"}, original.requested)
	require.Empty(t, other.requested)
	m.Update(loaded)
	require.Equal(t, "other-session", m.session.ID)
}

func TestSessionLoadLatestSelectionWinsOutOfOrderReplies(t *testing.T) {
	m, _ := newNavigationUI()
	older := m.loadSession("older")
	newer := m.loadSession("newer")
	m.Update(newer())
	require.Equal(t, "newer", m.session.ID)
	m.Update(older())
	require.Equal(t, "newer", m.session.ID, "late replies cannot replace the selected conversation")
}

func TestSessionLoadCannotUndoNewConversation(t *testing.T) {
	m, _ := newNavigationUI()
	load := m.loadSession("older")
	m.newSession()
	require.Nil(t, m.session)
	m.Update(load())
	require.Nil(t, m.session)
	require.Equal(t, uiLanding, m.state)
}

func TestSessionLoadErrorIsVisibleOnlyForCurrentRequest(t *testing.T) {
	m, w := newNavigationUI()
	w.getError = errors.New("fixture session unavailable")
	load := m.loadSession("missing")
	result := load()
	_, report := m.Update(result)
	outputs := executeInitializationCommands(report)
	found := false
	for _, output := range outputs {
		if info, ok := output.(util.InfoMsg); ok && info.Type == util.InfoTypeError {
			found = true
			require.Contains(t, info.Msg, "fixture session unavailable")
		}
	}
	require.True(t, found, "returning a tea.Cmd as a message silently discards load errors")
	m.loadSession("newer")
	_, stale := m.Update(result)
	for _, output := range executeInitializationCommands(stale) {
		if info, ok := output.(util.InfoMsg); ok {
			require.NotContains(t, info.Msg, "fixture session unavailable")
		}
	}
}

func TestSessionPresenceUsesCapturedWorkspace(t *testing.T) {
	m, original := newNavigationUI()
	cmd := m.reportCurrentSession("original-session")
	other := &navigationWorkspace{countingWorkspace: &countingWorkspace{ready: true}}
	m.com.Workspace = other
	cmd()
	require.Equal(t, []string{"original-session"}, original.presence)
	require.Empty(t, other.presence)
}
