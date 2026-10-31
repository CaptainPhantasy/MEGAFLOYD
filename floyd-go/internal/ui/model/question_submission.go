package model

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/question"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/legacy-ai/floyd/internal/workspace"
)

type questionSubmissionResultMsg struct {
	BatchID   string
	Accepted  bool
	Cancelled bool
	Err       error
}

// acceptQuestionRequest preserves a visible draft through duplicate request
// events and while the acknowledgment of an earlier answer is in flight.
func (m *UI) acceptQuestionRequest(batch question.Request) bool {
	if form, ok := m.activeInline.(*dialog.QuestionForm); ok {
		if form.BatchID == batch.ID {
			return false
		}
		for _, pending := range m.questionQueue {
			if pending.ID == batch.ID {
				return false
			}
		}
		m.questionQueue = append(m.questionQueue, batch)
		return false
	}
	return true
}

func (m *UI) configureQuestionSubmission(form *dialog.QuestionForm) {
	ws, ok := m.com.Workspace.(workspace.QuestionSubmissionWorkspace)
	id := form.BatchID
	form.OnSubmit = func(answers []question.Answer) tea.Cmd {
		return func() tea.Msg {
			if !ok {
				return questionSubmissionResultMsg{BatchID: id, Err: errors.New("question submission is unavailable; your answer is still here")}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			accepted, err := ws.SubmitQuestion(ctx, id, answers)
			return questionSubmissionResultMsg{BatchID: id, Accepted: accepted, Err: err}
		}
	}
	form.OnCancelRequest = func() tea.Cmd {
		return func() tea.Msg {
			if !ok {
				return questionSubmissionResultMsg{BatchID: id, Err: errors.New("question cancellation is unavailable; your answer is still here")}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			accepted, err := ws.CancelQuestionRequest(ctx, id)
			return questionSubmissionResultMsg{BatchID: id, Accepted: accepted, Cancelled: true, Err: err}
		}
	}
}

func (m *UI) handleQuestionSubmissionResult(result questionSubmissionResultMsg) tea.Cmd {
	form, ok := m.activeInline.(*dialog.QuestionForm)
	if !ok || form.BatchID != result.BatchID {
		return nil
	}
	if result.Err != nil || !result.Accepted {
		message := "This question is no longer pending. Your answer has not been submitted and is still here."
		if result.Err != nil {
			message = "Could not send your answer: " + result.Err.Error() + ". Your draft is still here; retry when ready."
		}
		form.SubmissionFailed(message)
		if result.Err == nil || errors.Is(result.Err, question.ErrStaleRequest) || errors.Is(result.Err, question.ErrSubmissionConflict) {
			form.ResolvedExternally()
		}
		m.updateLayoutAndSize()
		return util.ReportWarn(message)
	}
	m.activeInline = nil
	m.textarea.Focus()
	m.openNextQuestion()
	m.updateLayoutAndSize()
	if result.Cancelled {
		return util.ReportInfo("Question cancelled")
	}
	return util.ReportInfo("Answer received")
}

func (m *UI) openNextQuestion() {
	if m.activeInline != nil || len(m.questionQueue) == 0 {
		return
	}
	next := m.questionQueue[0]
	m.questionQueue = m.questionQueue[1:]
	m.openBatchFormDialog(next)
}

func (m *UI) resolveQuestionNotification(notice question.Notification) {
	for i := 0; i < len(m.questionQueue); i++ {
		if m.questionQueue[i].ID == notice.BatchID {
			m.questionQueue = append(m.questionQueue[:i], m.questionQueue[i+1:]...)
			break
		}
	}
	if form, ok := m.activeInline.(*dialog.QuestionForm); ok && form.BatchID == notice.BatchID && !form.Submitting() {
		form.ResolvedExternally()
		m.updateLayoutAndSize()
	}
}
