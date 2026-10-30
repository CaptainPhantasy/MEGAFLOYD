package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
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
	megafloydCoderPromptTmpl = mustDecodeTemplate("megafloyd-coder.md.tpl")
	taskPromptTmpl           = mustDecodeTemplate("task.md.tpl")
	initializePromptTmpl     = mustDecodeTemplate("initialize.md.tpl")
	protocolKernelTmpl       = mustDecodeTemplate("protocol_kernel.md.tpl")
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

// coderPrompt renders the one policy-bearing role template. Every executable
// alias shares it: there is no profile or binary-name dispatch left, so no
// alias can select a different behavioral policy. Project policy is discovered
// by the shared prompt loader; external prompt files are never read.
func coderPrompt(opts ...prompt.Option) (*prompt.Prompt, error) {
	systemPrompt, err := prompt.NewPrompt("coder", string(megafloydCoderPromptTmpl), opts...)
	if err != nil {
		return nil, err
	}
	return systemPrompt, nil
}

func taskPrompt(opts ...prompt.Option) (*prompt.Prompt, error) {
	systemPrompt, err := prompt.NewPrompt("task", string(taskPromptTmpl), opts...)
	if err != nil {
		return nil, err
	}
	return systemPrompt, nil
}

// InitializePrompt builds the project-initialization prompt.
func InitializePrompt(cfg *config.ConfigStore) (string, error) {
	systemPrompt, err := prompt.NewPrompt("initialize", string(initializePromptTmpl))
	if err != nil {
		return "", err
	}
	return systemPrompt.Build(context.Background(), "", "", cfg)
}

// FloydProtocol returns the complete bundled MegaFloyd standard.
func FloydProtocol() string {
	return prompt.StandardPolicy()
}

// ProtocolKernel returns the protocol kernel with the owner-mandated output
// style directive applied.
func ProtocolKernel() string {
	return protocolKernelPrompt()
}

// WebRolePrompt supplies the same approved standard to the Node runtime.
func WebRolePrompt() string {
	return strings.TrimSpace(protocolKernelPrompt()) + "\n\n" + prompt.StandardPolicy()
}
