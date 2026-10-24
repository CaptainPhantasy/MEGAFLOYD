package backend

import (
	"context"

	"charm.land/fantasy"
)

type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

func (b *Backend) ToolCatalog(ctx context.Context, workspaceID string) ([]ToolDescriptor, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	ws.runMu.Lock()
	coordinator := ws.AgentCoordinator
	ws.runMu.Unlock()
	if refresher, ok := coordinator.(interface{ RefreshTools(context.Context) error }); ok {
		if err := refresher.RefreshTools(ctx); err != nil {
			return nil, err
		}
	}
	catalog, ok := coordinator.(interface{ ToolCatalog() []fantasy.ToolInfo })
	if !ok {
		return nil, ErrAgentNotInitialized
	}
	result := []ToolDescriptor{}
	for _, info := range catalog.ToolCatalog() {
		required := info.Required
		if required == nil {
			required = []string{}
		}
		result = append(result, ToolDescriptor{
			Name: info.Name, Description: info.Description,
			InputSchema: map[string]any{"type": "object", "properties": info.Parameters, "required": required},
		})
	}
	return result, nil
}
