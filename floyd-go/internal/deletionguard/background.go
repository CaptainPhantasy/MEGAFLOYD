package deletionguard

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
)

// BackgroundJob is the durable correlation between one approved launch and a
// manager job. ExecutionID is unique across process restarts; shell IDs alone
// are reused. Classification retains pre-execution target and operation data.
type BackgroundJob struct {
	SessionID      string         `json:"session_id"`
	ToolCallID     string         `json:"tool_call_id"`
	ShellID        string         `json:"shell_id"`
	ExecutionID    string         `json:"execution_id"`
	Command        string         `json:"command"`
	WorkingDir     string         `json:"working_dir"`
	WorkspaceDir   string         `json:"workspace_dir"`
	ApprovalSet    string         `json:"approval_set"`
	Classification Classification `json:"classification"`
}

// BackgroundResult comes only from a matching live manager job's terminal
// channel. Errors, cancellation and timeout are retained without implying success.
type BackgroundResult struct {
	Job      BackgroundJob `json:"job"`
	Done     bool          `json:"done"`
	ExitCode int           `json:"exit_code"`
	Error    string        `json:"error,omitempty"`
}

// BindBackground preserves a live job's identity without treating launch as
// completion. Losing the process before this write leaves the launch uncertain.
func (g *Guard) BindBackground(sessionID string, c Classification, callID string, job BackgroundJob) (BackgroundJob, error) {
	if sessionID == "" || job.SessionID != sessionID || job.ToolCallID != callID || job.ShellID == "" || job.ExecutionID == "" || job.Command == "" || !filepath.IsAbs(job.WorkingDir) || !filepath.IsAbs(job.WorkspaceDir) || !reflect.DeepEqual(job.Classification, c) {
		return BackgroundJob{}, errors.New("background job does not match the approved launch")
	}
	g.ledger.mu.Lock()
	defer g.ledger.mu.Unlock()
	record, err := g.ledger.Read(sessionID)
	if err != nil {
		return BackgroundJob{}, err
	}
	approval, ok := approvalForSubset(record, c)
	if !ok {
		return BackgroundJob{}, ErrNotConfirmed
	}
	for _, previous := range record.BackgroundJobs {
		if previous.ToolCallID == callID || previous.ExecutionID == job.ExecutionID {
			return BackgroundJob{}, errors.New("background job is already bound")
		}
	}
	started := false
	for _, attempt := range record.Attempts {
		if attempt.ToolCallID != callID {
			continue
		}
		if attempt.State == "finished" {
			return BackgroundJob{}, errors.New("background launch already finished")
		}
		if attempt.State == "started" && attempt.ApprovalSet == approval.TargetSet && slices.Equal(CanonicalTargets(attempt.Targets), CanonicalTargets(c.Targets)) {
			started = true
		}
	}
	if !started {
		return BackgroundJob{}, errors.New("background job has no matching durable launch")
	}
	job.ApprovalSet = approval.TargetSet
	record.BackgroundJobs = append(record.BackgroundJobs, job)
	if err := g.ledger.write(sessionID, record); err != nil {
		return BackgroundJob{}, err
	}
	g.ledger.background.mu.Lock()
	var captured BackgroundJob
	bytes, _ := json.Marshal(job)
	_ = json.Unmarshal(bytes, &captured)
	g.ledger.background.bindings[backgroundEvidenceKey(BackgroundResult{Job: job})] = captured
	g.ledger.background.mu.Unlock()
	return job, nil
}

// PendingBackground returns correlations whose terminal outcome has not yet
// been durably recorded. Missing manager evidence must leave these blocked.
func (g *Guard) PendingBackground(sessionID string) ([]BackgroundJob, error) {
	record, err := g.ledger.Read(sessionID)
	if err != nil {
		return nil, err
	}
	var jobs []BackgroundJob
	for _, job := range record.BackgroundJobs {
		finished := false
		for _, attempt := range record.Attempts {
			if attempt.ToolCallID == job.ToolCallID && (attempt.State == "finished" || attempt.BackgroundOutcome != nil) {
				finished = true
				break
			}
		}
		if !finished {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// ReconcileBackground accepts terminal evidence for the exact protected
// correlation. A running observation cannot finish or approve any target.
func (g *Guard) ReconcileBackground(sessionID string, result BackgroundResult) error {
	if !result.Done {
		return nil
	}
	var successful []string
	if result.ExitCode == 0 && result.Error == "" {
		successful = result.Job.Classification.Targets
	}
	if result.Job.SessionID != sessionID {
		return errors.New("background outcome session mismatch")
	}
	// Only a launch bound by this trusted process can be cached across a
	// broker outage. Durable-only duplicate observations still validate through
	// recordOutcome, but cannot poison or displace live recovery evidence.
	g.ledger.background.mu.Lock()
	binding, known := g.ledger.background.bindings[backgroundEvidenceKey(result)]
	g.ledger.background.mu.Unlock()
	if !known || !reflect.DeepEqual(binding, result.Job) {
		return g.recordOutcome(sessionID, result.Job.Classification, result.Job.ToolCallID, successful, false, &result)
	}
	if err := g.ledger.background.remember(result); err != nil {
		return err
	}
	if err := g.recordOutcome(sessionID, result.Job.Classification, result.Job.ToolCallID, successful, false, &result); err != nil {
		return err
	}
	g.ledger.background.forget(result)
	return nil
}

// Failed persistence retains trusted in-process terminal evidence, including
// after job_kill removes the manager entry. This cache is never reconstructed
// from workspace files, output text or a new process's reused job identifiers.
const maxPendingBackgroundOutcomes = 50

type backgroundEvidence struct {
	mu       sync.Mutex
	results  map[string]BackgroundResult
	bindings map[string]BackgroundJob
	slots    map[string]bool
}

func backgroundEvidenceKey(result BackgroundResult) string {
	return result.Job.SessionID + "\x00" + result.Job.ExecutionID
}

func (e *backgroundEvidence) reserve(sessionID, callID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := sessionID + "\x00" + callID
	if e.slots[key] {
		return errors.New("execution already has a pending completion slot")
	}
	if len(e.slots) >= maxPendingBackgroundOutcomes {
		return errors.New("pending background completion evidence capacity reached; persist retained outcomes before another execution")
	}
	e.slots[key] = true
	return nil
}

func (e *backgroundEvidence) release(sessionID, callID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.slots, sessionID+"\x00"+callID)
}

func (e *backgroundEvidence) remember(result BackgroundResult) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := backgroundEvidenceKey(result)
	if previous, ok := e.results[key]; ok {
		if !reflect.DeepEqual(previous, result) {
			return errors.New("conflicting pending background evidence")
		}
		return nil
	}
	if len(e.results) >= maxPendingBackgroundOutcomes {
		return errors.New("pending background completion evidence capacity reached")
	}
	var captured BackgroundResult
	bytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes, &captured); err != nil {
		return err
	}
	e.results[key] = captured
	return nil
}

func (e *backgroundEvidence) forget(result BackgroundResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.results, backgroundEvidenceKey(result))
	delete(e.bindings, backgroundEvidenceKey(result))
	delete(e.slots, result.Job.SessionID+"\x00"+result.Job.ToolCallID)
}

// RetryBackgroundOutcomes retries retained terminal observations on an explicit
// reconciliation path. It neither polls indefinitely nor invents missing proof.
func (g *Guard) RetryBackgroundOutcomes(sessionID string) error {
	e := g.ledger.background
	e.mu.Lock()
	var results []BackgroundResult
	for _, result := range e.results {
		if result.Job.SessionID == sessionID {
			results = append(results, result)
		}
	}
	e.mu.Unlock()
	// Stable order makes repeated reconciliation deterministic.
	slices.SortFunc(results, func(a, b BackgroundResult) int {
		return strings.Compare(backgroundEvidenceKey(a), backgroundEvidenceKey(b))
	})
	for _, result := range results {
		if err := g.ReconcileBackground(sessionID, result); err != nil {
			return err
		}
	}
	return nil
}
