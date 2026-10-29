package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/client"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hookstore"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/server"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newHooksCommand())
}

func newHooksCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "hooks", Short: "Inspect configured hooks and recorded outcomes without executing hooks",
	}
	var configPath, toolName string
	var explainJSON bool
	explain := &cobra.Command{
		Use: "explain <event>", Args: cobra.ExactArgs(1),
		Short: "Preview matching hooks and their capabilities",
		Long:  "Read the active workspace's effective hooks without starting a server or agent. Use --config to inspect a JSON configuration snapshot without running shell configuration or hook commands.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var cfg *config.Config
			var cwd, source string
			if configPath != "" {
				data, err := os.ReadFile(configPath)
				if err != nil {
					return fmt.Errorf("read hook configuration snapshot: %w", err)
				}
				cfg = &config.Config{}
				if err := json.Unmarshal(data, cfg); err != nil {
					return fmt.Errorf("decode JSON configuration snapshot: %w", err)
				}
				if err := cfg.ValidateHooks(); err != nil {
					return err
				}
				source = "json_snapshot"
			} else {
				c, ws, err := hookInspectionWorkspace(cmd)
				if err != nil {
					return err
				}
				cfg, err = c.GetConfig(cmd.Context(), ws.ID)
				if err != nil {
					return err
				}
				cwd, source = ws.Path, "effective_runtime"
			}
			result, err := tools.ExplainHooks(cfg, args[0], toolName, cwd, source)
			if err != nil {
				return err
			}
			if explainJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s for tool %q (%s; no commands executed)\n", result.Event, result.ToolName, result.Source)
			if len(result.Entries) == 0 {
				_, err = fmt.Fprintln(out, "No hooks configured for this event.")
				return err
			}
			writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "CONFIG KEY\tORDER\tMODE\tMATCH\tBLOCK\tAPPROVE\tREWRITE\tOBSERVE\tNAME / STATUS")
			for _, row := range result.Entries {
				status := row.Name
				if row.DuplicateOf != nil {
					status += fmt.Sprintf(" (duplicate of index %d)", *row.DuplicateOf)
				}
				if row.Error != "" {
					status += " (" + row.Error + ")"
				}
				fmt.Fprintf(writer, "%s\t%d\t%s\t%t\t%t\t%t\t%t\t%t\t%s\n", row.ConfigKey, row.Order, row.Mode, row.Matched, row.CanBlock, row.CanApprove, row.CanRewrite, row.Observational, status)
			}
			if err := writer.Flush(); err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, "Order is composition order for parallel hooks; ordered hooks execute in order. Capabilities are not predicted outcomes. Use --json for command hashes and full fields.")
			return err
		},
	}
	explain.Flags().StringVar(&toolName, "tool", "", "Exact tool name to match")
	explain.Flags().StringVar(&configPath, "config", "", "Read a JSON config snapshot without shell expansion or configuration merging")
	explain.Flags().BoolVar(&explainJSON, "json", false, "Output JSON")

	var params tools.HookTraceParams
	var traceJSON bool
	trace := &cobra.Command{
		Use: "trace", Args: cobra.NoArgs,
		Short: "Read persisted hook events and job outcomes",
		Long:  "Read metadata from the active workspace's own history database. For an offline workspace, use --data-dir with its existing native data directory. This command never creates a workspace, migrates a database, or starts an agent.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			query, err := params.Query("")
			if err != nil {
				return err
			}
			dataDir, _ := cmd.Flags().GetString("data-dir")
			if dataDir == "" {
				_, ws, err := hookInspectionWorkspace(cmd)
				if err != nil {
					return err
				}
				dataDir = ws.DataDir
				if dataDir == "" {
					return errors.New("active workspace did not report its data directory; use --data-dir")
				}
			}
			reader, closer, err := hookstore.OpenReader(dataDir)
			if err != nil {
				return err
			}
			if closer != nil {
				defer closer.Close()
			}
			page, err := reader.List(cmd.Context(), query)
			if err != nil {
				return fmt.Errorf("read hook history: %w", err)
			}
			if traceJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(page)
			}
			out := cmd.OutOrStdout()
			if len(page.Records) == 0 {
				_, err = fmt.Fprintln(out, "No matching hook records. This does not prove that no action occurred before recording was available.")
				return err
			}
			writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "SEQ\tEVENT\tOUTCOME\tSTATE\tDECISION\tSESSION\tTOOL CALL\tEVENT ID\tJOB ID")
			for _, record := range page.Records {
				fmt.Fprintf(writer, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", record.Seq, record.Event, record.Outcome, record.State, record.Decision, record.SessionID, record.ToolCallID, record.EventID, record.JobID)
			}
			if err := writer.Flush(); err != nil {
				return err
			}
			if page.HasMore {
				_, err = fmt.Fprintf(out, "More records: repeat with --after %d and the same filters.\n", page.Next)
			}
			return err
		},
	}
	trace.Flags().StringVar(&params.SessionID, "session", "", "Filter by exact session ID")
	trace.Flags().StringVar(&params.ToolCallID, "tool-call", "", "Filter by exact tool call ID")
	trace.Flags().StringVar(&params.EventID, "event", "", "Filter by exact event ID")
	trace.Flags().StringVar(&params.JobID, "job", "", "Filter by exact asynchronous job ID")
	trace.Flags().Int64Var(&params.After, "after", 0, "Exclusive sequence cursor")
	trace.Flags().IntVar(&params.Limit, "limit", 100, "Page size, 1 through 500")
	trace.Flags().BoolVar(&traceJSON, "json", false, "Output JSON")
	command.AddCommand(explain, trace)
	return command
}

func hookInspectionWorkspace(cmd *cobra.Command) (*client.Client, proto.Workspace, error) {
	cwd, _ := cmd.Flags().GetString("cwd")
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, proto.Workspace{}, err
		}
	}
	cwd, err := canonicalHookInspectionPath(cwd)
	if err != nil {
		return nil, proto.Workspace{}, err
	}
	host, _ := cmd.Flags().GetString("host")
	if host == "" {
		host = server.DefaultHost()
	}
	address, err := server.ParseHostURL(host)
	if err != nil {
		return nil, proto.Workspace{}, err
	}
	c, err := client.NewClient(cwd, address.Scheme, address.Host)
	if err != nil {
		return nil, proto.Workspace{}, err
	}
	workspaces, err := c.ListWorkspaces(cmd.Context())
	if err != nil {
		return nil, proto.Workspace{}, fmt.Errorf("read active workspace: %w; explain also accepts --config JSON, and trace accepts --data-dir", err)
	}
	dataDir, _ := cmd.Flags().GetString("data-dir")
	var matched *proto.Workspace
	for _, ws := range workspaces {
		path, err := canonicalHookInspectionPath(ws.Path)
		if err != nil || path != cwd {
			continue
		}
		if dataDir != "" {
			requested, err := canonicalHookInspectionPath(dataDir)
			if err != nil {
				return nil, proto.Workspace{}, err
			}
			actual, err := canonicalHookInspectionPath(ws.DataDir)
			if err != nil || requested != actual {
				continue
			}
		}
		if matched != nil {
			return nil, proto.Workspace{}, errors.New("more than one active workspace matches; supply its --data-dir")
		}
		copy := ws
		matched = &copy
	}
	if matched == nil {
		return nil, proto.Workspace{}, errors.New("no active workspace matches --cwd; explain accepts --config JSON, and trace accepts its existing --data-dir")
	}
	return c, *matched, nil
}

func canonicalHookInspectionPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("workspace path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}
