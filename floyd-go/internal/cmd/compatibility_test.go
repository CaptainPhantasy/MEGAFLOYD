package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/pubsub"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRunOutputStructuredCompletion(t *testing.T) {
	for _, format := range []string{"json", "stream-json"} {
		t.Run(format, func(t *testing.T) {
			var output bytes.Buffer
			writer, err := newRunOutput(&output, format)
			require.NoError(t, err)
			writer.SetSession("session")
			_, err = writer.Write([]byte("hello "))
			require.NoError(t, err)
			_, err = writer.Write([]byte("world\n"))
			require.NoError(t, err)
			if format == "json" {
				require.Empty(t, output.String())
			} else {
				require.Contains(t, output.String(), "text_delta")
			}
			require.NoError(t, writer.Finish(context.Canceled))
			decoder := json.NewDecoder(&output)
			var last map[string]any
			for decoder.More() {
				require.NoError(t, decoder.Decode(&last))
			}
			require.Equal(t, "session", last["session_id"])
			require.Equal(t, "hello world", last["result"])
			require.Equal(t, true, last["is_error"])
			require.Equal(t, "error", last["subtype"])
		})
	}
}

func TestRunStreamCorrelatedLiveDeltas(t *testing.T) {
	var output bytes.Buffer
	s := &runStream{sessionID: "s", runID: "own", out: &output, read: map[string]int{}}
	emit := func(run, session, text string) {
		done, err := s.handle(pubsub.Event[proto.Message]{Payload: proto.Message{ID: "msg", RunID: run, SessionID: session, Role: proto.Assistant, Parts: []proto.ContentPart{proto.TextContent{Text: text}}}}, nil)
		require.NoError(t, err)
		require.False(t, done)
	}
	emit("foreign", "s", "wrong")
	emit("own", "other", "wrong")
	require.Empty(t, output.String())
	emit("own", "s", "live ")
	require.Equal(t, "live ", output.String())
	done, err := s.handle(pubsub.Event[proto.RunComplete]{Payload: proto.RunComplete{RunID: "own", SessionID: "s", MessageID: "msg", Text: "live final"}}, nil)
	require.NoError(t, err)
	require.True(t, done)
	require.Equal(t, "live final", output.String())
}

func TestNormalizeResumeArgsAndLaunchValidation(t *testing.T) {
	require.Equal(t, []string{"--resume=abc", "hello"}, normalizeResumeArgs([]string{"-r", "abc", "hello"}))
	require.Equal(t, []string{"--resume", "--model", "one"}, normalizeResumeArgs([]string{"--resume", "--model", "one"}))
	require.Equal(t, []string{"--", "--resume", "literal"}, normalizeResumeArgs([]string{"--", "--resume", "literal"}))
	c := &cobra.Command{}
	addPromptFlags(c)
	c.Flags().Bool("yolo", false, "")
	require.NoError(t, c.Flags().Set("permission-mode", "plan"))
	require.NoError(t, c.Flags().Set("dangerously-skip-permissions", "true"))
	_, _, err := launchOptions(c, nil)
	require.ErrorContains(t, err, "plan mode")
}

func isolatedManagementCommand(t *testing.T) (*cobra.Command, *bytes.Buffer, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("FLOYD_GLOBAL_CONFIG", filepath.Join(root, "config"))
	t.Setenv("FLOYD_GLOBAL_DATA", filepath.Join(root, "global"))
	t.Setenv("MEGAFLOYD_HOME", filepath.Join(root, "home"))
	c := &cobra.Command{Use: "megafloyd", SilenceErrors: true, SilenceUsage: true}
	c.PersistentFlags().String("cwd", root, "")
	c.PersistentFlags().String("data-dir", filepath.Join(root, "data"), "")
	c.AddCommand(newConfigCommand(), newMCPCommand(), newHooksCommand())
	var out bytes.Buffer
	c.SetOut(&out)
	return c, &out, root
}

func TestConfigurationAndMCPOfflineRoundtrip(t *testing.T) {
	c, out, root := isolatedManagementCommand(t)
	c.SetArgs([]string{"config", "set", "permissions.mode", "manual"})
	require.NoError(t, c.Execute())
	out.Reset()
	c.SetArgs([]string{"config", "get", "permissions.mode"})
	require.NoError(t, c.Execute())
	require.Equal(t, "\"manual\"\n", out.String())
	out.Reset()
	c.SetArgs([]string{"mcp", "add", "fixture", "echo", "hello"})
	require.NoError(t, c.Execute())
	out.Reset()
	c.SetArgs([]string{"mcp", "list"})
	require.NoError(t, c.Execute())
	require.Contains(t, out.String(), "fixture\tstdio\tenabled")
	out.Reset()
	c.SetArgs([]string{"mcp", "disable", "fixture"})
	require.NoError(t, c.Execute())
	out.Reset()
	c.SetArgs([]string{"mcp", "get", "fixture"})
	require.NoError(t, c.Execute())
	require.Contains(t, out.String(), `"disabled": true`)
	before, err := os.ReadFile(filepath.Join(root, "data/floyd.json"))
	require.NoError(t, err)
	c.SetArgs([]string{"config", "set", "options.progress", "not-a-bool"})
	require.Error(t, c.Execute())
	after, err := os.ReadFile(filepath.Join(root, "data/floyd.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestRedactedConfigurationDoesNotExposeCredentials(t *testing.T) {
	data, err := redactedJSON(map[string]any{"providers": map[string]any{"x": map[string]any{"api_key": "sensitive", "oauth_token": map[string]any{"access_token": "sensitive"}}}, "mcp": map[string]any{"x": map[string]any{"headers": map[string]any{"Authorization": "sensitive"}, "env": map[string]any{"PASSWORD": "sensitive"}}}})
	require.NoError(t, err)
	require.NotContains(t, string(data), "sensitive")
	require.Contains(t, string(data), "redacted")
}

func TestHooksOfflineInspectionWithoutServer(t *testing.T) {
	c, out, root := isolatedManagementCommand(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "floyd.json"), []byte(`{"hooks":{"PreToolUse":[{"name":"check","command":"echo private-command","matcher":"^bash$"}]}}`), 0o600))
	c.SetArgs([]string{"hooks", "explain", "PreToolUse", "--tool", "bash", "--json"})
	require.NoError(t, c.Execute())
	require.Contains(t, out.String(), "local_configuration")
	require.NotContains(t, out.String(), "private-command")
	out.Reset()
	c.SetArgs([]string{"hooks", "trace", "--json"})
	require.NoError(t, c.Execute())
	require.NotContains(t, out.String(), "socket")
	_, err := os.Stat(filepath.Join(root, "data/floyd.db"))
	require.True(t, errors.Is(err, os.ErrNotExist))
	require.True(t, strings.Contains(out.String(), "records"))
}

func TestStatsEmptyUnconfiguredWorkspaceNeedsNoProvider(t *testing.T) {
	c, out, root := isolatedManagementCommand(t)
	stats := &cobra.Command{Use: "stats", RunE: runStats}
	stats.Flags().Bool("all", false, "")
	stats.Flags().String("crawl-dir", "", "")
	c.AddCommand(stats)
	c.SetArgs([]string{"stats"})
	require.NoError(t, c.Execute())
	require.Contains(t, out.String(), "No usage yet")
	_, err := os.Stat(filepath.Join(root, "data/floyd.db"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunStreamIgnoresChildCompletionWithSameRunID(t *testing.T) {
	var output bytes.Buffer
	stream := &runStream{sessionID: "parent", runID: "shared", out: &output, read: map[string]int{}}
	done, err := stream.handle(pubsub.Event[proto.RunComplete]{Payload: proto.RunComplete{SessionID: "child", RunID: "shared", MessageID: "child-result", Text: "child text"}}, func() {})
	require.NoError(t, err)
	require.False(t, done)
	require.Empty(t, output.String())
}

func TestRedactedConfigurationMasksURLAndArgumentCredentials(t *testing.T) {
	input := map[string]any{"mcp": map[string]any{"fixture": map[string]any{
		"url":  "https://fixture-user:URL_PASSWORD@example.invalid/mcp?token=QUERY_TOKEN&apiKey=QUERY_KEY&mode=stream#access_token=FRAGMENT_TOKEN",
		"args": []string{"--port", "1234", "--header=Authorization: Bearer ASSIGNED_HEADER", "--user=ASSIGNED_PASSWORD", "--token", "ARG_TOKEN", "--api-key=ARG_KEY", "CLIENT_SECRET=ENV_SECRET", "-H", "Authorization: Bearer HEADER_TOKEN", "--user", "fixture:ARG_PASSWORD", "https://other:ARG_URL_PASSWORD@example.invalid/?key=ARG_QUERY_KEY"},
	}}}
	data, err := redactedJSON(input)
	require.NoError(t, err)
	for _, secret := range []string{"ASSIGNED_HEADER", "ASSIGNED_PASSWORD", "URL_PASSWORD", "QUERY_TOKEN", "QUERY_KEY", "FRAGMENT_TOKEN", "ARG_TOKEN", "ARG_KEY", "ENV_SECRET", "HEADER_TOKEN", "ARG_PASSWORD", "ARG_URL_PASSWORD", "ARG_QUERY_KEY"} {
		require.NotContains(t, string(data), secret)
	}
	require.Contains(t, string(data), "example.invalid/mcp")
	require.Contains(t, string(data), "mode=stream")
	require.Contains(t, string(data), "1234")
	// Redaction changes only the printed copy.
	require.Contains(t, input["mcp"].(map[string]any)["fixture"].(map[string]any)["url"], "URL_PASSWORD")
}
