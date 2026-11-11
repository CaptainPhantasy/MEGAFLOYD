package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools/mcp"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/permission"
)

// whitelistDockerTools contains Docker MCP tools that don't require permission.
var whitelistDockerTools = []string{
	"mcp_docker_mcp-find",
	"mcp_docker_mcp-add",
	"mcp_docker_mcp-remove",
	"mcp_docker_mcp-config-set",
	"mcp_docker_code-mode",
}

// GetMCPTools gets all the currently available MCP tools.
func GetMCPTools(permissions permission.Service, cfg *config.ConfigStore, wd string) []*Tool {
	var result []*Tool
	seen := make(map[string]int)
	for mcpName, tools := range mcp.Tools(cfg) {
		for _, tool := range tools {
			// Sanitized provider-facing names can collide when two distinct
			// tool names differ only in characters outside the allowed set;
			// keep them unique with a deterministic numeric suffix.
			t := &Tool{
				mcpName:     mcpName,
				tool:        tool,
				permissions: permissions,
				workingDir:  wd,
				cfg:         cfg,
			}
			name := t.Name()
			if n := seen[name]; n > 0 {
				suffix := fmt.Sprintf("-%d", n+1)
				t.overrideName = name[:max(1, len(name)-len(suffix))] + suffix
				seen[name] = n + 1
			} else {
				seen[name] = 1
			}
			result = append(result, t)
		}
	}
	return result
}

// Tool is a tool from a MCP.
type Tool struct {
	mcpName         string
	tool            *mcp.Tool
	cfg             *config.ConfigStore
	permissions     permission.Service
	workingDir      string
	overrideName    string
	providerOptions fantasy.ProviderOptions
}

func (m *Tool) SetProviderOptions(opts fantasy.ProviderOptions) {
	m.providerOptions = opts
}

func (m *Tool) ProviderOptions() fantasy.ProviderOptions {
	return m.providerOptions
}

func (m *Tool) Name() string {
	if m.overrideName != "" {
		return m.overrideName
	}
	return sanitizeToolName(fmt.Sprintf("mcp_%s_%s", m.mcpName, m.tool.Name))
}

// maxProviderToolNameLength matches the OpenAI tool-name limit that
// DeepSeek-compatible providers enforce alongside the character pattern.
const maxProviderToolNameLength = 64

// sanitizeToolName rewrites a composed tool name into the character set
// providers accept (^[a-zA-Z0-9_-]+$, at most 64 characters). MCP server
// names and tool names come from external configuration and may contain
// dots, slashes or non-ASCII; forwarding them unsanitized made the provider
// reject the entire request. Only the provider-facing alias changes: tool
// execution goes through the MCP protocol using the original tool name.
func sanitizeToolName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) <= maxProviderToolNameLength {
		return out
	}
	sum := sha256.Sum256([]byte(name))
	return out[:maxProviderToolNameLength-9] + "-" + hex.EncodeToString(sum[:])[:8]
}

func (m *Tool) MCP() string {
	return m.mcpName
}

func (m *Tool) MCPToolName() string {
	return m.tool.Name
}

func (m *Tool) Info() fantasy.ToolInfo {
	parameters := make(map[string]any)
	required := make([]string, 0)

	if input, ok := m.tool.InputSchema.(map[string]any); ok {
		if props, ok := input["properties"].(map[string]any); ok {
			// Clone before inlining: the properties map belongs to the
			// process-global MCP tool registry, is shared across every
			// coordinator and server handler, and must never be mutated
			// here. fantasy reassembles the schema as {"type": "object",
			// "properties": ..., "required": ...} with nowhere to carry
			// "$defs", so resolve every "$ref" against the original schema
			// now. Otherwise providers receive a dangling JSON Pointer and
			// reject the request (Zapier MCP tools use "$defs"/"__schema0").
			if cloned, ok := cloneSchemaValue(props).(map[string]any); ok {
				inlineSchemaRefs(cloned, input, nil, 0)
				parameters = cloned
			}
		}
		if req, ok := input["required"].([]any); ok {
			// Convert []any -> []string when elements are strings
			for _, v := range req {
				if s, ok := v.(string); ok {
					required = append(required, s)
				}
			}
		}
	}

	return fantasy.ToolInfo{
		Name:        m.Name(),
		Description: m.tool.Description,
		Parameters:  parameters,
		Required:    required,
	}
}

func (m *Tool) Run(ctx context.Context, params fantasy.ToolCall) (fantasy.ToolResponse, error) {
	sessionID := GetSessionFromContext(ctx)
	if sessionID == "" {
		return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for creating a new file")
	}

	// Skip permission for whitelisted Docker MCP tools.
	if !slices.Contains(whitelistDockerTools, params.Name) {
		permissionDescription := fmt.Sprintf("execute %s with the following parameters:", m.Info().Name)
		p, err := m.permissions.Request(
			ctx,
			permission.CreatePermissionRequest{
				SessionID:   sessionID,
				ToolCallID:  params.ID,
				Path:        m.workingDir,
				ToolName:    m.Info().Name,
				Action:      "execute",
				Description: permissionDescription,
				Params:      params.Input,
			},
		)
		if err != nil {
			return fantasy.ToolResponse{}, err
		}
		if !p {
			return NewPermissionDeniedResponse(), nil
		}
	}

	result, err := mcp.RunTool(ctx, m.cfg, m.mcpName, m.tool.Name, params.Input)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	switch result.Type {
	case "image", "media":
		if !GetSupportsImagesFromContext(ctx) {
			modelName := GetModelNameFromContext(ctx)
			return fantasy.NewTextErrorResponse(fmt.Sprintf("This model (%s) does not support image data.", modelName)), nil
		}

		var response fantasy.ToolResponse
		if result.Type == "image" {
			response = fantasy.NewImageResponse(result.Data, result.MediaType)
		} else {
			response = fantasy.NewMediaResponse(result.Data, result.MediaType)
		}
		response.Content = result.Content
		return response, nil
	default:
		return fantasy.NewTextResponse(result.Content), nil
	}
}

// maxSchemaRefDepth bounds the length of a "$ref" expansion chain on one
// branch while inlining referenced schema definitions. Proper cycle
// handling is done per expansion branch via the seen list.
const maxSchemaRefDepth = 64

// jsonPointerUnescape decodes the JSON Pointer escape sequences.
var jsonPointerUnescape = strings.NewReplacer("~1", "/", "~0", "~")

// inlineSchemaRefs expands every "$ref" inside node against root, which is
// the MCP tool's full input schema. fantasy rebuilds the tool schema from
// fantasy.ToolInfo as {"type": "object", "properties": ..., "required":
// ...} and has nowhere to carry "$defs", so a "$ref" that reaches it
// unresolved is a dangling JSON Pointer and OpenAI-compatible providers
// reject the whole request ("Pointer '/$defs/__schema0' does not exist").
// Pointers that cannot be resolved locally (external URLs, missing
// definitions, cycles) are replaced by a typed permissive schema so one
// malformed property cannot break every request. node must be private to
// the caller; the registry's maps are never mutated here.
func inlineSchemaRefs(node any, root map[string]any, seen []string, depth int) {
	switch v := node.(type) {
	case map[string]any:
		inlineSchemaRefsMap(v, root, seen, depth)
	case []any:
		for _, item := range v {
			inlineSchemaRefs(item, root, seen, depth)
		}
	}
}

func inlineSchemaRefsMap(node map[string]any, root map[string]any, seen []string, depth int) {
	if depth > maxSchemaRefDepth {
		delete(node, "$ref")
		return
	}

	if ref, ok := node["$ref"].(string); ok {
		if target, ok := resolveJSONPointer(ref, root); ok {
			if def, ok := target.(map[string]any); ok && !slices.Contains(seen, ref) {
				if clone, ok := cloneSchemaValue(def).(map[string]any); ok {
					// Overlay the definition on the surviving siblings:
					// JSON Schema 2020-12 applies keywords like
					// "description" alongside "$ref", so they must not be
					// discarded, and existing sibling keys win. The
					// definition is cloned per reference, so expansions
					// never share mutable state.
					delete(node, "$ref")
					for k, v := range clone {
						if _, exists := node[k]; !exists {
							node[k] = v
						}
					}
					inlineSchemaRefsMap(node, root, append(seen, ref), depth+1)
					return
				}
			}
		}
		// Unresolvable pointer: degrade this one property to a typed
		// permissive schema instead of leaving a dangling "$ref" or a
		// type-less empty object, both of which strict providers reject.
		delete(node, "$ref")
		if len(node) == 0 {
			node["type"] = "object"
			node["additionalProperties"] = true
		}
		return
	}

	for _, v := range node {
		inlineSchemaRefs(v, root, seen, depth)
	}
}

// resolveJSONPointer resolves a local "#..." JSON Pointer against root,
// decoding the "~0" and "~1" escapes. Non-local pointers are unsupported.
func resolveJSONPointer(pointer string, root map[string]any) (any, bool) {
	if pointer == "#" {
		return root, true
	}
	if !strings.HasPrefix(pointer, "#/") {
		return nil, false
	}
	var current any = root
	for _, raw := range strings.Split(pointer[2:], "/") {
		token := jsonPointerUnescape.Replace(raw)
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = m[token]; !ok {
			return nil, false
		}
	}
	return current, true
}

// cloneSchemaValue deep-copies schema data decoded from JSON so a
// definition referenced from several places is expanded independently
// everywhere it appears.
func cloneSchemaValue(v any) any {
	bts, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(bts, &out); err != nil {
		return v
	}
	return out
}
