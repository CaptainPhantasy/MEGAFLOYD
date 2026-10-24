package backend

import (
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/question"
)

// AnswerQuestion submits answers for a question. The returned bool
// reports whether this call resolved the pending request (true) or
// found it already resolved by a previous caller (false).
func (b *Backend) AnswerQuestion(workspaceID string, req proto.QuestionAnswer) (bool, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return false, err
	}

	responses := make([]question.Answer, len(req.Responses))
	for i, r := range req.Responses {
		responses[i] = question.Answer{
			QuestionID:  r.QuestionID,
			SelectedIDs: r.SelectedIDs,
			FillInText:  r.FillInText,
			Yes:         r.Yes,
			Notes:       r.Notes,
		}
	}

	return ws.Questions.AnswerRequest(req.BatchRequestID, responses), nil
}

// CancelQuestion cancels the pending question for a workspace.
// Returns true if a question was cancelled, false if none was
// pending.
func (b *Backend) CancelQuestion(workspaceID string) (bool, error) {
	return b.CancelQuestionRequest(workspaceID, "")
}

func (b *Backend) CancelQuestionRequest(workspaceID, requestID string) (bool, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return false, err
	}
	return ws.Questions.CancelRequest(requestID), nil
}
