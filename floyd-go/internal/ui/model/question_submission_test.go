package model

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/question"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/stretchr/testify/require"
)

type questionFlowWorkspace struct {
	*countingWorkspace
	questions question.Service
	fail      bool
	submits   int
}

func (w *questionFlowWorkspace) SubmitQuestion(ctx context.Context, id string, a []question.Answer) (bool, error) {
	w.submits++
	if w.fail {
		return false, errors.New("connection interrupted")
	}
	return w.questions.SubmitLocal(ctx, id, a)
}

func (w *questionFlowWorkspace) CancelQuestionRequest(ctx context.Context, id string) (bool, error) {
	return w.questions.CancelRequest(id), ctx.Err()
}

func questionFlowRequest(id string) question.Request {
	return question.Request{ID: id, SessionID: "s1", ToolCallID: id, Questions: []question.Question{{ID: id + "-q", Type: question.TypeFreeText, Text: "What should change?", Description: "Describe the required change."}}}
}

func questionResult(t *testing.T, cmd tea.Cmd) questionSubmissionResultMsg {
	t.Helper()
	require.NotNil(t, cmd)
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, nested := range batch {
			if nested != nil {
				if result, ok := nested().(questionSubmissionResultMsg); ok {
					return result
				}
			}
		}
	}
	result, ok := msg.(questionSubmissionResultMsg)
	require.True(t, ok, "got %T", msg)
	return result
}

func TestQuestionKeyboardDeliveryRetainsDraftAndAcknowledgesSavedAnswer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	s := question.NewService()
	question.ConfigureStorage(s, t.TempDir())
	events := s.Subscribe(ctx)
	answers := make(chan []question.Answer, 1)
	go func() { a, _ := s.Ask(ctx, questionFlowRequest("first")); answers <- a }()
	request := (<-events).Payload
	w := &questionFlowWorkspace{countingWorkspace: &countingWorkspace{ready: true}, questions: s, fail: true}
	u := newBusyUI(w.countingWorkspace)
	u.com.Workspace = w
	warmCaches(u, true)
	u.openBatchFormDialog(request)
	form := u.activeInline.(*dialog.QuestionForm)
	form.HandlePaste(tea.PasteMsg{Content: "Preserve my complete answer"})
	cmd := u.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Same(t, form, u.activeInline, "Enter must not dismiss before delivery")
	require.Equal(t, 0, w.submits, "network IO must run outside Update")
	result := questionResult(t, cmd)
	require.Error(t, result.Err)
	u.handleQuestionSubmissionResult(result)
	require.Same(t, form, u.activeInline)
	require.False(t, form.Submitting())
	w.fail = false
	result = questionResult(t, u.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyEnter}))
	require.NoError(t, result.Err)
	require.True(t, result.Accepted)
	u.handleQuestionSubmissionResult(result)
	require.Nil(t, u.activeInline)
	select {
	case received := <-answers:
		require.Equal(t, "Preserve my complete answer", received[0].FillInText)
	case <-ctx.Done():
		t.Fatal("answer did not reach waiting tool")
	}
}

func TestQuestionOldNotificationCannotDismissNextForm(t *testing.T) {
	u := newBusyUI(&countingWorkspace{ready: true})
	u.openBatchFormDialog(questionFlowRequest("next"))
	form := u.activeInline
	u.handleQuestionNotification(question.Notification{BatchID: "old"})
	require.Same(t, form, u.activeInline)
	u.openBatchFormDialog(questionFlowRequest("next"))
	require.Same(t, form, u.activeInline, "reconnect replay must preserve the draft")
}

func TestQuestionQueuesNextRequestUntilPriorAcknowledgment(t *testing.T) {
	u := newBusyUI(&countingWorkspace{ready: true})
	u.openBatchFormDialog(questionFlowRequest("first"))
	first := u.activeInline
	u.openBatchFormDialog(questionFlowRequest("next"))
	require.Same(t, first, u.activeInline)
	require.Len(t, u.questionQueue, 1)
	u.handleQuestionSubmissionResult(questionSubmissionResultMsg{BatchID: "first", Accepted: true})
	require.Equal(t, "next", u.activeInline.(*dialog.QuestionForm).BatchID)
	u.handleQuestionNotification(question.Notification{BatchID: "first"})
	require.Equal(t, "next", u.activeInline.(*dialog.QuestionForm).BatchID)
}

func TestQuestionStaleSubmissionKeepsDraftUntilExplicitDismissal(t *testing.T) {
	u := newBusyUI(&countingWorkspace{ready: true})
	u.openBatchFormDialog(questionFlowRequest("old"))
	form := u.activeInline.(*dialog.QuestionForm)
	form.HandlePaste(tea.PasteMsg{Content: "Keep the old draft visible"})
	u.handleQuestionSubmissionResult(questionSubmissionResultMsg{BatchID: "old", Err: question.ErrStaleRequest})
	require.Same(t, form, u.activeInline)
	u.openBatchFormDialog(questionFlowRequest("next"))
	u.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Equal(t, "next", u.activeInline.(*dialog.QuestionForm).BatchID)
}
