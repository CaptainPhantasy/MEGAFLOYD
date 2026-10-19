package model

import (
	"os"
	"path/filepath"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/legacy-ai/floyd/internal/commands"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/csync"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/legacy-ai/floyd/internal/version"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

func TestEveryRegisteredBuiltinHasHandler(t *testing.T) {
	t.Parallel()

	for _, spec := range commands.Builtins() {
		handler := builtinHandlerFor(spec.Name)
		require.NotEqual(t, handlerUnknown, handler, "/%s has no handler", spec.Name)
	}
}

func TestRequiredHandlerMappings(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		want builtinHandlerID
	}{
		{"new", handlerNew},
		{"status", handlerStatus},
		{"usage", handlerUsage},
		{"goal", handlerGoal},
	} {
		require.Equal(t, tc.want, builtinHandlerFor(tc.name), tc.name)
	}
}

func TestParseSlashInput(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		input string
		name  string
		args  string
		ok    bool
	}{
		{"", "", "", false},
		{"hello", "", "", false},
		{"/", "", "", false},
		{" /new ", "new", "", true},
		{"/NEW", "new", "", true},
		{"/goal ship the release", "goal", "ship the release", true},
		{"/btw  what is this?", "btw", "what is this?", true},
	} {
		name, args, ok := parseSlashInput(tc.input)
		require.Equal(t, tc.ok, ok, tc.input)
		require.Equal(t, tc.name, name, tc.input)
		require.Equal(t, tc.args, args, tc.input)
	}
}

func TestParseGoalArgs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		args      string
		action    goalAction
		objective string
	}{
		{"", goalShowStatus, ""},
		{"status", goalShowStatus, ""},
		{"cancel", goalCancelGoal, ""},
		{"pause", goalUnsupported, ""},
		{"resume", goalUnsupported, ""},
		{"replace something", goalUnsupported, ""},
		{"next something", goalUnsupported, ""},
		{"Ship feature X", goalStart, "Ship feature X"},
		{"-- cancel the rollout", goalStart, "cancel the rollout"},
	} {
		action, objective := parseGoalArgs(tc.args)
		require.Equal(t, tc.action, action, tc.args)
		require.Equal(t, tc.objective, objective, tc.args)
	}
}

func TestStartRalphGoalWritesState(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	require.NoError(t, startRalphGoal(dataDir, "session-1", "Ship feature X"))

	data, err := os.ReadFile(filepath.Join(dataDir, "ralph-loop.state.yaml"))
	require.NoError(t, err)
	state := string(data)
	require.Contains(t, state, "active: true")
	require.Contains(t, state, "session_id: session-1")
	require.Contains(t, state, "prompt: Ship feature X")
}

func TestBuildUsageReport(t *testing.T) {
	t.Parallel()

	sess := &session.Session{
		ID:               "sess-1",
		Title:            "Work",
		PromptTokens:     100,
		CompletionTokens: 50,
		Cost:             1.25,
	}
	cfg := testConfigWithModel(1000)

	report := buildUsageReport(sess, cfg)
	require.Contains(t, report, "Session: Work")
	require.Contains(t, report, "Prompt tokens:     100")
	require.Contains(t, report, "Completion tokens: 50")
	require.Contains(t, report, "Total tokens:      150")
	require.Contains(t, report, "Context: 150/1000 tokens (15.0%)")
	require.Contains(t, report, "Budget: $1.2500/$10.0000 (12.5%)")
}

func TestBuildStatusReport(t *testing.T) {
	t.Parallel()

	sess := &session.Session{ID: "sess-1", Title: "Work", PromptTokens: 10, CompletionTokens: 5}
	report := buildStatusReport(sess, "test-provider", "test-model", "/tmp/work", true, true, 2)

	require.Contains(t, report, "Floyd "+version.Version)
	require.Contains(t, report, "Session: Work")
	require.Contains(t, report, "Session ID: sess-1")
	require.Contains(t, report, "Working directory: /tmp/work")
	require.Contains(t, report, "Provider: test-provider")
	require.Contains(t, report, "Model: test-model")
	require.Contains(t, report, "Permission mode: Never Ask")
	require.Contains(t, report, "Agent: ready")
	require.Contains(t, report, "Queued prompts: 2")
	require.Contains(t, report, "Tokens: 15")
}

func TestLastAssistantText(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hi"}}},
		{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "first"}}},
		{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "last"}}},
	}
	require.Equal(t, "last", lastAssistantText(msgs))
	require.Equal(t, "", lastAssistantText(nil))
}

func testConfigWithModel(contextWindow int) *config.Config {
	providers := csync.NewMap[string, config.ProviderConfig]()
	providers.Set("test-provider", config.ProviderConfig{
		ID: "test-provider",
		Models: []catwalk.Model{{
			ID:            "test-model",
			ContextWindow: int64(contextWindow),
		}},
	})
	return &config.Config{
		Models: map[config.SelectedModelType]config.SelectedModel{
			config.SelectedModelTypeLarge: {
				Provider: "test-provider",
				Model:    "test-model",
			},
		},
		Providers: providers,
		Options: &config.Options{
			Budget: &config.BudgetConfig{MaxUSD: 10},
			TUI:    &config.TUIOptions{},
		},
	}
}

// builtinTestWorkspace implements only the methods the info and goal
// handlers touch; the embedded nil interface covers the rest.
type builtinTestWorkspace struct {
	workspace.Workspace
	cfg     *config.Config
	workDir string
	skip    bool
}

func (w *builtinTestWorkspace) Config() *config.Config       { return w.cfg }
func (w *builtinTestWorkspace) WorkingDir() string           { return w.workDir }
func (w *builtinTestWorkspace) PermissionSkipRequests() bool { return w.skip }
func (w *builtinTestWorkspace) PermissionSetSkipRequests(v bool) {
	w.skip = v
}
func (w *builtinTestWorkspace) AgentIsReady() bool         { return true }
func (w *builtinTestWorkspace) Store() *config.ConfigStore { return nil }

func newBuiltinTestUI(cfg *config.Config) (*UI, *dialog.Overlay) {
	st := styles.CharmtonePantera()
	ws := &builtinTestWorkspace{cfg: cfg, workDir: "/tmp/work"}
	overlay := dialog.NewOverlay()
	return &UI{
		com:    &common.Common{Workspace: ws, Styles: &st},
		dialog: overlay,
		session: &session.Session{
			ID:               "sess-1",
			Title:            "Work",
			PromptTokens:     100,
			CompletionTokens: 50,
			Cost:             1.25,
		},
	}, overlay
}

func TestRunBuiltinStatusOpensInfoPanel(t *testing.T) {
	t.Parallel()

	ui, overlay := newBuiltinTestUI(testConfigWithModel(1000))
	ui.runBuiltin("status", "", false)

	info, ok := overlay.Dialog(dialog.InfoID).(*dialog.Info)
	require.True(t, ok, "status must open the info panel")
	require.Equal(t, "Status", info.Title())
	require.Contains(t, info.Body(), "Session: Work")
	require.Contains(t, info.Body(), "Provider: test-provider")
}

func TestRunBuiltinUsageOpensInfoPanel(t *testing.T) {
	t.Parallel()

	ui, overlay := newBuiltinTestUI(testConfigWithModel(1000))
	ui.runBuiltin("usage", "", false)

	info, ok := overlay.Dialog(dialog.InfoID).(*dialog.Info)
	require.True(t, ok, "usage must open the info panel")
	require.Contains(t, info.Body(), "Total tokens:      150")
	require.Contains(t, info.Body(), "Context: 150/1000 tokens (15.0%)")
}

func TestRunBuiltinGoalShowsStatusAndStartsLoop(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithModel(1000)
	cfg.Options.DataDirectory = t.TempDir()
	ui, _ := newBuiltinTestUI(cfg)

	statusCmd := ui.runBuiltin("goal", "", false)
	require.NotNil(t, statusCmd)
	statusMsg, ok := statusCmd().(builtinResultMsg)
	require.True(t, ok)
	require.Equal(t, "Goal", statusMsg.Title)
	require.Contains(t, statusMsg.Body, "No active goal")

	cmd := ui.runBuiltin("goal", "Ship the release", false)
	require.NotNil(t, cmd)
	msg, ok := cmd().(builtinResultMsg)
	require.True(t, ok)
	require.NoError(t, msg.Err)
	require.Equal(t, "Ship the release", msg.Send)
	require.Contains(t, msg.Info, "Goal started")

	_, err := os.Stat(filepath.Join(cfg.Options.DataDirectory, "ralph-loop.state.yaml"))
	require.NoError(t, err, "starting a goal must write the ralph state file")
}

func TestRunBuiltinNewMapping(t *testing.T) {
	t.Parallel()

	// /new must route to the session-reset handler, not to a generic or
	// unknown path.
	require.Equal(t, handlerNew, builtinHandlerFor("new"))
}

func TestDispatchSlashInputRejectsBlockedAndUnknown(t *testing.T) {
	t.Parallel()

	ui, _ := newBuiltinTestUI(testConfigWithModel(1000))

	cmd, handled := ui.dispatchSlashInput("/plan do the thing")
	require.True(t, handled, "blocked commands must be reported, not sent to the agent")
	require.NotNil(t, cmd)

	cmd, handled = ui.dispatchSlashInput("/definitely-not-a-command")
	require.False(t, handled)
	require.Nil(t, cmd)
}

func TestDispatchSlashInputRunsBuiltins(t *testing.T) {
	t.Parallel()

	ui, overlay := newBuiltinTestUI(testConfigWithModel(1000))

	cmd, handled := ui.dispatchSlashInput("/version")
	require.True(t, handled)
	require.Nil(t, cmd)
	require.NotNil(t, overlay.Dialog(dialog.InfoID))

	_, handled = ui.dispatchSlashInput("/permission")
	require.True(t, handled)
	require.NotNil(t, overlay.Dialog(dialog.PermissionModeID))

	cmd, handled = ui.dispatchSlashInput("/auto")
	require.True(t, handled)
	require.NotNil(t, cmd)
	ws := ui.com.Workspace.(*builtinTestWorkspace)
	require.True(t, ws.skip, "/auto must enable Never Ask mode")

	_, handled = ui.dispatchSlashInput("/yolo")
	require.True(t, handled)
	require.False(t, ws.skip, "/yolo must toggle Never Ask off")
}

func TestSlashAndPaletteKeyBindings(t *testing.T) {
	t.Parallel()

	km := DefaultKeyMap()
	require.Contains(t, km.Editor.Commands.Keys(), "/")
	require.Contains(t, km.Commands.Keys(), "ctrl+p")
}
