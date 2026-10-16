package agent

import (
	"context"
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/legacy-ai/floyd/internal/agent/prompt"
	"github.com/legacy-ai/floyd/internal/config"
)

//go:embed templates/coder.md.tpl
var coderPromptTmpl []byte

//go:embed templates/task.md.tpl
var taskPromptTmpl []byte

//go:embed templates/initialize.md.tpl
var initializePromptTmpl []byte

// promptTemplate returns the template for the given prompt name. A template
// found in <global config dir>/prompts/<name>.md.tpl overrides the embedded
// default, so the engine's prompts are fully external and config-controlled.
func promptTemplate(name string, embedded []byte) string {
	overridePath := filepath.Join(filepath.Dir(config.GlobalConfig()), "prompts", name+".md.tpl")
	if b, err := os.ReadFile(overridePath); err == nil {
		slog.Info("Using external prompt template", "name", name, "path", overridePath)
		return string(b)
	}
	return string(embedded)
}

func coderPrompt(opts ...prompt.Option) (*prompt.Prompt, error) {
	systemPrompt, err := prompt.NewPrompt("coder", promptTemplate("coder", coderPromptTmpl), opts...)
	if err != nil {
		return nil, err
	}
	return systemPrompt, nil
}

func taskPrompt(opts ...prompt.Option) (*prompt.Prompt, error) {
	systemPrompt, err := prompt.NewPrompt("task", promptTemplate("task", taskPromptTmpl), opts...)
	if err != nil {
		return nil, err
	}
	return systemPrompt, nil
}

func InitializePrompt(cfg *config.ConfigStore) (string, error) {
	systemPrompt, err := prompt.NewPrompt("initialize", promptTemplate("initialize", initializePromptTmpl))
	if err != nil {
		return "", err
	}
	return systemPrompt.Build(context.Background(), "", "", cfg)
}
