package jobs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeArtifact(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func createTestJob(t *testing.T, dataDir, artifact string, requirements ...string) *Job {
	t.Helper()
	job, err := Create(dataDir, CreateOptions{
		Objective:    "Ship the thing",
		Requirements: requirements,
		Artifact:     artifact,
		Scope:        []string{"internal/jobs"},
	})
	require.NoError(t, err)
	return job
}

func TestCreateWritesRecord(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	artifact := writeArtifact(t, t.TempDir(), "app.go", "package main\n")

	job, err := Create(dataDir, CreateOptions{
		ID:           "job-1",
		Objective:    "Ship the thing",
		Requirements: []string{"Tests pass", "Build clean"},
		Artifact:     artifact,
		Scope:        []string{"./internal/jobs/", "internal/jobs"},
	})
	require.NoError(t, err)

	assert.Equal(t, "job-1", job.ID)
	assert.Equal(t, "Ship the thing", job.Objective)
	assert.Equal(t, StatusOpen, job.Status)
	require.Len(t, job.Requirements, 2)
	assert.Equal(t, "tests-pass", job.Requirements[0].ID)
	assert.Equal(t, "build-clean", job.Requirements[1].ID)
	assert.Equal(t, []string{"internal/jobs"}, job.Scope)
	assert.Equal(t, HashBytes([]byte("package main\n")), job.ArtifactHash)

	jobPath := filepath.Join(dataDir, "jobs", "job-1", "job.json")
	data, err := os.ReadFile(jobPath)
	require.NoError(t, err)
	var persisted Job
	require.NoError(t, json.Unmarshal(data, &persisted))
	assert.Equal(t, job.Objective, persisted.Objective)

	entries, err := JournalEntries(dataDir, "job-1", 0)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "contract created", entries[0].Action)
}

func TestCreateValidation(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()

	tests := []struct {
		name string
		opts CreateOptions
		want error
	}{
		{
			name: "missing objective",
			opts: CreateOptions{Requirements: []string{"x"}},
		},
		{
			name: "missing requirements",
			opts: CreateOptions{Objective: "x"},
			want: ErrNoRequirements,
		},
		{
			name: "path traversal id",
			opts: CreateOptions{ID: "../escape", Objective: "x", Requirements: []string{"y"}},
			want: ErrInvalidID,
		},
		{
			name: "blank requirement",
			opts: CreateOptions{ID: "ok", Objective: "x", Requirements: []string{"  "}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Create(dataDir, tt.opts)
			require.Error(t, err)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			}
		})
	}

	_, err := Create(dataDir, CreateOptions{ID: "dup", Objective: "x", Requirements: []string{"y"}})
	require.NoError(t, err)
	_, err = Create(dataDir, CreateOptions{ID: "dup", Objective: "x", Requirements: []string{"z"}})
	require.ErrorIs(t, err, ErrExists)
}

func TestLoadNotFoundAndInvalidID(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	_, err := Load(dataDir, "missing")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = Load(dataDir, "../../etc/passwd")
	require.ErrorIs(t, err, ErrInvalidID)
}

func TestListNewestFirst(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	first, err := Create(dataDir, CreateOptions{ID: "first", Objective: "one", Requirements: []string{"a"}})
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	_, err = Create(dataDir, CreateOptions{ID: "second", Objective: "two", Requirements: []string{"b"}})
	require.NoError(t, err)

	list, err := List(dataDir)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "second", list[0].ID)
	assert.Equal(t, first.ID, list[1].ID)
}

func TestListEmpty(t *testing.T) {
	t.Parallel()
	list, err := List(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestAddRequirement(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	job := createTestJob(t, dataDir, "", "First check")

	req, err := AddRequirement(dataDir, job.ID, "Second check")
	require.NoError(t, err)
	assert.Equal(t, "second-check", req.ID)

	req, err = AddRequirement(dataDir, job.ID, "First check")
	require.NoError(t, err)
	assert.Equal(t, "first-check-2", req.ID)

	loaded, err := Load(dataDir, job.ID)
	require.NoError(t, err)
	require.Len(t, loaded.Requirements, 3)

	entries, err := JournalEntries(dataDir, job.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, "requirement added", entries[len(entries)-1].Action)

	loaded.Status = StatusComplete
	require.NoError(t, saveJob(filepath.Join(dataDir, "jobs", job.ID), loaded))
	_, err = AddRequirement(dataDir, job.ID, "Third")
	require.ErrorIs(t, err, ErrCompleted)
}

func TestRecordEvidenceWritesReceipt(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	artifact := writeArtifact(t, t.TempDir(), "app.go", "package main\n")
	job := createTestJob(t, dataDir, artifact, "Tests pass")
	output := "ok  github.com/legacy-ai/floyd/internal/jobs"

	ev, err := RecordEvidence(dataDir, job.ID, EvidenceInput{
		Requirement: "tests-pass",
		Command:     "go test ./internal/jobs",
		ExitCode:    0,
		Output:      output,
	})
	require.NoError(t, err)
	assert.True(t, ev.Passed)
	assert.Equal(t, HashBytes([]byte(output)), ev.OutputHash)
	assert.Equal(t, HashBytes([]byte("package main\n")), ev.ArtifactHash)
	assert.Equal(t, "evidence/tests-pass/1.output.txt", ev.ExcerptPath)

	jobDir := filepath.Join(dataDir, "jobs", job.ID)
	excerpt, err := os.ReadFile(filepath.Join(jobDir, "evidence", "tests-pass", "1.output.txt"))
	require.NoError(t, err)
	assert.Equal(t, output, string(excerpt))

	data, err := os.ReadFile(filepath.Join(jobDir, "evidence", "tests-pass", "1.json"))
	require.NoError(t, err)
	var receipt Evidence
	require.NoError(t, json.Unmarshal(data, &receipt))
	assert.Equal(t, *ev, receipt)

	ev2, err := RecordEvidence(dataDir, job.ID, EvidenceInput{
		Requirement: "tests-pass",
		Command:     "go test ./internal/jobs",
		ExitCode:    1,
		Output:      "FAIL",
	})
	require.NoError(t, err)
	assert.False(t, ev2.Passed)
	assert.FileExists(t, filepath.Join(jobDir, "evidence", "tests-pass", "2.json"))

	entries, err := JournalEntries(dataDir, job.ID, 0)
	require.NoError(t, err)
	var recorded int
	for _, entry := range entries {
		if entry.Action == "evidence recorded" {
			recorded++
		}
	}
	assert.Equal(t, 2, recorded)
}

func TestRecordEvidenceRequiresArtifactHash(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	job := createTestJob(t, dataDir, "", "Tests pass")

	_, err := RecordEvidence(dataDir, job.ID, EvidenceInput{
		Requirement: "tests-pass",
		Command:     "true",
	})
	require.ErrorIs(t, err, ErrNoArtifact)

	ev, err := RecordEvidence(dataDir, job.ID, EvidenceInput{
		Requirement:  "tests-pass",
		Command:      "true",
		ArtifactHash: strings.Repeat("a", 64),
	})
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("a", 64), ev.ArtifactHash)
}

func TestRecordEvidenceRejectsUnknownRequirementAndCompletedJob(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	job := createTestJob(t, dataDir, "", "Tests pass")

	_, err := RecordEvidence(dataDir, job.ID, EvidenceInput{
		Requirement:  "nope",
		Command:      "true",
		ArtifactHash: strings.Repeat("b", 64),
	})
	require.ErrorIs(t, err, ErrRequirementAbsent)

	loaded, err := Load(dataDir, job.ID)
	require.NoError(t, err)
	loaded.Status = StatusComplete
	require.NoError(t, saveJob(filepath.Join(dataDir, "jobs", job.ID), loaded))

	_, err = RecordEvidence(dataDir, job.ID, EvidenceInput{
		Requirement:  "tests-pass",
		Command:      "true",
		ArtifactHash: strings.Repeat("b", 64),
	})
	require.ErrorIs(t, err, ErrCompleted)
}

func TestStatusVerdicts(t *testing.T) {
	t.Parallel()

	t.Run("unchecked without evidence", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		artifact := writeArtifact(t, t.TempDir(), "app.go", "v1")
		job := createTestJob(t, dataDir, artifact, "Tests pass")

		status, err := Status(dataDir, job.ID)
		require.NoError(t, err)
		require.Len(t, status.Requirements, 1)
		assert.Equal(t, VerdictUnchecked, status.Requirements[0].Status)
		assert.False(t, status.Complete)
	})

	t.Run("passed and failed follow exit code", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		artifact := writeArtifact(t, t.TempDir(), "app.go", "v1")
		job := createTestJob(t, dataDir, artifact, "Tests pass")

		_, err := RecordEvidence(dataDir, job.ID, EvidenceInput{
			Requirement: "tests-pass", Command: "go test", ExitCode: 0,
		})
		require.NoError(t, err)
		status, err := Status(dataDir, job.ID)
		require.NoError(t, err)
		assert.Equal(t, VerdictPassed, status.Requirements[0].Status)
		assert.True(t, status.Complete)

		_, err = RecordEvidence(dataDir, job.ID, EvidenceInput{
			Requirement: "tests-pass", Command: "go test", ExitCode: 1,
		})
		require.NoError(t, err)
		status, err = Status(dataDir, job.ID)
		require.NoError(t, err)
		assert.Equal(t, VerdictFailed, status.Requirements[0].Status)
		assert.False(t, status.Complete)
		assert.Contains(t, status.Requirements[0].Reason, "exit 1")

		_, err = RecordEvidence(dataDir, job.ID, EvidenceInput{
			Requirement: "tests-pass", Command: "go test", ExitCode: 0,
		})
		require.NoError(t, err)
		status, err = Status(dataDir, job.ID)
		require.NoError(t, err)
		assert.Equal(t, VerdictPassed, status.Requirements[0].Status)
	})
}

// TestEvidenceInvalidationOnArtifactChange is the core rule: a passing proof
// is valid only for the bytes it observed.
func TestEvidenceInvalidationOnArtifactChange(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	artifact := writeArtifact(t, t.TempDir(), "app.go", "v1")
	job := createTestJob(t, dataDir, artifact, "Tests pass", "Build clean")

	for _, req := range []string{"tests-pass", "build-clean"} {
		_, err := RecordEvidence(dataDir, job.ID, EvidenceInput{
			Requirement: req, Command: "go test", ExitCode: 0,
		})
		require.NoError(t, err)
	}

	report, err := CheckCompletion(dataDir, job.ID)
	require.NoError(t, err)
	assert.True(t, report.Complete)
	assert.Empty(t, report.Blocking)
	assert.Equal(t, HashBytes([]byte("v1")), report.ArtifactHash)

	require.NoError(t, os.WriteFile(artifact, []byte("v2"), 0o644))

	report, err = CheckCompletion(dataDir, job.ID)
	require.NoError(t, err)
	assert.False(t, report.Complete)
	require.Len(t, report.Blocking, 2)
	assert.Contains(t, report.Blocking[0], "artifact changed")
	require.Len(t, report.Requirements, 2)
	for _, req := range report.Requirements {
		assert.Equal(t, VerdictUnchecked, req.Status)
		assert.Contains(t, req.Reason, "artifact changed")
	}
	assert.Equal(t, HashBytes([]byte("v2")), report.ArtifactHash)

	_, err = RecordEvidence(dataDir, job.ID, EvidenceInput{
		Requirement: "tests-pass", Command: "go test", ExitCode: 0,
	})
	require.NoError(t, err)
	report, err = CheckCompletion(dataDir, job.ID)
	require.NoError(t, err)
	assert.False(t, report.Complete, "one refreshed proof must not complete the whole job")

	_, err = RecordEvidence(dataDir, job.ID, EvidenceInput{
		Requirement: "build-clean", Command: "go build", ExitCode: 0,
	})
	require.NoError(t, err)
	report, err = CheckCompletion(dataDir, job.ID)
	require.NoError(t, err)
	assert.True(t, report.Complete)
	assert.Equal(t, HashBytes([]byte("v2")), report.ArtifactHash)
}

func TestMissingArtifactInvalidatesEvidence(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	artifactDir := t.TempDir()
	artifact := writeArtifact(t, artifactDir, "app.go", "v1")
	job := createTestJob(t, dataDir, artifact, "Tests pass")

	_, err := RecordEvidence(dataDir, job.ID, EvidenceInput{
		Requirement: "tests-pass", Command: "go test", ExitCode: 0,
	})
	require.NoError(t, err)
	require.NoError(t, os.Remove(artifact))

	status, err := Status(dataDir, job.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, status.ArtifactError)
	assert.Equal(t, VerdictUnchecked, status.Requirements[0].Status)
	assert.Contains(t, status.Requirements[0].Reason, "artifact unavailable")
}

func TestMarkCompleteRefusesWithoutEvidence(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	artifact := writeArtifact(t, t.TempDir(), "app.go", "v1")
	job := createTestJob(t, dataDir, artifact, "Tests pass")

	_, err := MarkComplete(dataDir, job.ID)
	require.ErrorIs(t, err, ErrIncomplete)
	loaded, err := Load(dataDir, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, loaded.Status)

	_, err = RecordEvidence(dataDir, job.ID, EvidenceInput{
		Requirement: "tests-pass", Command: "go test", ExitCode: 0,
	})
	require.NoError(t, err)
	completed, err := MarkComplete(dataDir, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusComplete, completed.Status)

	report, err := CheckCompletion(dataDir, job.ID)
	require.NoError(t, err)
	assert.True(t, report.Complete)
}

func TestJournalAndWhy(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	job := createTestJob(t, dataDir, "", "Tests pass")

	entry, err := Journal(dataDir, job.ID, JournalEntry{
		Tool:   "bash",
		Action: "ran tests",
		Detail: "go test ./...",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, entry.ID)
	assert.Equal(t, job.ID, entry.JobID)
	assert.False(t, entry.Timestamp.IsZero())

	found, err := Why(dataDir, job.ID, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, "ran tests", found.Action)

	_, err = Why(dataDir, job.ID, "j-does-not-exist")
	require.ErrorIs(t, err, ErrUnknownAction)
	_, err = Why(dataDir, job.ID, "")
	require.ErrorIs(t, err, ErrUnknownAction)
}

func TestJournalEntriesLimit(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	job := createTestJob(t, dataDir, "", "Tests pass")

	for _, action := range []string{"one", "two", "three"} {
		_, err := Journal(dataDir, job.ID, JournalEntry{Action: action})
		require.NoError(t, err)
	}
	entries, err := JournalEntries(dataDir, job.ID, 2)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "two", entries[0].Action)
	assert.Equal(t, "three", entries[1].Action)
}

func TestExportHandoff(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	artifact := writeArtifact(t, t.TempDir(), "app.go", "v1")
	job := createTestJob(t, dataDir, artifact, "Tests pass")

	path, err := ExportHandoff(dataDir, job.ID)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dataDir, "jobs", job.ID, "handoff.md"), path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	for _, want := range []string{
		"# Handoff — " + job.ID,
		"Ship the thing",
		"[UNCHECKED] tests-pass",
		"Produce passing evidence",
		"Continuation point",
	} {
		assert.Contains(t, content, want)
	}

	entries, err := JournalEntries(dataDir, job.ID, 0)
	require.NoError(t, err)
	var exported bool
	for _, entry := range entries {
		if entry.Action == "handoff exported" {
			exported = true
		}
	}
	assert.True(t, exported)
}

func TestCheckDrift(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	artifact := writeArtifact(t, t.TempDir(), "app.go", "v1")
	job, err := Create(dataDir, CreateOptions{
		Objective:    "Scope the change",
		Requirements: []string{"Tests pass"},
		Artifact:     artifact,
		Scope:        []string{"internal/jobs", "README.md"},
	})
	require.NoError(t, err)

	violations, err := CheckDrift(dataDir, job.ID, []string{
		"internal/jobs/jobs.go",
		"internal/agent/coordinator.go",
		"README.md",
		"./README.md",
	})
	require.NoError(t, err)
	require.Len(t, violations, 1)
	assert.Equal(t, "internal/agent/coordinator.go", violations[0].Path)
	assert.Equal(t, "outside declared scope", violations[0].Reason)

	noScope, err := Create(dataDir, CreateOptions{
		ID: "no-scope", Objective: "No scope", Requirements: []string{"x"},
	})
	require.NoError(t, err)
	violations, err = CheckDrift(dataDir, noScope.ID, []string{"anything.go"})
	require.NoError(t, err)
	require.Len(t, violations, 1)
	assert.Equal(t, "no scope declared in contract", violations[0].Reason)

	_, err = CheckDrift(dataDir, "missing", []string{"x"})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestHashFile(t *testing.T) {
	t.Parallel()
	path := writeArtifact(t, t.TempDir(), "app.go", "content")
	hash, err := HashFile(path)
	require.NoError(t, err)
	assert.Equal(t, HashBytes([]byte("content")), hash)
	_, err = HashFile(filepath.Join(t.TempDir(), "missing"))
	require.True(t, errors.Is(err, os.ErrNotExist))
}
