package workspace

import (
	"context"

	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/question"
)

// QuestionSubmissionWorkspace correlates a submission with its displayed
// request and returns transport or persistence errors without dropping drafts.
type QuestionSubmissionWorkspace interface {
	SubmitQuestion(context.Context, string, []question.Answer) (bool, error)
	CancelQuestionRequest(context.Context, string) (bool, error)
}

func (w *AppWorkspace) SubmitQuestion(ctx context.Context, id string, answers []question.Answer) (bool, error) {
	return w.app.Questions.SubmitLocal(ctx, id, answers)
}

func (w *AppWorkspace) CancelQuestionRequest(ctx context.Context, id string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return w.app.Questions.CancelRequest(id), nil
}

func (w *ClientWorkspace) SubmitQuestion(ctx context.Context, id string, answers []question.Answer) (bool, error) {
	in := proto.QuestionAnswer{BatchRequestID: id, Responses: make([]proto.QuestionResponse, len(answers))}
	for i, answer := range answers {
		in.Responses[i] = proto.QuestionResponse{QuestionID: answer.QuestionID, SelectedIDs: answer.SelectedIDs, FillInText: answer.FillInText, Yes: answer.Yes, Notes: answer.Notes}
	}
	return w.client.AnswerQuestionBatch(ctx, w.workspaceID(), in)
}

func (w *ClientWorkspace) CancelQuestionRequest(ctx context.Context, id string) (bool, error) {
	return w.client.CancelQuestionRequest(ctx, w.workspaceID(), id)
}
