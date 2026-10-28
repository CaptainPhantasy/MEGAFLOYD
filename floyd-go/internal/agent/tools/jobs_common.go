package tools

import (
	"context"
	"fmt"
	"path/filepath"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/jobs"
	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/legacy-ai/floyd/internal/permission"
)

// jobsDataDirectory resolves the directory that owns the jobs/ subtree. An
// explicit data directory wins; otherwise job records follow the same
// account-private workspace store as the scratchpad, so one person's job
// records are never discovered inside a shared project directory.
func jobsDataDirectory(dataDir, workingDir string) (string, error) {
	if dataDir != "" {
		return dataDir, nil
	}
	return megafloyd.PersonalDataDir(workingDir, "")
}

// requestJobsPermission routes a job-record mutation through the same
// permission path as the write tool. A nil service (tests) is permissive.
func requestJobsPermission(
	ctx context.Context,
	permissions permission.Service,
	call fantasy.ToolCall,
	toolName, action, path, description string,
	params any,
) (bool, error) {
	if permissions == nil {
		return true, nil
	}
	sessionID := GetSessionFromContext(ctx)
	if sessionID == "" {
		return false, fmt.Errorf("session_id is required")
	}
	return permissions.Request(ctx, permission.CreatePermissionRequest{
		SessionID:   sessionID,
		Path:        path,
		ToolCallID:  call.ID,
		ToolName:    toolName,
		Action:      action,
		Description: description,
		Params:      params,
	})
}

// artifactHashFor resolves the hash evidence must be bound to. An explicit
// hash wins; an explicit path is hashed; otherwise the job's artifact is used.
func artifactHashFor(workingDir, artifactPath, explicitHash, jobArtifact string) (string, error) {
	if explicitHash != "" {
		return explicitHash, nil
	}
	if artifactPath != "" {
		if !filepath.IsAbs(artifactPath) {
			artifactPath = filepath.Join(workingDir, artifactPath)
		}
		return jobs.HashFile(artifactPath)
	}
	if jobArtifact != "" {
		return jobs.HashFile(jobArtifact)
	}
	return "", jobs.ErrNoArtifact
}

func shortJobHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	if hash == "" {
		return "(none)"
	}
	return hash
}
