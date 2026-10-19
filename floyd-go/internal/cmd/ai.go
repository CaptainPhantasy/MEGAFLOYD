package cmd

import (
	"encoding/json"
	"errors"

	"github.com/legacy-ai/floyd/internal/ai"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/spf13/cobra"
)

var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "AI helpers",
}

var aiDryRunCmd = &cobra.Command{
	Use:   "dry-run",
	Short: "Render a completion request without calling providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		template, _ := cmd.Flags().GetString("template")
		user, _ := cmd.Flags().GetString("user")
		systemOverride, _ := cmd.Flags().GetString("system")
		model, _ := cmd.Flags().GetString("model")
		maxTokens, _ := cmd.Flags().GetInt("max-tokens")
		temperature, _ := cmd.Flags().GetFloat64("temperature")
		topP, _ := cmd.Flags().GetFloat64("top-p")
		topK, _ := cmd.Flags().GetInt("top-k")
		stop, _ := cmd.Flags().GetStringArray("stop")
		pretty, _ := cmd.Flags().GetBool("pretty")

		var system string
		if template != "" {
			vars, _ := cmd.Flags().GetStringSlice("var")
			values, err := parseVars(vars)
			if err != nil {
				return err
			}
			prompt, sys, err := ai.UsePromptTemplate(template, values)
			if err != nil {
				return err
			}
			if user == "" {
				user = prompt
			}
			system = sys
		}

		if systemOverride != "" {
			system = systemOverride
		}

		if user == "" {
			return errors.New("user prompt is required (use --user or --template)")
		}

		if model == "" {
			debug, _ := cmd.Flags().GetBool("debug")
			dataDir, _ := cmd.Flags().GetString("data-dir")
			cwd, err := ResolveCwd(cmd)
			if err != nil {
				return err
			}
			cfgStore, err := config.Load(cwd, dataDir, debug)
			if err != nil {
				return err
			}
			cfg := cfgStore.Config()
			if err != nil {
				return err
			}
			if selected, ok := cfg.Models[config.SelectedModelTypeLarge]; ok && selected.Model != "" {
				model = selected.Model
				if maxTokens == 0 && selected.MaxTokens > 0 && !cmd.Flags().Changed("max-tokens") {
					maxTokens = int(selected.MaxTokens)
				}
				if selected.Temperature != nil && !cmd.Flags().Changed("temperature") {
					temperature = *selected.Temperature
				}
				if selected.TopP != nil && !cmd.Flags().Changed("top-p") {
					topP = *selected.TopP
				}
				if selected.TopK != nil && !cmd.Flags().Changed("top-k") {
					topK = int(*selected.TopK)
				}
			}
		}
		if model == "" {
			return errors.New("model is required (configure a default model or pass --model)")
		}

		req := ai.CompletionRequest{
			Model:    model,
			Messages: []ai.Message{{Role: ai.RoleUser, Content: user}},
			Stream:   false,
			System:   system,
		}
		if maxTokens > 0 {
			req.MaxTokens = &maxTokens
		}
		if cmd.Flags().Changed("temperature") || temperature != 0 {
			req.Temperature = &temperature
		}
		if cmd.Flags().Changed("top-p") || topP != 0 {
			req.TopP = &topP
		}
		if cmd.Flags().Changed("top-k") || topK != 0 {
			req.TopK = &topK
		}
		if len(stop) > 0 {
			req.StopSequences = stop
		}

		var payload []byte
		var err error
		if pretty {
			payload, err = json.MarshalIndent(req, "", "  ")
		} else {
			payload, err = json.Marshal(req)
		}
		if err != nil {
			return err
		}
		cmd.Println(string(payload))
		return nil
	},
}

func init() {
	aiDryRunCmd.Flags().String("template", "", "Prompt template name")
	aiDryRunCmd.Flags().StringSlice("var", nil, "Template variable (repeatable), e.g. --var key=value")
	aiDryRunCmd.Flags().String("user", "", "User prompt override")
	aiDryRunCmd.Flags().String("system", "", "System prompt override")
	aiDryRunCmd.Flags().String("model", "", "Model to include in the request")
	aiDryRunCmd.Flags().Int("max-tokens", 0, "Max tokens to include in the request")
	aiDryRunCmd.Flags().Float64("temperature", 0, "Temperature to include in the request")
	aiDryRunCmd.Flags().Float64("top-p", 0, "Top-p to include in the request")
	aiDryRunCmd.Flags().Int("top-k", 0, "Top-k to include in the request")
	aiDryRunCmd.Flags().StringArray("stop", nil, "Stop sequence (repeatable)")
	aiDryRunCmd.Flags().Bool("pretty", true, "Pretty print JSON output")

	aiCmd.AddCommand(aiDryRunCmd)
}
