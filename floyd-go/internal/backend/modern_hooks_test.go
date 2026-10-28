package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/hookstore"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func TestModernHooksDirectShellUsesWorkspaceDispatcher(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "rewritten_success", true: "rejected"}[blocked], func(t *testing.T) {
			b, _ := newTestBackend(t)
			cwd := t.TempDir()
			ws, _ := insertTestWorkspace(t, b, cwd)
			dataDir := t.TempDir()
			conn, err := db.Connect(t.Context(), dataDir)
			require.NoError(t, err)
			defer db.Release(dataDir)
			pre := `echo '{"updated_input":{"command":"printf rewritten > changed.txt; printf done"}}'`
			if blocked {
				pre = `echo '{"decision":"deny","reason":"fixture denies direct shell"}'`
			}
			cfg := &config.Config{Options: &config.Options{DataDirectory: dataDir}, Hooks: map[string][]config.HookConfig{
				hooks.EventPreToolUse: {{Command: pre}}, hooks.EventPostToolUse: {{Command: `echo '{}'`}}, hooks.EventActionRejected: {{Command: `echo '{}'`}},
			}}
			require.NoError(t, cfg.ValidateHooks())
			store := config.NewTestStoreWithWorkingDir(cfg, cwd)
			runtime := hookruntime.New(t.Context(), store, hookstore.New(conn))
			defer runtime.Close(context.Background())
			q := db.New(conn)
			sessions := session.NewService(q, conn)
			messages := message.NewService(q)
			ws.App = &app.App{Hooks: runtime, Messages: messages, Sessions: sessions}
			ws.Cfg = store
			sess, err := sessions.Create(t.Context(), "direct shell fixture")
			require.NoError(t, err)
			response, err := b.RunShellCommand(t.Context(), ws.ID, proto.ShellCommandRequest{SessionID: sess.ID, Command: "printf original > original.txt"})
			if blocked {
				require.ErrorContains(t, err, "fixture denies")
				require.NoFileExists(t, filepath.Join(cwd, "changed.txt"))
			} else {
				require.NoError(t, err)
				require.Zero(t, response.ExitCode)
				require.Contains(t, response.Output, "done")
				content, err := os.ReadFile(filepath.Join(cwd, "changed.txt"))
				require.NoError(t, err)
				require.Equal(t, "rewritten", string(content))
			}
			require.NoFileExists(t, filepath.Join(cwd, "original.txt"))
			page, err := runtime.Store().List(t.Context(), hookstore.Query{SessionID: sess.ID})
			require.NoError(t, err)
			// Every configured hook appends a start and a terminal record under
			// one execution ID. The accepted rewrite also runs through the
			// mechanical dispatcher, which retains two native receipts: the
			// registered schema check and the tool result. A rejected rewrite
			// never reaches the tool body, so it only has hook records.
			wantHooks := []string{
				"hook[0]", "hook[0]", "registered-tool-schema", "native-tool-result", "hook[0]", "hook[0]",
			}
			wantEvents := []string{
				hooks.EventPreToolUse, hooks.EventPreToolUse, hooks.EventPreToolUse,
				hooks.EventPostToolUse, hooks.EventPostToolUse, hooks.EventPostToolUse,
			}
			if blocked {
				wantHooks = []string{"hook[0]", "hook[0]", "hook[0]", "hook[0]"}
				wantEvents = []string{
					hooks.EventPreToolUse, hooks.EventPreToolUse,
					hooks.EventActionRejected, hooks.EventActionRejected,
				}
			}
			require.Len(t, page.Records, len(wantEvents))
			for i, record := range page.Records {
				require.Equal(t, wantHooks[i], record.HookName, "record %d hook", i)
				require.Equal(t, wantEvents[i], record.Event, "record %d event", i)
			}
			require.Equal(t, "running", page.Records[0].State)
			require.Equal(t, "succeeded", page.Records[len(page.Records)-1].State)
			require.NotEmpty(t, page.Records[0].ToolCallID)
			require.NotEmpty(t, page.Records[0].RunID)
			if !blocked {
				require.Equal(t, "builtin", page.Records[2].Mode)
				require.Equal(t, "builtin", page.Records[3].Mode)
			}
			msgs, err := messages.List(t.Context(), sess.ID)
			require.NoError(t, err)
			if blocked {
				require.Empty(t, msgs)
			} else {
				require.NotEmpty(t, msgs)
			}
		})
	}
}
