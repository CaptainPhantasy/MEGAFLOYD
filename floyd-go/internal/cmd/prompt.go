package cmd

import (
	"errors"
	"sort"
	"strings"

	"github.com/legacy-ai/floyd/internal/ai"
	"github.com/spf13/cobra"
)

var promptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Prompt templates for AI tasks",
}

var promptListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available prompt templates",
	Run: func(cmd *cobra.Command, args []string) {
		keys := make([]string, 0, len(ai.PromptTemplates))
		for key := range ai.PromptTemplates {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			cmd.Println(key)
		}
	},
}

var promptRenderCmd = &cobra.Command{
	Use:   "render <template>",
	Short: "Render a prompt template",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		vars, _ := cmd.Flags().GetStringSlice("var")
		values, err := parseVars(vars)
		if err != nil {
			return err
		}

		prompt, system, err := ai.UsePromptTemplate(name, values)
		if err != nil {
			return err
		}

		showSystem, _ := cmd.Flags().GetBool("system")
		if showSystem && system != "" {
			cmd.Println("--- system ---")
			cmd.Println(strings.TrimSpace(system))
			cmd.Println("--- user ---")
		}
		cmd.Println(strings.TrimSpace(prompt))
		return nil
	},
}

func init() {
	promptRenderCmd.Flags().StringSlice("var", nil, "Template variable (repeatable), e.g. --var key=value")
	promptRenderCmd.Flags().Bool("system", true, "Include system prompt in output")

	promptCmd.AddCommand(promptListCmd, promptRenderCmd)
	rootCmd.AddCommand(promptCmd)
}

func parseVars(items []string) (map[string]any, error) {
	values := map[string]any{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return nil, errors.New("Invalid --var format; expected key=value")
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, errors.New("Invalid --var format; key cannot be empty")
		}
		values[key] = value
	}
	return values, nil
}
