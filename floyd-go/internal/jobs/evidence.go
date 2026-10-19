package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Evidence is one recorded check, bound to the artifact hash it observed.
type Evidence struct {
	Requirement string `json:"requirement"`
	Command     string `json:"command"`
	ExitCode    int    `json:"exit_code"`
	OutputHash  string `json:"output_hash"`
	// ExcerptPath is relative to the job directory so a copied job packet
	// stays self-contained.
	ExcerptPath     string    `json:"excerpt_path"`
	ArtifactHash    string    `json:"artifact_hash"`
	ArtifactVersion string    `json:"artifact_version,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
	Passed          bool      `json:"passed"`
}

// EvidenceInput describes a check to record.
type EvidenceInput struct {
	Requirement     string
	Command         string
	ExitCode        int
	Output          string
	ArtifactHash    string
	ArtifactVersion string
	// Timestamp is optional and defaults to now.
	Timestamp time.Time
}

// RequirementStatus is the verdict for one requirement.
type RequirementStatus struct {
	ID       string     `json:"id"`
	Text     string     `json:"text"`
	Status   string     `json:"status"`
	Reason   string     `json:"reason,omitempty"`
	Evidence []Evidence `json:"evidence,omitempty"`
}

// JobStatus is the full per-requirement picture at the current artifact hash.
type JobStatus struct {
	JobID               string              `json:"job_id"`
	Objective           string              `json:"objective"`
	Status              string              `json:"status"`
	Artifact            string              `json:"artifact,omitempty"`
	CurrentArtifactHash string              `json:"current_artifact_hash,omitempty"`
	ArtifactError       string              `json:"artifact_error,omitempty"`
	Requirements        []RequirementStatus `json:"requirements"`
	Complete            bool                `json:"complete"`
}

// CompletionReport is the answer to "can this job be marked complete?".
type CompletionReport struct {
	Complete     bool                `json:"complete"`
	ArtifactHash string              `json:"current_artifact_hash,omitempty"`
	Requirements []RequirementStatus `json:"requirements"`
	Blocking     []string            `json:"blocking,omitempty"`
}

// RecordEvidence writes an evidence receipt and its output excerpt. The
// receipt is bound to a non-empty artifact hash: the supplied hash, else the
// job's declared artifact hashed now.
func RecordEvidence(dataDir, jobID string, in EvidenceInput) (*Evidence, error) {
	dir, err := jobDir(dataDir, jobID)
	if err != nil {
		return nil, err
	}
	job, err := loadJobFromDir(dir)
	if err != nil {
		return nil, err
	}
	if job.Status == StatusComplete {
		return nil, ErrCompleted
	}
	if strings.TrimSpace(in.Command) == "" {
		return nil, errors.New("command is required")
	}

	var requirement *Requirement
	for i := range job.Requirements {
		if job.Requirements[i].ID == in.Requirement {
			requirement = &job.Requirements[i]
			break
		}
	}
	if requirement == nil {
		return nil, fmt.Errorf("%w: %s", ErrRequirementAbsent, in.Requirement)
	}

	artifactHash := strings.TrimSpace(in.ArtifactHash)
	if artifactHash == "" && job.Artifact != "" {
		hash, err := HashFile(job.Artifact)
		if err != nil {
			return nil, err
		}
		artifactHash = hash
	}
	if artifactHash == "" {
		return nil, ErrNoArtifact
	}

	evidenceDir := filepath.Join(dir, "evidence", requirement.ID)
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating evidence directory: %w", err)
	}
	seq, err := nextEvidenceNumber(evidenceDir)
	if err != nil {
		return nil, err
	}

	excerptRel := filepath.ToSlash(filepath.Join("evidence", requirement.ID, fmt.Sprintf("%d.output.txt", seq)))
	excerptPath := filepath.Join(dir, filepath.FromSlash(excerptRel))
	if err := writeFileAtomic(excerptPath, []byte(in.Output)); err != nil {
		return nil, err
	}

	ts := in.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	ev := Evidence{
		Requirement:     requirement.ID,
		Command:         in.Command,
		ExitCode:        in.ExitCode,
		OutputHash:      HashBytes([]byte(in.Output)),
		ExcerptPath:     excerptRel,
		ArtifactHash:    artifactHash,
		ArtifactVersion: in.ArtifactVersion,
		Timestamp:       ts,
		Passed:          in.ExitCode == 0,
	}
	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding evidence: %w", err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(filepath.Join(evidenceDir, fmt.Sprintf("%d.json", seq)), data); err != nil {
		return nil, err
	}

	job.UpdatedAt = time.Now().UTC()
	if err := saveJob(dir, job); err != nil {
		return nil, err
	}
	result := VerdictFailed
	if ev.Passed {
		result = VerdictPassed
	}
	if _, err := Journal(dataDir, jobID, JournalEntry{
		Tool:         "prove",
		Action:       "evidence recorded",
		Requirement:  requirement.ID,
		ArtifactHash: artifactHash,
		Detail:       in.Command,
		Result:       result,
	}); err != nil {
		return nil, err
	}
	return &ev, nil
}

// Status computes the per-requirement verdict at the artifact's current hash.
// Passing evidence recorded against a different hash is invalidated.
func Status(dataDir, jobID string) (*JobStatus, error) {
	dir, err := jobDir(dataDir, jobID)
	if err != nil {
		return nil, err
	}
	job, err := loadJobFromDir(dir)
	if err != nil {
		return nil, err
	}
	return statusFor(dir, job)
}

// CheckCompletion reports whether every requirement has passing evidence for
// the artifact's current hash.
func CheckCompletion(dataDir, jobID string) (*CompletionReport, error) {
	status, err := Status(dataDir, jobID)
	if err != nil {
		return nil, err
	}
	report := &CompletionReport{
		Complete:     status.Complete,
		ArtifactHash: status.CurrentArtifactHash,
		Requirements: status.Requirements,
	}
	for _, req := range status.Requirements {
		if req.Status != VerdictPassed {
			reason := req.Reason
			if reason == "" {
				reason = req.Status
			}
			report.Blocking = append(report.Blocking, fmt.Sprintf("%s: %s", req.ID, reason))
		}
	}
	return report, nil
}

// MarkComplete flips the job status only when CheckCompletion passes.
func MarkComplete(dataDir, jobID string) (*Job, error) {
	report, err := CheckCompletion(dataDir, jobID)
	if err != nil {
		return nil, err
	}
	if !report.Complete {
		return nil, fmt.Errorf("%w: %s", ErrIncomplete, strings.Join(report.Blocking, "; "))
	}
	dir, err := jobDir(dataDir, jobID)
	if err != nil {
		return nil, err
	}
	job, err := loadJobFromDir(dir)
	if err != nil {
		return nil, err
	}
	job.Status = StatusComplete
	job.UpdatedAt = time.Now().UTC()
	if err := saveJob(dir, job); err != nil {
		return nil, err
	}
	if _, err := Journal(dataDir, jobID, JournalEntry{
		Tool:   "contract",
		Action: "completion recorded",
		Result: VerdictPassed,
	}); err != nil {
		return nil, err
	}
	return job, nil
}

func statusFor(dir string, job *Job) (*JobStatus, error) {
	status := &JobStatus{
		JobID:        job.ID,
		Objective:    job.Objective,
		Status:       job.Status,
		Artifact:     job.Artifact,
		Requirements: make([]RequirementStatus, 0, len(job.Requirements)),
	}

	currentHash := ""
	if job.Artifact != "" {
		hash, err := HashFile(job.Artifact)
		if err != nil {
			status.ArtifactError = err.Error()
		} else {
			currentHash = hash
		}
	}
	status.CurrentArtifactHash = currentHash

	allPassed := len(job.Requirements) > 0
	for _, req := range job.Requirements {
		evidence, err := loadEvidence(dir, req.ID)
		if err != nil {
			return nil, err
		}
		rs := RequirementStatus{
			ID:       req.ID,
			Text:     req.Text,
			Status:   VerdictUnchecked,
			Evidence: evidence,
		}
		if len(evidence) == 0 {
			rs.Reason = "no evidence recorded"
			allPassed = false
			status.Requirements = append(status.Requirements, rs)
			continue
		}

		latest := evidence[len(evidence)-1]
		switch {
		case !latest.Passed:
			rs.Status = VerdictFailed
			rs.Reason = fmt.Sprintf("latest evidence failed (exit %d)", latest.ExitCode)
			allPassed = false
		case job.Artifact != "" && status.ArtifactError != "":
			rs.Status = VerdictUnchecked
			rs.Reason = "artifact unavailable: " + status.ArtifactError
			allPassed = false
		case job.Artifact != "" && latest.ArtifactHash != currentHash:
			rs.Status = VerdictUnchecked
			rs.Reason = fmt.Sprintf("artifact changed since evidence was recorded (evidence %s, current %s)", shortHash(latest.ArtifactHash), shortHash(currentHash))
			allPassed = false
		default:
			rs.Status = VerdictPassed
		}
		status.Requirements = append(status.Requirements, rs)
	}
	status.Complete = allPassed
	return status, nil
}

func loadEvidence(dir, requirementID string) ([]Evidence, error) {
	evidenceDir := filepath.Join(dir, "evidence", requirementID)
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing evidence: %w", err)
	}
	type numbered struct {
		seq int
		ev  Evidence
	}
	var records []numbered
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		seq, err := strconv.Atoi(strings.TrimSuffix(name, ".json"))
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(evidenceDir, name))
		if err != nil {
			return nil, fmt.Errorf("reading evidence: %w", err)
		}
		var ev Evidence
		if err := json.Unmarshal(data, &ev); err != nil {
			return nil, fmt.Errorf("parsing evidence %s: %w", name, err)
		}
		records = append(records, numbered{seq: seq, ev: ev})
	}
	slices.SortFunc(records, func(a, b numbered) int { return a.seq - b.seq })
	out := make([]Evidence, 0, len(records))
	for _, record := range records {
		out = append(out, record.ev)
	}
	return out, nil
}

func nextEvidenceNumber(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("listing evidence directory: %w", err)
	}
	maxSeq := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		seq, err := strconv.Atoi(strings.TrimSuffix(name, ".json"))
		if err == nil && seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq + 1, nil
}

func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	if hash == "" {
		return "(none)"
	}
	return hash
}
