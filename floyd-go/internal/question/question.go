// Package question provides services for asking the user questions
// via the TUI and blocking until an answer is received. It mirrors
// the permission service pattern: publish a request over pubsub,
// block on a channel, and resolve when the UI sends back answers.
//
// Requests and replies are correlated because notifications and submissions
// from different clients can arrive after the next request has opened.
package question

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/pubsub"
)

// ErrCancelled is returned by Ask when the user cancels the question.
var ErrCancelled = errors.New("question cancelled by user")

// ErrLocalInputRequired refuses protected confirmations over untrusted RPC input.
var ErrLocalInputRequired = errors.New("permanent-deletion confirmation requires direct in-process human input; use quarantine or an in-process TUI")

// Type identifies the kind of question to present.
type Type string

const (
	TypeYesNo        Type = "yes_no"
	TypeSingleChoice Type = "single_choice"
	TypeMultiChoice  Type = "multi_choice"
	TypeFreeText     Type = "free_text"
)

// Choice represents a single selectable option.
type Choice struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Question is a single question definition within a Request.
type Question struct {
	ID          string   `json:"id"`
	Type        Type     `json:"type"`
	Label       string   `json:"label,omitempty"`
	Text        string   `json:"question"`
	Description string   `json:"description,omitempty"`
	Choices     []Choice `json:"choices,omitempty"`
}

// Answer carries the user's response to a single Question.
type Answer struct {
	QuestionID  string            `json:"question_id"`
	SelectedIDs []string          `json:"selected_ids,omitempty"`
	FillInText  string            `json:"fill_in_text,omitempty"`
	Yes         *bool             `json:"yes,omitempty"`
	Notes       map[string]string `json:"notes,omitempty"`
}

// HasNotes reports whether any notes were attached.
func (a Answer) HasNotes() bool { return len(a.Notes) > 0 }

// Request is the service envelope published to the UI. It contains
// one or more Questions. A single question renders without tabs;
// multiple questions render as a tabbed form with confirmation.
type Request struct {
	// RequireLocalInput is engine-owned and cannot be supplied through JSON.
	RequireLocalInput  bool       `json:"-"`
	ID                 string     `json:"id"`
	SessionID          string     `json:"session_id"`
	ToolCallID         string     `json:"tool_call_id"`
	Questions          []Question `json:"questions"`
	ConfirmTitle       string     `json:"confirm_title,omitempty"`
	ConfirmDescription string     `json:"confirm_description,omitempty"`
}

// Validate checks that a Request has valid fields. For multiple
// questions, ConfirmTitle and ConfirmDescription are required.
func (r Request) Validate() error {
	if len(r.Questions) == 0 {
		return fmt.Errorf("at least one question is required")
	}
	if len(r.Questions) > MaxQuestions {
		return fmt.Errorf("questions exceed maximum of %d (got %d)", MaxQuestions, len(r.Questions))
	}
	for i, q := range r.Questions {
		if err := q.Validate(); err != nil {
			return fmt.Errorf("question %d: %w", i+1, err)
		}
	}
	return nil
}

// Validate checks that a Question has valid fields. Error messages
// are written for LLM consumption: specific and actionable.
func (q Question) Validate() error {
	label := q.identifier()
	if q.Text == "" {
		return fmt.Errorf("%s: question text is required", label)
	}
	if len(q.Text) > MaxQuestionLength {
		return fmt.Errorf("%s: text exceeds %d characters (got %d)", label, MaxQuestionLength, len(q.Text))
	}
	if q.Description == "" {
		return fmt.Errorf("%s: description is required", label)
	}
	if len(q.Description) > MaxDescriptionLength {
		return fmt.Errorf("%s: description exceeds %d characters (got %d)", label, MaxDescriptionLength, len(q.Description))
	}
	switch q.Type {
	case TypeYesNo, TypeFreeText:
		// No choices needed.
	case TypeSingleChoice, TypeMultiChoice:
		if len(q.Choices) < 2 {
			return fmt.Errorf("%s: %s requires at least 2 choices in the \"choices\" array (got %d). Use \"choices\", not \"options\"", label, q.Type, len(q.Choices))
		}
		if len(q.Choices) > MaxChoices {
			return fmt.Errorf("%s: choices exceed maximum of %d (got %d)", label, MaxChoices, len(q.Choices))
		}
		seen := make(map[string]bool, len(q.Choices))
		for i, c := range q.Choices {
			if c.ID == "" {
				return fmt.Errorf("%s: choice %d must have an \"id\" field", label, i+1)
			}
			if seen[c.ID] {
				return fmt.Errorf("%s: choice %d has duplicate id %q", label, i+1, c.ID)
			}
			seen[c.ID] = true
			if c.Label == "" {
				return fmt.Errorf("%s: choice %d (%s) must have a \"label\" field", label, i+1, c.ID)
			}
			if len(c.Label) > MaxChoiceLabelLength {
				return fmt.Errorf("%s: choice %d label exceeds %d characters (got %d)", label, i+1, MaxChoiceLabelLength, len(c.Label))
			}
			if len(c.Description) > MaxChoiceDescriptionLength {
				return fmt.Errorf("%s: choice %d description exceeds %d characters (got %d)", label, i+1, MaxChoiceDescriptionLength, len(c.Description))
			}
		}
	default:
		return fmt.Errorf("%s: unknown type %q (must be yes_no, single_choice, multi_choice, or free_text)", label, q.Type)
	}
	return nil
}

// identifier returns a human-readable label for error messages.
// Uses the question label, text excerpt, or a fallback.
func (q Question) identifier() string {
	if q.Label != "" {
		return fmt.Sprintf("[%s]", q.Label)
	}
	if q.Text != "" {
		t := q.Text
		if len(t) > 40 {
			t = t[:40] + "…"
		}
		return fmt.Sprintf("[%s]", t)
	}
	return "[unnamed question]"
}

const (
	MaxQuestionLength          = 240
	MaxDescriptionLength       = 600
	MaxChoiceLabelLength       = 200
	MaxChoiceDescriptionLength = 200
	MaxChoices                 = 5
	MaxQuestions               = 5
)

// Notification is published when a question batch is resolved so
// that non-answering clients can dismiss their open forms.
type Notification struct {
	BatchID string `json:"batch_id"`
}

// Service manages the lifecycle of question requests. Only one
// question can be pending at a time.
type Service interface {
	pubsub.Subscriber[Request]

	// SubscribeNotifications returns a channel for question
	// resolution notifications.
	SubscribeNotifications(ctx context.Context) <-chan pubsub.Event[Notification]

	// Ask publishes questions and blocks until the user answers
	// or the context is cancelled.
	Ask(ctx context.Context, req Request) ([]Answer, error)

	// Answer resolves the pending question with the given answers.
	Answer(answers []Answer) bool
	AnswerRequest(id string, answers []Answer) bool
	SubmitLocal(ctx context.Context, id string, answers []Answer) (bool, error)
	SubmitRemote(ctx context.Context, id string, answers []Answer) (bool, error)
	Pending() []Request

	// Cancel cancels the pending question. Returns false if no
	// question is pending.
	Cancel() bool
	CancelRequest(id string) bool
}

type answerResult struct {
	answers []Answer
	err     error
}

type questionService struct {
	remoteOnly         bool
	broker             *pubsub.Broker[Request]
	notificationBroker *pubsub.Broker[Notification]
	mu                 sync.Mutex
	pending            chan answerResult
	cancelled          chan struct{}
	pendingID          string
	pendingRequest     *Request
	askMu              sync.Mutex
	storageDir         string
	received           map[string]submissionRecord
}

// NewService creates a question service for direct in-process input.
func NewService() *questionService {
	return &questionService{
		broker:             pubsub.NewBroker[Request](),
		notificationBroker: pubsub.NewBroker[Notification](),
	}
}

// NewRemoteService preserves ordinary questions but rejects protected human
// confirmations because RPC clients do not provide an authenticated local input.
func NewRemoteService() *questionService {
	service := NewService()
	service.remoteOnly = true
	return service
}

// Subscribe returns a channel for question events.
func (s *questionService) Subscribe(ctx context.Context) <-chan pubsub.Event[Request] {
	return s.broker.Subscribe(ctx)
}

// SubscribeNotifications returns a channel for question resolution
// notifications.
func (s *questionService) SubscribeNotifications(ctx context.Context) <-chan pubsub.Event[Notification] {
	return s.notificationBroker.Subscribe(ctx)
}

// Ask publishes a request and blocks until the user answers.
func (s *questionService) Ask(ctx context.Context, req Request) ([]Answer, error) {
	if permission.IsNonInteractive(ctx) {
		return nil, fmt.Errorf("cannot ask a question without an interactive terminal: %w", permission.ErrInteractionRequired)
	}
	if req.RequireLocalInput && s.remoteOnly {
		return nil, ErrLocalInputRequired
	}
	if req.ID == "" {
		if !req.RequireLocalInput && req.SessionID != "" && req.ToolCallID != "" {
			req.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(req.SessionID+"\x00"+req.ToolCallID)).String()
		} else {
			req.ID = uuid.New().String()
		}
	}
	for i := range req.Questions {
		if req.Questions[i].ID == "" {
			req.Questions[i].ID = fmt.Sprintf("%s:%d", req.ID, i)
		}
	}

	// Apply defaults for multi-question confirm fields.
	if len(req.Questions) >= 2 {
		if req.ConfirmTitle == "" {
			req.ConfirmTitle = "Ready to go?"
		}
		if req.ConfirmDescription == "" {
			req.ConfirmDescription = "Review your answers above and confirm."
		}
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	s.askMu.Lock()
	defer s.askMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !req.RequireLocalInput {
		s.mu.Lock()
		record, found, err := s.loadSubmission(req.ID)
		s.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if found {
			if !sameJSON(record.Request, req) {
				return nil, ErrSubmissionConflict
			}
			return cloneAnswers(record.Answers), nil
		}
	}
	responseCh := make(chan answerResult, 1)
	cancelCh := make(chan struct{})
	s.mu.Lock()
	s.pending = responseCh
	s.cancelled = cancelCh
	s.pendingID = req.ID
	s.pendingRequest = &req
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		unresolved := s.pendingID == req.ID
		s.pending = nil
		s.cancelled = nil
		s.pendingID = ""
		s.pendingRequest = nil
		s.mu.Unlock()
		if unresolved {
			s.notificationBroker.Publish(pubsub.CreatedEvent, Notification{BatchID: req.ID})
		}
	}()

	s.broker.Publish(pubsub.CreatedEvent, req)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-cancelCh:
		return nil, ErrCancelled
	case result := <-responseCh:
		return result.answers, result.err
	}
}

// Answer resolves a question from the direct in-process UI. RPC adapters must
// use AnswerRequest so protected confirmations cannot accept network input.
func (s *questionService) Answer(answers []Answer) bool {
	return s.answerRequest("", answers, true)
}

// AnswerRequest resolves ordinary questions submitted by remote adapters. A
// protected request is atomically refused and released with an explicit error.
func (s *questionService) AnswerRequest(id string, answers []Answer) bool {
	return s.answerRequest(id, answers, false)
}

func (s *questionService) answerRequest(id string, answers []Answer, local bool) bool {
	accepted, _ := s.submit(context.Background(), id, answers, local, false)
	return accepted
}

func (s *questionService) submit(ctx context.Context, id string, answers []Answer, local, retry bool) (bool, error) {
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return false, err
	}
	protected := s.pendingRequest != nil && s.pendingID == id && s.pendingRequest.RequireLocalInput
	if retry && id != "" && !protected {
		record, found, err := s.loadSubmission(id)
		if err != nil || found {
			if err != nil {
				s.mu.Unlock()
				return false, err
			}
			if !sameJSON(record.Answers, answers) {
				s.mu.Unlock()
				return false, ErrSubmissionConflict
			}
			if s.pending == nil || s.pendingID != id {
				s.mu.Unlock()
				return true, nil
			}
		}
	}
	if s.pending == nil || (id != "" && id != s.pendingID) {
		s.mu.Unlock()
		return false, ErrStaleRequest
	}
	batchID, ch := s.pendingID, s.pending
	accepted := local || (s.pendingRequest != nil && !s.pendingRequest.RequireLocalInput)
	if accepted {
		if err := validateAnswers(*s.pendingRequest, answers); err != nil {
			s.mu.Unlock()
			return false, err
		}
		answers = cloneAnswers(answers)
		if !s.pendingRequest.RequireLocalInput {
			if err := s.saveSubmission(submissionRecord{Request: *s.pendingRequest, Answers: answers}); err != nil {
				s.mu.Unlock()
				return false, err
			}
		}
	}
	s.pending, s.cancelled, s.pendingRequest = nil, nil, nil
	s.pendingID = ""
	s.mu.Unlock()
	result := answerResult{answers: answers}
	if !accepted {
		result = answerResult{err: ErrLocalInputRequired}
	}
	ch <- result
	s.notificationBroker.Publish(pubsub.CreatedEvent, Notification{BatchID: batchID})
	if !accepted {
		return false, ErrLocalInputRequired
	}
	return true, nil
}

// Cancel cancels the pending question. Returns false if no
// question is pending.
func (s *questionService) Cancel() bool {
	return s.CancelRequest("")
}

func (s *questionService) CancelRequest(id string) bool {
	s.mu.Lock()
	if s.cancelled == nil || (id != "" && id != s.pendingID) {
		s.mu.Unlock()
		return false
	}
	batchID, cancelCh := s.pendingID, s.cancelled
	s.pending, s.cancelled, s.pendingRequest = nil, nil, nil
	s.pendingID = ""
	s.mu.Unlock()
	close(cancelCh)
	s.notificationBroker.Publish(pubsub.CreatedEvent, Notification{BatchID: batchID})
	return true
}
