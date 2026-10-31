package question

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/stretchr/testify/require"
)

func TestAnswerRejectsResponsesFromAnotherQuestion(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	s := NewService()
	events := s.Subscribe(ctx)
	go func() { _, _ = s.Ask(ctx, pendingHandoffQuestion()) }()
	req := awaitPendingQuestion(t, events)
	require.False(t, s.AnswerRequest(req.ID, []Answer{{QuestionID: "previous-question", FillInText: "Keep this answer"}}))
	require.Len(t, s.Pending(), 1, "a stale answer must not resolve the current question")
}

func TestNonInteractiveQuestionsNeverWaitForHumanInput(t *testing.T) {
	for _, req := range []Request{pendingHandoffQuestion(), protectedQuestion()} {
		t.Run(req.ID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(permission.WithNonInteractive(t.Context()), time.Second)
			defer cancel()
			s := NewService()
			events := s.Subscribe(ctx)
			answers, err := s.Ask(ctx, req)
			require.ErrorIs(t, err, permission.ErrInteractionRequired)
			require.Nil(t, answers)
			require.Empty(t, s.Pending())
			select {
			case <-events:
				t.Fatal("an unattended question was published")
			default:
			}
		})
	}
}

func TestSubmissionPersistsBeforeAcknowledgmentAndRecovers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewService()
	ConfigureStorage(s, dir)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	events := s.Subscribe(ctx)
	result := make(chan pendingQuestionResult, 1)
	go func() { a, e := s.Ask(ctx, pendingHandoffQuestion()); result <- pendingQuestionResult{a, e} }()
	req := awaitPendingQuestion(t, events)
	answers := []Answer{{QuestionID: req.Questions[0].ID, FillInText: "The complete human answer"}}
	accepted, err := s.SubmitLocal(ctx, req.ID, answers)
	require.NoError(t, err)
	require.True(t, accepted)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	info, err := files[0].Info()
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	first := <-result
	require.NoError(t, first.err)
	require.Equal(t, answers, first.answers)
	accepted, err = s.SubmitLocal(ctx, req.ID, answers)
	require.NoError(t, err)
	require.True(t, accepted, "retry after a lost acknowledgment is idempotent")
	changed := cloneAnswers(answers)
	changed[0].FillInText = "A different answer"
	accepted, err = s.SubmitLocal(ctx, req.ID, changed)
	require.ErrorIs(t, err, ErrSubmissionConflict)
	require.False(t, accepted)

	restarted := NewService()
	ConfigureStorage(restarted, dir)
	replayed, err := restarted.Ask(ctx, pendingHandoffQuestion())
	require.NoError(t, err)
	require.Equal(t, answers, replayed)
	require.Empty(t, restarted.Pending(), "a recorded ordinary answer must survive restart without reasking")
}

func TestSubmissionPublicationFailureRetainsPendingAnswer(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewService()
	ConfigureStorage(s, dir)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	events := s.Subscribe(ctx)
	done := make(chan pendingQuestionResult, 1)
	go func() { a, e := s.Ask(ctx, pendingHandoffQuestion()); done <- pendingQuestionResult{a, e} }()
	req := awaitPendingQuestion(t, events)
	answer := []Answer{{QuestionID: req.Questions[0].ID, FillInText: "Keep the answer until it is saved"}}
	path := s.submissionPath(req.ID)
	require.NoError(t, os.Mkdir(path, 0o700))
	// The first-attempt path reaches publication without reading a retry record.
	accepted, err := s.submit(ctx, req.ID, answer, true, false)
	require.Error(t, err)
	require.False(t, accepted)
	require.Empty(t, s.received, "failed publication must not be acknowledged in memory")
	require.Len(t, s.Pending(), 1)
	select {
	case <-done:
		t.Fatal("failed publication must not deliver the answer")
	default:
	}
	require.NoError(t, os.Rename(path, filepath.Join(dir, "saved-blocking-directory")))
	accepted, err = s.SubmitLocal(ctx, req.ID, answer)
	require.NoError(t, err)
	require.True(t, accepted)
	result := <-done
	require.NoError(t, result.err)
	require.Equal(t, answer, result.answers)

	restarted := NewService()
	ConfigureStorage(restarted, dir)
	replayed, err := restarted.Ask(ctx, pendingHandoffQuestion())
	require.NoError(t, err)
	require.Equal(t, answer, replayed)
}

func TestFailedSubmissionKeepsQuestionPendingForRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("fixture"), 0o600))
	s := NewService()
	ConfigureStorage(s, blocked)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	events := s.Subscribe(ctx)
	done := make(chan pendingQuestionResult, 1)
	// Start without storage so reading a deliberately invalid storage path
	// does not fail before the user has a form to answer.
	ConfigureStorage(s, "")
	go func() { a, e := s.Ask(ctx, pendingHandoffQuestion()); done <- pendingQuestionResult{a, e} }()
	req := awaitPendingQuestion(t, events)
	ConfigureStorage(s, blocked)
	answers := []Answer{{QuestionID: req.Questions[0].ID, FillInText: "Preserve this draft"}}
	accepted, err := s.SubmitLocal(ctx, req.ID, answers)
	require.Error(t, err)
	require.False(t, accepted)
	require.Len(t, s.Pending(), 1)
	ConfigureStorage(s, filepath.Join(dir, "working"))
	accepted, err = s.SubmitLocal(ctx, req.ID, answers)
	require.NoError(t, err)
	require.True(t, accepted)
	require.Equal(t, answers, (<-done).answers)
}

func TestSubmissionRecoversAfterPersistenceBeforeDispatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewService()
	ConfigureStorage(s, dir)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	events := s.Subscribe(ctx)
	go func() { _, _ = s.Ask(ctx, pendingHandoffQuestion()) }()
	req := awaitPendingQuestion(t, events)
	answers := []Answer{{QuestionID: req.Questions[0].ID, SelectedIDs: []string{"terminal"}, Notes: map[string]string{"terminal": "Preserve this explanation too"}}}
	// The process can stop after the atomic write but before dispatching to the
	// tool. Reconstruct that exact on-disk boundary without claiming delivery.
	s.mu.Lock()
	err := s.saveSubmission(submissionRecord{Request: req, Answers: answers})
	s.mu.Unlock()
	require.NoError(t, err)
	require.Len(t, s.Pending(), 1)

	restarted := NewService()
	ConfigureStorage(restarted, dir)
	recovered, err := restarted.Ask(ctx, pendingHandoffQuestion())
	require.NoError(t, err)
	require.Equal(t, answers, recovered)
}

func TestProtectedConfirmationIsNeverReplayed(t *testing.T) {
	t.Parallel()
	s := NewService()
	dir := t.TempDir()
	ConfigureStorage(s, dir)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	events := s.Subscribe(ctx)
	done := make(chan pendingQuestionResult, 1)
	go func() { a, e := s.Ask(ctx, protectedQuestion()); done <- pendingQuestionResult{a, e} }()
	req := awaitPendingQuestion(t, events)
	yes := true
	answers := []Answer{{QuestionID: req.Questions[0].ID, Yes: &yes}}
	accepted, err := s.SubmitLocal(ctx, req.ID, answers)
	require.NoError(t, err)
	require.True(t, accepted)
	require.NoError(t, (<-done).err)
	accepted, err = s.SubmitLocal(ctx, req.ID, answers)
	require.ErrorIs(t, err, ErrStaleRequest)
	require.False(t, accepted)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files)
}
