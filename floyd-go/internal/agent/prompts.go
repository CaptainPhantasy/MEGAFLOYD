package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/legacy-ai/floyd/internal/agent/prompt"
	"github.com/legacy-ai/floyd/internal/config"
)

// The authored prompt layers are compiled in as gzip+base64 blobs, never as
// plain text: internal/agent/templates/*.tpl remains the editable source, and
// scripts/obfuscate-prompts.mjs regenerates internal/agent/templates_blob.go.
// Decoding happens once, in memory, so a shipped binary contains no readable
// identity text.
var (
	floydGeneralPromptTmpl    = mustDecodeTemplate("floyd-general.md.tpl")
	superfloydCoderPromptTmpl = mustDecodeTemplate("superfloyd-coder.md.tpl")
	coderPromptTmpl           = mustDecodeTemplate("coder.md.tpl")
	taskPromptTmpl            = mustDecodeTemplate("task.md.tpl")
	initializePromptTmpl      = mustDecodeTemplate("initialize.md.tpl")
	floydProtocolTmpl         = mustDecodeTemplate("floyd_protocol.md.tpl")
	protocolKernelTmpl        = mustDecodeTemplate("protocol_kernel.md.tpl")
)

// decodeTemplate inflates one encoded template. It returns nil when the name
// is unknown or the blob is corrupt.
func decodeTemplate(name string) []byte {
	encoded, ok := encodedTemplates[name]
	if !ok {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil
	}
	return out
}

// mustDecodeTemplate fails at start-up rather than silently running without a
// prompt layer.
func mustDecodeTemplate(name string) []byte {
	b := decodeTemplate(name)
	if len(bytes.TrimSpace(b)) == 0 {
		panic("agent: missing or corrupt embedded prompt template " + name)
	}
	return b
}

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

// ProtocolKernel returns the protocol kernel with the owner-mandated output
// style directive applied.
func ProtocolKernel() string {
	return protocolKernelPrompt()
}

// WebRolePrompt returns the complete authored identity stack for consumers
// outside the Go session loop — currently the Node web agent, which otherwise
// runs with a generic system prompt and none of these layers. The order
// matches the Go session: protocol kernel, FLOYD protocol, selected role
// template.
func WebRolePrompt() string {
	return strings.Join([]string{
		strings.TrimSpace(protocolKernelPrompt()),
		strings.TrimSpace(FloydProtocol()),
		strings.TrimSpace(promptTemplate("coder", roleTemplateFor(os.Getenv("FLOYD_RUNTIME_PROFILE"), os.Args[0]))),
	}, "\n\n")
}
