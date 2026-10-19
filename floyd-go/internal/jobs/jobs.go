// Package jobs implements durable job records: a contract of requirements,
// evidence receipts bound to an artifact hash, an append-only journal, and a
// portable handoff packet under <DataDirectory>/jobs/<jobID>/.
package jobs

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Job lifecycle states.
const (
	StatusOpen     = "open"
	StatusComplete = "complete"
)

// Requirement verdicts reported by Status and CheckCompletion.
const (
	VerdictPassed    = "passed"
	VerdictFailed    = "failed"
	VerdictUnchecked = "unchecked"
)

// Sentinel errors. Callers can test them with errors.Is.
var (
	ErrNotFound          = errors.New("job not found")
	ErrExists            = errors.New("job already exists")
	ErrInvalidID         = errors.New("invalid job or requirement id")
	ErrNoRequirements    = errors.New("job has no requirements")
	ErrRequirementExists = errors.New("requirement already exists")
	ErrRequirementAbsent = errors.New("requirement not found")
	ErrCompleted         = errors.New("job is complete")
	ErrIncomplete        = errors.New("job is incomplete")
	ErrNoArtifact        = errors.New("no artifact hash available")
	ErrUnknownAction     = errors.New("unknown journal action")
)

// Requirement is one checkable outcome of a job.
type Requirement struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Job is the persisted contract.
type Job struct {
	ID           string        `json:"id"`
	Objective    string        `json:"objective"`
	Requirements []Requirement `json:"requirements"`
	Status       string        `json:"status"`
	// Artifact is the optional file whose SHA-256 binds evidence. When set,
	// passing evidence recorded against a different hash is invalidated.
	Artifact string `json:"artifact,omitempty"`
	// ArtifactHash is the artifact hash captured when the job was created.
	ArtifactHash string `json:"artifact_hash,omitempty"`
	// Scope lists the paths this job is allowed to change. Drift uses it.
	Scope     []string  `json:"scope,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateOptions describes a new job.
type CreateOptions struct {
	// ID is optional; when empty an id is derived from Objective.
	ID           string
	Objective    string
	Requirements []string
	Artifact     string
	Scope        []string
}

// Create writes a new job record and returns it.
func Create(dataDir string, opts CreateOptions) (*Job, error) {
	objective := strings.TrimSpace(opts.Objective)
	if objective == "" {
		return nil, errors.New("objective is required")
	}
	if len(opts.Requirements) == 0 {
		return nil, ErrNoRequirements
	}

	id := strings.TrimSpace(opts.ID)
	if id == "" {
		id = newJobID(objective)
	}
	if err := validateID(id); err != nil {
		return nil, err
	}

	dir, err := jobDir(dataDir, id)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, jobFile)); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrExists, id)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("checking job directory: %w", err)
	}

	reqs := make([]Requirement, 0, len(opts.Requirements))
	seen := make(map[string]bool, len(opts.Requirements))
	for _, text := range opts.Requirements {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, errors.New("requirement text is required")
		}
		reqID := uniqueRequirementID(text, seen)
		seen[reqID] = true
		reqs = append(reqs, Requirement{ID: reqID, Text: text})
	}

	artifact, artifactHash, err := resolveArtifact(opts.Artifact)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	job := &Job{
		ID:           id,
		Objective:    objective,
		Requirements: reqs,
		Status:       StatusOpen,
		Artifact:     artifact,
		ArtifactHash: artifactHash,
		Scope:        normalizeScope(opts.Scope),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating job directory: %w", err)
	}
	if err := saveJob(dir, job); err != nil {
		return nil, err
	}
	if _, err := Journal(dataDir, id, JournalEntry{
		Tool:   "contract",
		Action: "contract created",
		Detail: objective,
	}); err != nil {
		return nil, err
	}
	return job, nil
}

// Load reads a job record.
func Load(dataDir, jobID string) (*Job, error) {
	dir, err := jobDir(dataDir, jobID)
	if err != nil {
		return nil, err
	}
	return loadJobFromDir(dir)
}

// List reads every valid job under the data directory, most recently updated
// first.
func List(dataDir string) ([]*Job, error) {
	entries, err := os.ReadDir(jobsRoot(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing jobs: %w", err)
	}

	var out []*Job
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(jobsRoot(dataDir), entry.Name())
		job, err := loadJobFromDir(dir)
		if err != nil {
			continue
		}
		out = append(out, job)
	}
	slices.SortFunc(out, func(a, b *Job) int {
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return b.UpdatedAt.Compare(a.UpdatedAt)
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

// AddRequirement appends a requirement to an open job.
func AddRequirement(dataDir, jobID, text string) (*Requirement, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("requirement text is required")
	}

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

	seen := make(map[string]bool, len(job.Requirements)+1)
	for _, req := range job.Requirements {
		seen[req.ID] = true
	}
	req := Requirement{ID: uniqueRequirementID(text, seen), Text: text}
	job.Requirements = append(job.Requirements, req)
	job.UpdatedAt = time.Now().UTC()
	if err := saveJob(dir, job); err != nil {
		return nil, err
	}
	if _, err := Journal(dataDir, jobID, JournalEntry{
		Tool:        "contract",
		Action:      "requirement added",
		Requirement: req.ID,
		Detail:      text,
	}); err != nil {
		return nil, err
	}
	return &req, nil
}

// HashFile returns the lowercase hex SHA-256 of a file.
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// HashBytes returns the lowercase hex SHA-256 of b.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

const jobFile = "job.json"

func jobsRoot(dataDir string) string {
	return filepath.Join(dataDir, "jobs")
}

func jobDir(dataDir, jobID string) (string, error) {
	if err := validateID(jobID); err != nil {
		return "", err
	}
	return filepath.Join(jobsRoot(dataDir), jobID), nil
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validateID(id string) error {
	if id == "" || len(id) > 128 || !idPattern.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	return nil
}

func loadJobFromDir(dir string) (*Job, error) {
	data, err := os.ReadFile(filepath.Join(dir, jobFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, filepath.Base(dir))
		}
		return nil, fmt.Errorf("reading job record: %w", err)
	}
	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("parsing job record: %w", err)
	}
	if job.Status == "" {
		job.Status = StatusOpen
	}
	return &job, nil
}

func saveJob(dir string, job *Job) error {
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding job record: %w", err)
	}
	data = append(data, '\n')
	return writeFileAtomic(filepath.Join(dir, jobFile), data)
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temporary file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("setting file mode: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replacing file: %w", err)
	}
	return nil
}

func resolveArtifact(path string) (string, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("resolving artifact: %w", err)
	}
	hash, err := HashFile(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return abs, "", nil
		}
		return "", "", err
	}
	return abs, hash, nil
}

func normalizeScope(scope []string) []string {
	out := make([]string, 0, len(scope))
	seen := make(map[string]bool, len(scope))
	for _, p := range scope {
		p = normalizePath(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func normalizePath(p string) string {
	p = strings.TrimSpace(filepath.ToSlash(p))
	p = strings.TrimPrefix(p, "./")
	p = filepath.ToSlash(filepath.Clean(p))
	if p == "." {
		return ""
	}
	return p
}

func newJobID(objective string) string {
	base := slugify(objective)
	if base == "" {
		base = "job"
	}
	if len(base) > 48 {
		base = strings.Trim(base[:48], "-")
	}
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
	}
	return base + "-" + hex.EncodeToString(buf[:])
}

func uniqueRequirementID(text string, seen map[string]bool) string {
	base := slugify(text)
	if base == "" {
		base = "requirement"
	}
	if len(base) > 64 {
		base = strings.Trim(base[:64], "-")
	}
	id := base
	for i := 2; seen[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

func slugify(s string) string {
	var sb strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
			lastDash = false
		case !lastDash && sb.Len() > 0:
			sb.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(sb.String(), "-")
}
