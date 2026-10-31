package model

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

func configuredProviderReport(cfg *config.Config) string {
	if cfg == nil || cfg.Providers == nil {
		return "No providers are configured. Use /model to configure a provider."
	}
	var sections []string
	for id, provider := range cfg.Providers.Seq2() {
		var body strings.Builder
		fmt.Fprintf(&body, "%s (%s)\n", provider.Name, id)
		for _, model := range provider.Models {
			fmt.Fprintf(&body, "  %s/%s — %s\n", id, model.ID, model.Name)
		}
		sections = append(sections, body.String())
	}
	sort.Strings(sections)
	return strings.Join(sections, "\n") + "\nSelect with /model provider/model or browse /model."
}
