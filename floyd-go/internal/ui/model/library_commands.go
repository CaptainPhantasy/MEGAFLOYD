package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/ai"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/legacy-ai/floyd/internal/version"
)

const (
	providersDialogID      = "configured-providers"
	providerModelsDialogID = "configured-provider-models"
	promptsDialogID        = "prompt-templates"
)

func (m *UI) insertLibraryDraft(text string) {
	if m.textarea.Value() != "" {
		m.textarea.MoveToEnd()
		m.textarea.InsertString("\n\n")
	}
	m.textarea.InsertString(text)
}

func (m *UI) providerCommand(args string) tea.Cmd {
	cfg := m.com.Config()
	if cfg == nil || cfg.Providers == nil {
		return util.ReportWarn("No providers are configured. Use /model to configure one.")
	}
	var choices []dialog.Choice
	if id := strings.TrimSpace(args); id != "" {
		provider, ok := cfg.Providers.Get(id)
		if !ok {
			return util.ReportWarn("Unknown configured provider: " + id)
		}
		for _, model := range provider.Models {
			choices = append(choices, dialog.Choice{Value: id + "/" + model.ID, Label: model.Name + " (" + model.ID + ")"})
		}
		if len(choices) == 0 {
			return m.showInfo("Provider "+id, "No models configured. Use /settings to inspect its configuration.")
		}
		m.dialog.OpenDialog(dialog.NewChoices(m.com, providerModelsDialogID, "Models — "+id, choices))
		return nil
	}
	for id, provider := range cfg.Providers.Seq2() {
		choices = append(choices, dialog.Choice{Value: id, Label: fmt.Sprintf("%s (%s) · %d models", provider.Name, id, len(provider.Models))})
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].Value < choices[j].Value })
	m.dialog.OpenDialog(dialog.NewChoices(m.com, providersDialogID, "Providers", choices))
	return nil
}

func (m *UI) promptTemplateCommand(args string) tea.Cmd {
	name, raw, _ := strings.Cut(strings.TrimSpace(args), " ")
	if name == "" || name == "list" {
		var choices []dialog.Choice
		for name := range ai.PromptTemplates {
			choices = append(choices, dialog.Choice{Value: name, Label: name})
		}
		sort.Slice(choices, func(i, j int) bool { return choices[i].Value < choices[j].Value })
		m.dialog.OpenDialog(dialog.NewChoices(m.com, promptsDialogID, "Prompt Templates", choices))
		return nil
	}
	values := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return util.ReportError(fmt.Errorf("template values must be a JSON object: %w", err))
		}
	}
	prompt, _, err := ai.UsePromptTemplate(name, values)
	if err != nil {
		return util.ReportError(err)
	}
	// These templates become editable user text; their legacy system prose is
	// deliberately not injected as another behavioral policy.
	m.insertLibraryDraft(prompt)
	m.dialog.CloseDialog(promptsDialogID)
	m.dialog.CloseDialog(dialog.CommandsID)
	m.focus = uiFocusEditor
	m.textarea.Focus()
	m.updateLayoutAndSize()
	return util.ReportInfo("Template inserted. Edit its fields, then send it.")
}

func (m *UI) handleChoice(choice dialog.ActionChoose) tea.Cmd {
	switch choice.DialogID {
	case providersDialogID:
		return m.providerCommand(choice.Value)
	case providerModelsDialogID:
		cmd := m.selectNamedModel(choice.Value, config.SelectedModelTypeLarge)
		m.dialog.CloseDialog(providerModelsDialogID)
		return cmd
	case promptsDialogID:
		return m.promptTemplateCommand(choice.Value)
	}
	return nil
}

func (m *UI) todosCommand() tea.Cmd {
	if m.session == nil {
		return m.showInfo("Todos", "No active session.")
	}
	if len(m.session.Todos) == 0 {
		return m.showInfo("Todos", "No todos in this session.")
	}
	var body strings.Builder
	for i, item := range m.session.Todos {
		fmt.Fprintf(&body, "%d. [%s] %s\n", i+1, item.Status, item.Content)
	}
	return m.showInfo("Todos", body.String())
}

func (m *UI) releaseNotesCommand() tea.Cmd {
	notes := version.DefaultChangelog()
	var body strings.Builder
	fmt.Fprintf(&body, "Running build: %s\nCommit: %s\nBuild ID: %s\nNotes updated: %s\n\n", version.Version, version.Commit, version.BuildID, notes.LastUpdated)
	for _, entry := range notes.Entries {
		fmt.Fprintf(&body, "%s · %s · %s\n%s\n\n", entry.Version, entry.Date, entry.Title, entry.Description)
	}
	return m.showInfo("Release Notes", body.String())
}
