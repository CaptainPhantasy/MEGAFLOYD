package agent

import (
	"context"
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/legacy-ai/floyd/internal/agent/prompt"
	"github.com/legacy-ai/floyd/internal/config"
)

//go:embed templates/floyd-general.md.tpl
var floydGeneralPromptTmpl []byte

//go:embed templates/superfloyd-coder.md.tpl
var superfloydCoderPromptTmpl []byte

//go:embed templates/task.md.tpl
var taskPromptTmpl []byte

//go:embed templates/initialize.md.tpl
var initializePromptTmpl []byte

//go:embed templates/floyd_protocol.md.tpl
var floydProtocolTmpl []byte

//go:embed templates/protocol_kernel.md.tpl
var protocolKernelTmpl []byte

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

// roleTemplateFor selects the role template for the running binary. The
// superfloyd profile carries the SuperFloyd identity; anything else falls back
// to the general Floyd agent. Selection is pure so it can be tested without
// environment mutation.
func roleTemplateFor(profile, binaryName string) []byte {
	if strings.EqualFold(strings.TrimSpace(profile), "superfloyd") {
		return superfloydCoderPromptTmpl
	}
	bin := strings.ToLower(filepath.Base(binaryName))
	if strings.Contains(bin, "superfloyd") || strings.Contains(bin, "megafloyd") {
		return superfloydCoderPromptTmpl
	}
	return floydGeneralPromptTmpl
}

func coderPrompt(opts ...prompt.Option) (*prompt.Prompt, error) {
	tmpl := roleTemplateFor(os.Getenv("FLOYD_RUNTIME_PROFILE"), os.Args[0])
	systemPrompt, err := prompt.NewPrompt("coder", promptTemplate("coder", tmpl), opts...)
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

// InitializePrompt builds the project-initialization prompt.
func InitializePrompt(cfg *config.ConfigStore) (string, error) {
	systemPrompt, err := prompt.NewPrompt("initialize", promptTemplate("initialize", initializePromptTmpl))
	if err != nil {
		return "", err
	}
	return systemPrompt.Build(context.Background(), "", "", cfg)
}

// FloydProtocol returns the standard FLOYD.md protocol content.
func FloydProtocol() string {
	return string(floydProtocolTmpl)
}
