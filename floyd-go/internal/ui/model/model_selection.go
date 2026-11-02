package model

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/ui/util"
)

type namedModelAppliedMsg struct {
	Provider string
	Type     config.SelectedModelType
	Err      error
	Name     string
}

func (m *UI) selectNamedModel(name string, typ config.SelectedModelType) tea.Cmd {
	if m.isAgentBusy() {
		return util.ReportWarn("Agent is busy, please wait before changing models.")
	}
	selected, err := config.ResolveModel(m.com.Config(), name)
	if err != nil {
		return util.ReportError(err)
	}
	ws := m.com.Workspace
	return func() tea.Msg {
		result := namedModelAppliedMsg{Provider: selected.Provider, Type: typ, Name: selected.Model}
		if err := ws.UpdatePreferredModel(config.ScopeGlobal, typ, selected); err != nil {
			result.Err = err
			return result
		}
		if err := ws.UpdateAgentModel(context.Background()); err != nil {
			result.Err = fmt.Errorf("model saved but could not be activated: %w", err)
		}
		return result
	}
}
