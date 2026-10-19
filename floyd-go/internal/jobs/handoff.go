package jobs

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const handoffFile = "handoff.md"

// ExportHandoff writes a portable handoff packet and returns its path.
func ExportHandoff(dataDir, jobID string) (string, error) {
	dir, err := jobDir(dataDir, jobID)
	if err != nil {
		return "", err
	}
	job, err := loadJobFromDir(dir)
	if err != nil {
		return "", err
	}
	status, err := statusFor(dir, job)
	if err != nil {
		return "", err
	}
	entries, err := JournalEntries(dataDir, jobID, 10)
	if err != nil {
		return "", err
	}

	content := renderHandoff(job, status, entries)
	path := filepath.Join(dir, handoffFile)
	if err := writeFileAtomic(path, []byte(content)); err != nil {
		return "", err
	}
	if _, err := Journal(dataDir, jobID, JournalEntry{
		Tool:   "handoff",
		Action: "handoff exported",
		Detail: path,
	}); err != nil {
		return "", err
	}
	return path, nil
}

func renderHandoff(job *Job, status *JobStatus, entries []JournalEntry) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Handoff — %s\n\n", job.ID)
	fmt.Fprintf(&sb, "Objective: %s\n\n", job.Objective)
	fmt.Fprintf(&sb, "Status: %s\n", job.Status)
	if job.Artifact != "" {
		fmt.Fprintf(&sb, "Artifact: %s\n", job.Artifact)
		hash := status.CurrentArtifactHash
		if hash == "" {
			hash = "(unavailable)"
		}
		fmt.Fprintf(&sb, "Current artifact SHA-256: %s\n", hash)
		fmt.Fprintf(&sb, "Created with artifact SHA-256: %s\n", orNone(job.ArtifactHash))
	}
	fmt.Fprintf(&sb, "Created: %s\n", job.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(&sb, "Updated: %s\n\n", job.UpdatedAt.Format(time.RFC3339))

	if len(job.Scope) > 0 {
		sb.WriteString("## Scope\n\n")
		for _, path := range job.Scope {
			fmt.Fprintf(&sb, "- %s\n", path)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Requirements\n\n")
	for _, req := range status.Requirements {
		fmt.Fprintf(&sb, "- [%s] %s — %s\n", strings.ToUpper(req.Status), req.ID, req.Text)
		if req.Reason != "" {
			fmt.Fprintf(&sb, "  - reason: %s\n", req.Reason)
		}
		if len(req.Evidence) > 0 {
			latest := req.Evidence[len(req.Evidence)-1]
			fmt.Fprintf(&sb, "  - latest evidence: exit %d, artifact %s, journal excerpt %s\n",
				latest.ExitCode, shortHash(latest.ArtifactHash), latest.ExcerptPath)
		}
	}

	sb.WriteString("\n## Next action\n\n")
	next := "None — every requirement has passing evidence for the current artifact."
	for _, req := range status.Requirements {
		if req.Status != VerdictPassed {
			next = fmt.Sprintf("Produce passing evidence for %q (%s), then re-run completion.", req.ID, req.Text)
			if req.Reason != "" {
				next += " Current blocker: " + req.Reason + "."
			}
			break
		}
	}
	sb.WriteString(next)
	sb.WriteString("\n\n")

	if len(entries) > 0 {
		sb.WriteString("## Recent journal\n\n")
		for _, entry := range entries {
			fmt.Fprintf(&sb, "- %s %s %s: %s", entry.ID, entry.Timestamp.Format(time.RFC3339), entry.Tool, entry.Action)
			if entry.Detail != "" {
				fmt.Fprintf(&sb, " (%s)", entry.Detail)
			}
			if entry.Result != "" {
				fmt.Fprintf(&sb, " => %s", entry.Result)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Continuation point\n\n")
	fmt.Fprintf(&sb, "Job record: %s\n", filepath.Join(jobDirPath(job.ID), jobFile))
	fmt.Fprintf(&sb, "Evidence: %s/evidence/\n", jobDirPath(job.ID))
	fmt.Fprintf(&sb, "Journal: %s/%s\n", jobDirPath(job.ID), journalFile)
	return sb.String()
}

// jobDirPath is used only for display in the handoff packet; the real
// location is resolved by the caller's data directory.
func jobDirPath(jobID string) string {
	return filepath.ToSlash(filepath.Join("jobs", jobID))
}

func orNone(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
