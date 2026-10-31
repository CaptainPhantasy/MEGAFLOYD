package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/agent/prompt"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/filetracker"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

type initializationModel struct {
	fantasy.LanguageModel
	writes []tools.WriteParams
	calls  []fantasy.Call
	fail   bool
}

func (*initializationModel) Provider() string { return "fixture" }
func (*initializationModel) Model() string    { return "fixture" }
func (*initializationModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return &fantasy.Response{Content: fantasy.ResponseContent{fantasy.TextContent{Text: "initialized"}}, FinishReason: fantasy.FinishReasonStop}, nil
}

func (m *initializationModel) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	index := len(m.calls)
	m.calls = append(m.calls, call)
	if m.fail {
		return nil, errors.New("fixture model unavailable")
	}
	return func(yield func(fantasy.StreamPart) bool) {
		if index < len(m.writes) {
			input, _ := json.Marshal(m.writes[index])
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolCall, ID: fmt.Sprintf("initialize-%d", index), ToolCallName: tools.WriteToolName, ToolCallInput: string(input)}) {
				return
			}
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls})
			return
		}
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "done"}) {
			return
		}
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "done", Delta: "Initialization files created."}) {
			return
		}
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "done"}) {
			return
		}
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
	}, nil
}

type initializationCoordinator struct {
	agent.Coordinator
	runner agent.SessionAgent
}

func (c *initializationCoordinator) Run(ctx context.Context, sid, text string, a ...message.Attachment) (*fantasy.AgentResult, error) {
	return c.runner.Run(ctx, agent.SessionAgentCall{SessionID: sid, Prompt: text, Attachments: a})
}
func (*initializationCoordinator) IsBusy() bool                      { return false }
func (*initializationCoordinator) IsSessionBusy(string) bool         { return false }
func (*initializationCoordinator) QueuedPrompts(string) int          { return 0 }
func (*initializationCoordinator) QueuedPromptsList(string) []string { return nil }
func (c *initializationCoordinator) Model() agent.Model              { return c.runner.Model() }

type initializationPermissions struct {
	permission.Service
	requests int
}

func (p *initializationPermissions) Request(ctx context.Context, in permission.CreatePermissionRequest) (bool, error) {
	p.requests++
	return p.Service.Request(ctx, in)
}

func executeInitializationCommands(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var result []tea.Msg
		for _, child := range batch {
			result = append(result, executeInitializationCommands(child)...)
		}
		return result
	}
	return []tea.Msg{msg}
}

func TestInitializeRunsNativeToolsInExistingSession(t *testing.T) {
	for _, mode := range []string{"missing-policy", "existing-policy", "model-failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx := t.Context()
			root := t.TempDir()
			data := filepath.Join(root, ".test-state")
			conn, err := db.Connect(ctx, data)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Release(data)) })
			q := db.New(conn)
			a := app.NewForTest(ctx)
			t.Cleanup(a.ShutdownForTest)
			a.Sessions = session.NewService(q, conn)
			a.Messages = message.NewService(q)
			a.History = history.NewService(q, conn)
			a.FileTracker = filetracker.NewService(q)
			perm := &initializationPermissions{Service: permission.NewPermissionService(root, false, []string{"write"})}
			a.Permissions = perm
			policy := prompt.StandardPolicy()
			policyPath := filepath.Join(root, "MegaFloyd.md")
			ssot := filepath.Join(root, "docs", "Fixture_SSOT.md")
			require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package fixture\n"), 0o600))
			model := &initializationModel{fail: mode == "model-failure"}
			if mode == "existing-policy" {
				require.NoError(t, os.WriteFile(policyPath, []byte(policy), 0o600))
			} else {
				model.writes = append(model.writes, tools.WriteParams{FilePath: policyPath, Content: policy})
			}
			model.writes = append(model.writes, tools.WriteParams{FilePath: ssot, Content: "# Fixture\n\nProject facts derived from main.go.\n"})
			definition := agent.Model{Model: model, CatwalkCfg: catwalk.Model{ContextWindow: 200000, DefaultMaxTokens: 10000}}
			runner := agent.NewSessionAgent(agent.SessionAgentOptions{LargeModel: definition, SmallModel: definition, SystemPrompt: policy, DisableAutoSummarize: true, Sessions: a.Sessions, Messages: a.Messages, History: a.History, WorkingDir: root, Tools: []fantasy.AgentTool{tools.NewWriteTool(nil, perm, a.History, a.FileTracker, root)}})
			a.AgentCoordinator = &initializationCoordinator{runner: runner}
			cfg := testConfigWithModel(200000)
			cfg.Options.DataDirectory = data
			store := config.NewTestStoreWithWorkingDir(cfg, root)
			ws := workspace.NewAppWorkspace(a, store)
			sess, err := a.Sessions.Create(ctx, "Keep this conversation")
			require.NoError(t, err)
			first, err := a.Messages.Create(ctx, sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "KEEP_PRIOR_CONVERSATION"}}})
			require.NoError(t, err)
			u := newBusyUI(&countingWorkspace{ready: true})
			u.com.Workspace = ws
			u.session = &sess
			warmCaches(u, false)
			command, handled := u.dispatchSlashInput("/init")
			require.True(t, handled)
			require.NotNil(t, command)
			built := command().(projectInitializationPromptMsg)
			require.NoError(t, built.Err)
			require.Equal(t, sess.ID, built.SessionID)
			require.NoFileExists(t, filepath.Join(data, config.InitFlagFilename), "queueing a prompt is not initialization")
			_, run := u.Update(built)
			results := executeInitializationCommands(run)
			require.Equal(t, sess.ID, u.session.ID)
			stored, err := a.Messages.Get(ctx, first.ID)
			require.NoError(t, err)
			require.Equal(t, "KEEP_PRIOR_CONVERSATION", stored.Content().Text)
			sessions, err := a.Sessions.List(ctx)
			require.NoError(t, err)
			require.Len(t, sessions, 1)
			require.NoFileExists(t, filepath.Join(data, config.InitFlagFilename))
			if mode == "model-failure" {
				require.NoFileExists(t, policyPath)
				needed, err := ws.ProjectNeedsInitialization()
				require.NoError(t, err)
				require.True(t, needed)
				found := false
				for _, result := range results {
					if info, ok := result.(util.InfoMsg); ok && info.Type == util.InfoTypeError {
						found = true
					}
				}
				require.True(t, found, "execution failure must reach the UI")
				return
			}
			got, err := os.ReadFile(policyPath)
			require.NoError(t, err)
			require.Equal(t, policy, string(got))
			require.FileExists(t, ssot)
			require.Equal(t, len(model.writes), perm.requests)
			var modelInput strings.Builder
			for _, m := range model.calls[0].Prompt {
				for _, part := range m.Content {
					if text, ok := part.(fantasy.TextPart); ok {
						modelInput.WriteString(text.Text)
					}
				}
			}
			require.Contains(t, modelInput.String(), "KEEP_PRIOR_CONVERSATION")
			require.Contains(t, modelInput.String(), policy)
			needed, err := ws.ProjectNeedsInitialization()
			require.NoError(t, err)
			require.False(t, needed, "actual policy artifact satisfies normal discovery")
		})
	}
}

type initializationSessionSpy struct {
	*countingWorkspace
	submittedTo string
	marked      int
	readSession string
	readPaths   []string
}

func (*initializationSessionSpy) InitializePrompt() (string, error) {
	return "Initialize the selected project", nil
}

func (w *initializationSessionSpy) AgentRun(_ context.Context, id, _ string, _ ...message.Attachment) error {
	w.submittedTo = id
	return nil
}
func (w *initializationSessionSpy) MarkProjectInitialized() error { w.marked++; return nil }
func (w *initializationSessionSpy) FileTrackerRecordRead(_ context.Context, sid, path string) {
	w.readSession = sid
	w.readPaths = append(w.readPaths, path)
}

func TestInitializationPromptKeepsSourceSessionAfterNavigation(t *testing.T) {
	w := &initializationSessionSpy{countingWorkspace: &countingWorkspace{ready: true}}
	m := newBusyUI(w.countingWorkspace)
	m.com.Workspace = w
	warmCaches(m, false)
	command := m.initializeProject()
	m.session = &session.Session{ID: "different-session"}
	built := command().(projectInitializationPromptMsg)
	_, run := m.Update(built)
	executeInitializationCommands(run)
	require.Equal(t, "s1", w.submittedTo)
	require.Equal(t, "different-session", m.session.ID)
	require.Zero(t, w.marked)
}

func TestInitializationExecutionKeepsWorkspaceAfterNavigation(t *testing.T) {
	original := &initializationSessionSpy{countingWorkspace: &countingWorkspace{ready: true}}
	other := &initializationSessionSpy{countingWorkspace: &countingWorkspace{ready: true}}
	m := newBusyUI(original.countingWorkspace)
	m.com.Workspace = original
	m.sessionFileReads = []string{"original.go"}
	warmCaches(m, false)
	built := m.initializeProject()().(projectInitializationPromptMsg)
	_, run := m.Update(built)
	// Commands run asynchronously. Navigation can occur after Update has
	// prepared them but before either file tracking or AgentRun executes.
	m.com.Workspace = other
	m.session = &session.Session{ID: "other-session"}
	m.sessionFileReads[0] = "other.go"
	executeInitializationCommands(run)
	require.Equal(t, "s1", original.submittedTo)
	require.Equal(t, "s1", original.readSession)
	require.Equal(t, []string{"original.go"}, original.readPaths)
	require.Empty(t, other.submittedTo)
	require.Empty(t, other.readPaths)
}
