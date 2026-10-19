package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/stretchr/testify/require"
)

func runTool(t *testing.T, tool fantasy.AgentTool, ctx context.Context, name string, params any) fantasy.ToolResponse {
	t.Helper()
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "test-call", Name: name, Input: string(input)})
	require.NoError(t, err)
	return resp
}

func pathTestPermissions(t *testing.T) (*permission.Service, context.Context) {
	t.Helper()
	svc := permission.NewPermissionService(t.TempDir(), true, nil)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	return &svc, ctx
}

func TestFileToolsAllowAdditionalDir(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	extra := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(extra, "shared.txt"), []byte("shared needle\n"), 0o644))

	perms, ctx := pathTestPermissions(t)
	(*perms).SetAdditionalDirs([]string{extra})
	tracker := &mockEditFileTracker{lastRead: time.Now().Add(time.Hour)}

	view := NewViewTool(nil, *perms, tracker, nil, work)
	resp := runTool(t, view, ctx, ViewToolName, ViewParams{FilePath: filepath.Join(extra, "shared.txt")})
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "shared needle")

	write := NewWriteTool(nil, *perms, &mockHistoryService{}, tracker, work)
	resp = runTool(t, write, ctx, WriteToolName, WriteParams{
		FilePath: filepath.Join(extra, "new.txt"),
		Content:  "created\n",
	})
	require.False(t, resp.IsError, resp.Content)
	data, err := os.ReadFile(filepath.Join(extra, "new.txt"))
	require.NoError(t, err)
	require.Equal(t, "created\n", string(data))

	edit := NewEditTool(nil, *perms, &mockHistoryService{}, tracker, work)
	resp = runTool(t, edit, ctx, EditToolName, EditParams{
		FilePath:  filepath.Join(extra, "shared.txt"),
		OldString: "needle",
		NewString: "thread",
	})
	require.False(t, resp.IsError, resp.Content)

	multi := NewMultiEditTool(nil, *perms, &mockHistoryService{}, tracker, work)
	resp = runTool(t, multi, ctx, MultiEditToolName, MultiEditParams{
		FilePath: filepath.Join(extra, "shared.txt"),
		Edits:    []MultiEditOperation{{OldString: "thread", NewString: "pin"}},
	})
	require.False(t, resp.IsError, resp.Content)

	smart := NewSmartReplaceTool(*perms, work)
	resp = runTool(t, smart, ctx, "smart_replace", SmartReplaceParams{
		FilePath: filepath.Join(extra, "shared.txt"),
		Search:   "pin",
		Replace:  "needle",
	})
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "Strict replace successful")

	grep := NewGrepTool(*perms, work, config.ToolGrep{})
	resp = runTool(t, grep, ctx, GrepToolName, GrepParams{Pattern: "needle", Path: extra})
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "shared.txt")

	glob := NewGlobTool(*perms, work, config.ToolGlob{})
	resp = runTool(t, glob, ctx, GlobToolName, GlobParams{Pattern: "*.txt", Path: extra})
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "shared.txt")

	ls := NewLsTool(*perms, work, config.ToolLs{})
	resp = runTool(t, ls, ctx, LSToolName, LSParams{Path: extra})
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "shared.txt")
}

func TestFileToolsDenyOutsideDirectories(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("secret needle\n"), 0o644))

	perms, ctx := pathTestPermissions(t)
	tracker := &mockEditFileTracker{lastRead: time.Now().Add(time.Hour)}

	assertDenied := func(t *testing.T, resp fantasy.ToolResponse) {
		t.Helper()
		require.True(t, resp.IsError, resp.Content)
		require.Contains(t, resp.Content, "outside the allowed workspace directories")
	}

	assertDenied(t, runTool(t, NewViewTool(nil, *perms, tracker, nil, work), ctx, ViewToolName,
		ViewParams{FilePath: outsideFile}))
	assertDenied(t, runTool(t, NewWriteTool(nil, *perms, &mockHistoryService{}, tracker, work), ctx, WriteToolName,
		WriteParams{FilePath: filepath.Join(outside, "new.txt"), Content: "x"}))
	assertDenied(t, runTool(t, NewEditTool(nil, *perms, &mockHistoryService{}, tracker, work), ctx, EditToolName,
		EditParams{FilePath: outsideFile, OldString: "secret", NewString: "public"}))
	assertDenied(t, runTool(t, NewMultiEditTool(nil, *perms, &mockHistoryService{}, tracker, work), ctx, MultiEditToolName,
		MultiEditParams{FilePath: outsideFile, Edits: []MultiEditOperation{{OldString: "secret", NewString: "public"}}}))
	assertDenied(t, runTool(t, NewSmartReplaceTool(*perms, work), ctx, "smart_replace",
		SmartReplaceParams{FilePath: outsideFile, Search: "secret", Replace: "public"}))
	assertDenied(t, runTool(t, NewGrepTool(*perms, work, config.ToolGrep{}), ctx, GrepToolName,
		GrepParams{Pattern: "secret", Path: outside}))
	assertDenied(t, runTool(t, NewGlobTool(*perms, work, config.ToolGlob{}), ctx, GlobToolName,
		GlobParams{Pattern: "*.txt", Path: outside}))
	assertDenied(t, runTool(t, NewLsTool(*perms, work, config.ToolLs{}), ctx, LSToolName,
		LSParams{Path: outside}))

	patch := "--- a/../outside/secret.txt\n+++ b/../outside/secret.txt\n@@ -1 +1 @@\n-secret needle\n+public needle\n"
	assertDenied(t, runTool(t, NewApplyPatchTool(*perms, work), ctx, "apply_patch",
		ApplyPatchParams{Patch: patch}))
}

func TestPatchTargetPaths(t *testing.T) {
	t.Parallel()

	patch := "--- a/internal/main.go\t2026-10-19\n+++ b/internal/main.go\t2026-10-19\n" +
		"--- /dev/null\n+++ b/new.txt\n"
	require.Equal(t, []string{"internal/main.go", "internal/main.go", "new.txt"}, patchTargetPaths(patch))
}
