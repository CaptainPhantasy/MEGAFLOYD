package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/skills"
	"github.com/stretchr/testify/require"
)

func TestCanonicalExecutableSkillDiscoveryAndPermission(t *testing.T) {
	dir := t.TempDir()
	index := "version: 1\nskills:\n  - name: canonical-skill\n    description: Canonical skill smoke test\n    runtime:\n      type: bash\n      command: 'printf \"%s|%s|%s\" \"$PWD\" \"$SKILL_PROOF\" \"\x24{PATH:+path-present}\" > result.txt; cat result.txt'\n      env:\n        SKILL_PROOF: wired\n      timeout: 5\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skills.yaml"), []byte(index), 0o600))
	found, states := skills.DiscoverWithStates([]string{dir})
	require.Len(t, found, 1)
	require.Len(t, states, 1)
	require.Equal(t, skills.StateNormal, states[0].State)
	require.Contains(t, skills.ToPromptXML(found), "run_skill")
	perms := &recordingPermissionService{allow: false}
	tool := NewRunSkillTool(perms, dir, found)
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "canonical-skill-session")
	call := fantasy.ToolCall{ID: "skill-call", Name: RunSkillToolName, Input: "{\"name\":\"canonical-skill\"}"}
	denied, err := tool.Run(ctx, call)
	require.NoError(t, err)
	require.True(t, denied.IsError)
	_, err = os.Stat(filepath.Join(dir, "result.txt"))
	require.True(t, os.IsNotExist(err))
	perms.allow = true
	result, err := tool.Run(ctx, call)
	require.NoError(t, err)
	require.False(t, result.IsError, result.Content)
	require.Equal(t, 2, perms.requestCount)
	written, err := os.ReadFile(filepath.Join(dir, "result.txt"))
	require.NoError(t, err)
	resolvedDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	require.Equal(t, resolvedDir+"|wired|path-present", string(written))
}
