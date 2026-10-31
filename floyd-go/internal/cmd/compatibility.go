package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/spf13/cobra"
)

const resumePicker = "__session_picker__"

func addRuntimeFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	if f.Lookup("model") == nil {
		f.StringP("model", "m", "", "Model name or provider/model for this run")
	}
	if f.Lookup("small-model") == nil {
		f.String("small-model", "", "Small model name or provider/model")
	}
	if f.Lookup("quiet") == nil {
		f.BoolP("quiet", "q", false, "Hide spinner")
	}
	if f.Lookup("verbose") == nil {
		f.Bool("verbose", false, "Show logs")
	}
	f.StringSlice("add-dir", nil, "Additional directory the agent may access (repeatable)")
	f.String("permission-mode", "", "Permission mode: manual, yolo, auto, or plan")
	f.StringSlice("allowedTools", nil, "Exact tool names allowed without ordinary permission prompts (comma-separated)")
	f.Bool("dangerously-skip-permissions", false, "Skip ordinary permission prompts; protected-operation confirmations remain required")
}

func addPromptFlags(cmd *cobra.Command) {
	addRuntimeFlags(cmd)
	f := cmd.Flags()
	f.BoolP("print", "p", false, "Run the prompt non-interactively and exit")
	f.String("output-format", "text", "Prompt output format: text, json, or stream-json")
	f.BoolP("continue-legacy", "C", false, "Continue the most recent session (legacy spelling)")
	_ = f.MarkHidden("continue-legacy")
}

func continueRequested(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("continue")
	old, _ := cmd.Flags().GetBool("continue-legacy")
	return v || old
}

func requestedSession(cmd *cobra.Command) (string, error) {
	id, _ := cmd.Flags().GetString("session")
	resume, _ := cmd.Flags().GetString(resumeFlag)
	if resume != "" {
		if id != "" {
			return "", fmt.Errorf("use either --session or --resume")
		}
		if resume != resumePicker {
			id = resume
		}
	}
	if continueRequested(cmd) && (id != "" || resume != "") {
		return "", fmt.Errorf("--continue cannot be combined with --session or --resume")
	}
	return id, nil
}

// Normalize optional resume arguments before pflag interprets an argument-free
// --resume as a picker request. Literal values can always use --resume=ID.
func normalizeResumeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}
		if (arg == "--resume" || arg == "-r") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			out = append(out, "--resume="+args[i+1])
			i++
			continue
		}
		out = append(out, arg)
	}
	return out
}

func launchOptions(cmd *cobra.Command, cfg *config.Config) (config.RuntimeOptions, bool, error) {
	var opts config.RuntimeOptions
	mode, _ := cmd.Flags().GetString("permission-mode")
	yolo, _ := cmd.Flags().GetBool("yolo")
	skip, _ := cmd.Flags().GetBool("dangerously-skip-permissions")
	planMode := mode == "plan"
	if mode != "" {
		switch mode {
		case "default", "manual", "plan":
			mode = "manual"
		case "acceptEdits", "yolo":
			mode = "yolo"
		case "bypassPermissions", "auto":
			mode = "auto"
		default:
			return opts, false, fmt.Errorf("unknown permission mode %q", mode)
		}
		opts.PermissionMode = &mode
	}
	if yolo || skip {
		if planMode {
			return opts, false, fmt.Errorf("plan mode cannot be combined with skipped permissions")
		}
		mode = "auto"
		opts.PermissionMode = &mode
	}
	if cmd.Flags().Changed("allowedTools") {
		tools, _ := cmd.Flags().GetStringSlice("allowedTools")
		opts.AllowedTools = []string{}
		aliases := map[string]string{"read": "view", "multiedit": "multi_edit", "webfetch": "fetch", "websearch": "web_search"}
		for _, tool := range tools {
			for _, name := range strings.Fields(tool) {
				name = strings.ToLower(name)
				if mapped, ok := aliases[name]; ok {
					name = mapped
				}
				opts.AllowedTools = append(opts.AllowedTools, name)
			}
		}
	}
	opts.AdditionalDirs, _ = cmd.Flags().GetStringSlice("add-dir")
	if cfg != nil {
		large, _ := cmd.Flags().GetString("model")
		small, _ := cmd.Flags().GetString("small-model")
		opts.Models = map[config.SelectedModelType]config.SelectedModel{}
		for _, entry := range []struct {
			kind config.SelectedModelType
			name string
		}{{config.SelectedModelTypeLarge, large}, {config.SelectedModelTypeSmall, small}} {
			if entry.name == "" {
				continue
			}
			selected, err := config.ResolveModel(cfg, entry.name)
			if err != nil {
				return opts, false, err
			}
			opts.Models[entry.kind] = selected
		}

	}
	return opts, planMode, nil
}

func applyLaunchOptions(cmd *cobra.Command, ws workspace.Workspace) (bool, error) {
	opts, planMode, err := launchOptions(cmd, ws.Config())
	if err != nil {
		return false, err
	}
	if opts.PermissionMode == nil && opts.AllowedTools == nil && len(opts.AdditionalDirs) == 0 && len(opts.Models) == 0 {
		return planMode, nil
	}
	runtime, ok := ws.(workspace.RuntimeOptionsWorkspace)
	if !ok {
		return false, fmt.Errorf("workspace does not support runtime launch options")
	}
	if err := runtime.ApplyRuntimeOptions(cmd.Context(), opts); err != nil {
		return false, err
	}
	if len(opts.Models) > 0 {
		if err := ws.UpdateAgentModel(cmd.Context()); err != nil {
			return false, err
		}
	}
	return planMode, nil
}

func enterLaunchPlan(ctx context.Context, ws workspace.Workspace, sessionID string) error {
	_, err := ws.Plan(ctx, sessionID, &proto.RuntimePlanRequest{Action: "enter"})
	return err
}
