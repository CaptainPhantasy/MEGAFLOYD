package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/legacy-ai/floyd/internal/agent/tools/mcp"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
)

func inspectCommandConfig(cmd *cobra.Command) (*config.Config, []string, error) {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return nil, nil, err
	}
	data, _ := cmd.Flags().GetString("data-dir")
	return config.Inspect(cmd.Context(), cwd, data)
}

func configScope(cmd *cobra.Command) (config.Scope, error) {
	scope, _ := cmd.Flags().GetString("scope")
	switch scope {
	case "workspace", "local", "project":
		return config.ScopeWorkspace, nil
	case "global", "user":
		return config.ScopeGlobal, nil
	default:
		return config.ScopeWorkspace, fmt.Errorf("unknown configuration scope %q", scope)
	}
}

func setCommandConfig(cmd *cobra.Command, key string, value any) error {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return err
	}
	data, _ := cmd.Flags().GetString("data-dir")
	scope, err := configScope(cmd)
	if err != nil {
		return err
	}
	path, err := config.EditField(cmd.Context(), cwd, data, scope, key, value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Saved %s in %s\n", key, path)
	return err
}

func redactedJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var object any
	if err = json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for key, value := range x {
				lower := strings.ToLower(key)
				if credentialName(key) || lower == "headers" || lower == "extra_headers" || lower == "env" {
					if value != nil && value != "" {
						x[key] = "[redacted]"
					}
					continue
				}
				if lower == "args" || lower == "arguments" {
					if args, ok := value.([]any); ok {
						redactCredentialArgs(args)
					}
				}
				x[key] = walk(value)
			}
		case []any:
			for i, value := range x {
				x[i] = walk(value)
			}
		case string:
			return redactCredentialURL(x)
		}
		return v
	}
	object = walk(object)
	return json.MarshalIndent(object, "", "  ")
}

// Printed configuration must not disclose credentials transported outside the
// conventional api_key/env/headers fields (for example MCP URLs and argv).
func credentialName(name string) bool {
	name = strings.ToLower(strings.TrimLeft(name, "-"))
	name = strings.NewReplacer("-", "", "_", "").Replace(name)
	return strings.Contains(name, "secret") || strings.Contains(name, "password") ||
		strings.Contains(name, "token") || strings.Contains(name, "credential") ||
		strings.Contains(name, "apikey") || strings.Contains(name, "accesskey") ||
		strings.Contains(name, "privatekey") || strings.Contains(name, "signature") ||
		name == "key" || name == "auth" || name == "authorization" || name == "cookie" || name == "sessionid" || name == "sig"
}

func redactCredentialURL(value string) string {
	if !strings.Contains(value, "://") {
		return value
	}
	u, err := url.Parse(value)
	if err != nil {
		return "[redacted malformed URL]"
	}
	if u.Host == "" {
		return value
	}
	if u.User != nil {
		u.User = url.User("[redacted]")
	}
	redactQuery := func(raw string) string {
		query, err := url.ParseQuery(raw)
		if err != nil {
			return "[redacted]"
		}
		changed := false
		for key := range query {
			if credentialName(key) {
				query.Set(key, "[redacted]")
				changed = true
			}
		}
		if changed {
			return query.Encode()
		}
		return raw
	}
	u.RawQuery = redactQuery(u.RawQuery)
	if strings.Contains(u.Fragment, "=") {
		u.Fragment = redactQuery(u.Fragment)
		u.RawFragment = ""
	}
	return u.String()
}

func credentialArgument(name string) bool {
	return credentialName(name) || name == "-H" || name == "--header" || name == "--headers" || name == "-u" || name == "--user"
}

func redactCredentialArgs(args []any) {
	redactNext := false
	for i, arg := range args {
		value, ok := arg.(string)
		if !ok {
			continue
		}
		if redactNext {
			args[i] = "[redacted]"
			redactNext = false
			continue
		}
		if strings.Contains(value, "=") {
			// Wrappers such as --env=API_KEY=value contain another assignment.
			// Inspect each name, stopping at the first credential value so its
			// complete remainder (including any further '=') stays hidden.
			remaining := value
			prefix := ""
			for {
				key, rest, assigned := strings.Cut(remaining, "=")
				if !assigned {
					break
				}
				prefix += key + "="
				if credentialArgument(key) {
					args[i] = prefix + "[redacted]"
					break
				}
				remaining = rest
			}
		} else if strings.HasPrefix(value, "-") && credentialArgument(value) {
			redactNext = true
		}
	}
}

func newConfigCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Inspect and edit configuration without starting an agent", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error { return c.Help() }}
	command.PersistentFlags().String("scope", "workspace", "Write scope: workspace or user")
	command.AddCommand(&cobra.Command{Use: "get [key]", Aliases: []string{"list"}, Short: "Print effective configuration with credentials redacted", Args: cobra.MaximumNArgs(1), RunE: func(c *cobra.Command, args []string) error {
		cfg, _, err := inspectCommandConfig(c)
		if err != nil {
			return err
		}
		data, err := redactedJSON(cfg)
		if err != nil {
			return err
		}
		if len(args) > 0 {
			v := gjson.GetBytes(data, args[0])
			if !v.Exists() {
				return fmt.Errorf("configuration key %q not found", args[0])
			}
			data = []byte(v.Raw)
		}
		_, err = fmt.Fprintln(c.OutOrStdout(), string(data))
		return err
	}})
	command.AddCommand(&cobra.Command{Use: "set <key> <value>", Short: "Validate and atomically save a JSON value or string", Args: cobra.ExactArgs(2), RunE: func(c *cobra.Command, args []string) error {
		var value any
		if json.Unmarshal([]byte(args[1]), &value) != nil {
			value = args[1]
		}
		return setCommandConfig(c, args[0], value)
	}})
	command.AddCommand(&cobra.Command{Use: "path", Short: "List the configuration files loaded for this workspace", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		_, paths, err := inspectCommandConfig(c)
		if err != nil {
			return err
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(paths)
	}})
	return command
}

func newMCPCommand() *cobra.Command {
	command := &cobra.Command{Use: "mcp", Short: "Manage Model Context Protocol servers", RunE: func(c *cobra.Command, _ []string) error { return c.Help() }}
	command.PersistentFlags().String("scope", "workspace", "Write scope: workspace or user")
	command.AddCommand(&cobra.Command{Use: "list", Short: "List configured servers without starting them", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		cfg, _, err := inspectCommandConfig(c)
		if err != nil {
			return err
		}
		names := make([]string, 0, len(cfg.MCP))
		for name := range cfg.MCP {
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) == 0 {
			_, err = fmt.Fprintln(c.OutOrStdout(), "No MCP servers configured. Use megafloyd mcp add <name> <command-or-url>.")
			return err
		}
		for _, name := range names {
			m := cfg.MCP[name]
			state := "enabled"
			if m.Disabled {
				state = "disabled"
			}
			if _, err = fmt.Fprintf(c.OutOrStdout(), "%s\t%s\t%s\n", name, m.Type, state); err != nil {
				return err
			}
		}
		return nil
	}})
	command.AddCommand(&cobra.Command{Use: "get <name>", Short: "Show a server configuration with credentials redacted", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		cfg, _, err := inspectCommandConfig(c)
		if err != nil {
			return err
		}
		server, ok := cfg.MCP[args[0]]
		if !ok {
			return fmt.Errorf("MCP server %q not found", args[0])
		}
		data, err := redactedJSON(server)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(c.OutOrStdout(), string(data))
		return err
	}})
	add := &cobra.Command{Use: "add <name> <command-or-url> [args...]", Short: "Add a stdio, HTTP, or SSE MCP server", Args: cobra.MinimumNArgs(2), RunE: func(c *cobra.Command, args []string) error {
		if strings.ContainsAny(args[0], ".\\#*?[]") || strings.TrimSpace(args[0]) == "" {
			return fmt.Errorf("MCP name must be a simple nonempty identifier")
		}
		cfg, _, err := inspectCommandConfig(c)
		if err != nil {
			return err
		}
		if _, exists := cfg.MCP[args[0]]; exists {
			return fmt.Errorf("MCP server %q already exists; edit it with config set", args[0])
		}
		transport, _ := c.Flags().GetString("transport")
		if transport == "" {
			transport = "stdio"
			if strings.HasPrefix(args[1], "http://") || strings.HasPrefix(args[1], "https://") {
				transport = "http"
			}
		}
		server := config.MCPConfig{Type: config.MCPType(transport)}
		switch transport {
		case "stdio":
			server.Command = args[1]
			server.Args = args[2:]
		case "http", "sse":
			u, err := url.Parse(args[1])
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
				return fmt.Errorf("expected an HTTP(S) MCP URL")
			}
			if len(args) != 2 {
				return fmt.Errorf("URL transports do not accept command arguments")
			}
			server.URL = args[1]
		default:
			return fmt.Errorf("unknown MCP transport %q", transport)
		}
		return setCommandConfig(c, "mcp."+args[0], server)
	}}
	add.Flags().StringP("transport", "t", "", "Transport: stdio, http, or sse (inferred from target)")
	command.AddCommand(add)
	for _, action := range []string{"enable", "disable", "remove"} {
		action := action
		command.AddCommand(&cobra.Command{Use: action + " <name>", Short: map[string]string{"enable": "Enable a configured server", "disable": "Disable a configured server", "remove": "Disable a server while preserving its configuration for recovery"}[action], Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
			cfg, _, err := inspectCommandConfig(c)
			if err != nil {
				return err
			}
			if _, ok := cfg.MCP[args[0]]; !ok {
				return fmt.Errorf("MCP server %q not found", args[0])
			}
			return setCommandConfig(c, "mcp."+strings.ReplaceAll(args[0], ".", "\\.")+".disabled", action != "enable")
		}})
	}
	command.AddCommand(&cobra.Command{Use: "test [name]", Short: "Connect configured servers and report discovered tool counts", Args: cobra.MaximumNArgs(1), RunE: func(c *cobra.Command, args []string) error {
		ws, cleanup, err := setupWorkspace(c)
		if err != nil {
			return err
		}
		defer cleanup()
		deadline := time.NewTimer(30 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			states := ws.MCPGetStates()
			pending := false
			names := []string{}
			for name, cfg := range ws.Config().MCP {
				if len(args) > 0 && name != args[0] {
					continue
				}
				names = append(names, name)
				if !cfg.Disabled {
					state, ok := states[name]
					if !ok || state.State == mcp.StateStarting {
						pending = true
					}
				}
			}
			if len(args) > 0 && len(names) == 0 {
				return fmt.Errorf("MCP server %q not found", args[0])
			}
			if !pending {
				sort.Strings(names)
				var failures []string
				for _, name := range names {
					state := states[name]
					fmt.Fprintf(c.OutOrStdout(), "%s\t%s\t%d tools\n", name, state.State.String(), state.Counts.Tools)
					if state.State != mcp.StateConnected && state.State != mcp.StateDisabled {
						failures = append(failures, name+": "+state.State.String())
					}
				}
				if len(failures) > 0 {
					return fmt.Errorf("MCP connection failed: %s", strings.Join(failures, "; "))
				}
				return nil
			}
			select {
			case <-c.Context().Done():
				return c.Context().Err()
			case <-deadline.C:
				return fmt.Errorf("MCP initialization did not complete within 30 seconds")
			case <-ticker.C:
			}
		}
	}})
	return command
}

func init() {
	rootCmd.AddCommand(newConfigCommand(), newMCPCommand(), &cobra.Command{Use: "setup-token", Short: "Authorize and securely store a device-login token", Long: authLoginCmd.Long, Args: cobra.NoArgs, RunE: runAuthLogin})
}
