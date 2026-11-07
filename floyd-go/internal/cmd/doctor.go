package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/legacy-ai/floyd/internal/agent/prompt"
	"github.com/legacy-ai/floyd/internal/version"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use: "doctor", Short: "Check the installation, configuration, models, and tool environment",
	Long: "Inspect this binary and effective workspace configuration without starting an agent or contacting model providers. Use mcp test to check live MCP connections.", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		cfg, paths, err := inspectCommandConfig(cmd)
		if err != nil {
			return fmt.Errorf("configuration invalid: %w", err)
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		type check struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		}
		checks := []check{{"binary", "ok", exe}, {"version", "ok", version.Version + " (build " + version.BuildID + ")"}, {"platform", "ok", runtime.GOOS + "/" + runtime.GOARCH}, {"workspace", "ok", cwd}, {"data_directory", "ok", cfg.Options.DataDirectory}, {"configuration", "ok", fmt.Sprintf("%d file(s) loaded", len(paths))}}
		if version.Version == "devel" {
			checks[1].Status = "development"
		}
		for _, name := range []string{"git", "rg", "sh"} {
			path, err := exec.LookPath(name)
			status := "ok"
			if err != nil {
				status = "missing"
				path = "Install " + name + " or add it to PATH"
			}
			checks = append(checks, check{name, status, path})
		}
		providers := cfg.Providers.Copy()
		enabled := 0
		for _, p := range providers {
			if !p.Disable {
				enabled++
			}
		}
		state := "ok"
		detail := fmt.Sprintf("%d configured provider(s); credentials and connectivity are not probed", enabled)
		if enabled == 0 {
			state = "setup_required"
			detail = "Run megafloyd to select a provider, or megafloyd config set providers.<id> '<JSON>'"
		}
		checks = append(checks, check{"providers", state, detail})
		for slot, selection := range cfg.Models {
			state := "ok"
			if !cfg.IsModelAvailable(selection.Provider, selection.Model) {
				state = "missing"
			}
			checks = append(checks, check{"model_" + string(slot), state, selection.Provider + "/" + selection.Model})
		}
		standard := prompt.CheckStandard(prompt.StandardPolicySHA256(), prompt.StandardCandidates())
		state = "embedded"
		if standard.BundlePath != "" {
			state = "ok"
		}
		if standard.BundlePresent && !standard.Match {
			state = "mismatch"
		}
		checks = append(checks, check{"instruction_standard", state, standard.LoadedSHA256})
		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(checks); err != nil {
				return err
			}
		} else {
			for _, c := range checks {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-22s %-15s %s\n", c.Name, c.Status, c.Detail); err != nil {
					return err
				}
			}
			for _, path := range paths {
				fmt.Fprintf(cmd.OutOrStdout(), "Loaded configuration: %s\n", path)
			}
		}
		if standard.BundlePresent && !standard.Match {
			return fmt.Errorf("instruction standard differs from the embedded build; rebuild or restore the matching package")
		}
		return nil
	},
}

func init() { doctorCmd.Flags().Bool("json", false, "Emit structured diagnostic results") }
func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

func getMaxParallelism() string {
	if v := os.Getenv("SUPERFLOYD_MAX_PARALLEL"); v != "" {
		return v
	}
	return "default (1)"
}
