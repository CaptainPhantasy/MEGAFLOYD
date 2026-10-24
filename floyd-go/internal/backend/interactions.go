package backend

import "github.com/legacy-ai/floyd/internal/proto"

func (b *Backend) PendingInteractions(workspaceID string) (proto.PendingInteractions, error) {
	result := proto.PendingInteractions{Permissions: []proto.PermissionRequest{}, Questions: []proto.QuestionRequest{}}
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return result, err
	}
	if ws.Permissions != nil {
		for _, req := range ws.Permissions.Pending() {
			result.Permissions = append(result.Permissions, proto.PermissionRequest{
				ID: req.ID, SessionID: req.SessionID, ToolCallID: req.ToolCallID,
				ToolName: req.ToolName, Description: req.Description,
				Action: req.Action, Path: req.Path, Params: req.Params,
			})
		}
	}
	if ws.Questions != nil {
		for _, req := range ws.Questions.Pending() {
			item := proto.QuestionRequest{
				ID: req.ID, SessionID: req.SessionID, ToolCallID: req.ToolCallID,
				ConfirmTitle: req.ConfirmTitle, ConfirmDescription: req.ConfirmDescription,
				Questions: []proto.QuestionItem{},
			}
			for _, question := range req.Questions {
				q := proto.QuestionItem{ID: question.ID, Type: string(question.Type), Label: question.Label, Question: question.Text, Description: question.Description}
				for _, choice := range question.Choices {
					q.Choices = append(q.Choices, proto.QuestionChoice{ID: choice.ID, Label: choice.Label, Description: choice.Description})
				}
				item.Questions = append(item.Questions, q)
			}
			result.Questions = append(result.Questions, item)
		}
	}
	return result, nil
}
