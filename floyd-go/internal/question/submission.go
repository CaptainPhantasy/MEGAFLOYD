package question

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	ErrStaleRequest       = errors.New("this question is no longer pending; your answer has not been submitted")
	ErrSubmissionConflict = errors.New("this question already has a different recorded answer; your new answer has not been submitted")
)

type submissionRecord struct {
	Request Request  `json:"request"`
	Answers []Answer `json:"answers"`
}

// ConfigureStorage retains accepted ordinary answers before acknowledging them.
// The agent's existing tool result remains the transcript; this private record
// also lets a retried call recover after interruption before result delivery.
func ConfigureStorage(service Service, directory string) {
	if s, ok := service.(*questionService); ok {
		s.mu.Lock()
		s.storageDir = directory
		s.mu.Unlock()
	}
}

// SubmitLocal accepts a correlated response from the in-process human UI.
func (s *questionService) SubmitLocal(ctx context.Context, id string, answers []Answer) (bool, error) {
	if id == "" {
		return false, ErrStaleRequest
	}
	return s.submit(ctx, id, answers, true, true)
}

// SubmitRemote accepts ordinary answers. Protected confirmations still require
// direct local input and are never recovered from an ordinary answer record.
func (s *questionService) SubmitRemote(ctx context.Context, id string, answers []Answer) (bool, error) {
	if id == "" {
		return false, ErrStaleRequest
	}
	return s.submit(ctx, id, answers, false, true)
}

func sameJSON(a, b any) bool {
	aa, ea := json.Marshal(a)
	bb, eb := json.Marshal(b)
	return ea == nil && eb == nil && bytes.Equal(aa, bb)
}

func cloneAnswers(in []Answer) []Answer {
	data, _ := json.Marshal(in)
	var out []Answer
	_ = json.Unmarshal(data, &out)
	return out
}

func validateAnswers(req Request, answers []Answer) error {
	if len(answers) != len(req.Questions) {
		return errors.New("answer every question before submitting")
	}
	for i, answer := range answers {
		q := req.Questions[i]
		if answer.QuestionID != q.ID {
			return errors.New("answer does not belong to the displayed question; your draft is unchanged")
		}
		known := make(map[string]bool, len(q.Choices))
		for _, choice := range q.Choices {
			known[choice.ID] = true
		}
		for _, selected := range answer.SelectedIDs {
			if !known[selected] {
				return fmt.Errorf("unknown choice %q for question %q", selected, q.ID)
			}
		}
		if q.Type == TypeSingleChoice && len(answer.SelectedIDs) > 1 {
			return errors.New("select only one choice for a single-choice question")
		}
	}
	return nil
}

func (s *questionService) submissionPath(id string) string {
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(s.storageDir, hex.EncodeToString(sum[:])+".json")
}

func (s *questionService) loadSubmission(id string) (submissionRecord, bool, error) {
	if r, found := s.received[id]; found {
		return r, true, nil
	}
	if s.storageDir == "" {
		return submissionRecord{}, false, nil
	}
	data, err := os.ReadFile(s.submissionPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return submissionRecord{}, false, nil
	}
	if err != nil {
		return submissionRecord{}, false, fmt.Errorf("read saved question answer: %w", err)
	}
	var record submissionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return record, false, fmt.Errorf("read saved question answer: %w", err)
	}
	if record.Request.ID != id || record.Request.RequireLocalInput {
		return record, false, errors.New("saved question answer identity does not match")
	}
	if err := validateAnswers(record.Request, record.Answers); err != nil {
		return record, false, fmt.Errorf("saved question answer is incomplete: %w", err)
	}
	return record, true, nil
}

func (s *questionService) saveSubmission(record submissionRecord) error {
	if s.storageDir != "" {
		if err := os.MkdirAll(s.storageDir, 0o700); err != nil {
			return fmt.Errorf("save question answer: %w", err)
		}
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(s.storageDir, ".answer-*.pending")
		if err != nil {
			return fmt.Errorf("save question answer: %w", err)
		}
		_, writeErr := file.Write(data)
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr != nil {
			return fmt.Errorf("save question answer: %w", writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("save question answer: %w", closeErr)
		}
		if err := os.Rename(file.Name(), s.submissionPath(record.Request.ID)); err != nil {
			return fmt.Errorf("save question answer: %w", err)
		}
		dir, err := os.Open(s.storageDir)
		if err != nil {
			return fmt.Errorf("save question answer: %w", err)
		}
		syncErr := dir.Sync()
		_ = dir.Close()
		if syncErr != nil {
			return fmt.Errorf("save question answer: %w", syncErr)
		}
	}
	if s.received == nil {
		s.received = make(map[string]submissionRecord)
	}
	s.received[record.Request.ID] = record
	return nil
}
