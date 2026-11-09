package tools

import (
	"testing"

	"github.com/legacy-ai/floyd/internal/agent/tools/mcp"
	"github.com/stretchr/testify/require"
)

// newMCPToolWithSchema builds a Tool wrapper around an MCP tool carrying
// the given input schema, matching how GetMCPTools wires live servers.
func newMCPToolWithSchema(inputSchema map[string]any) *Tool {
	return &Tool{
		mcpName: "zapier",
		tool:    &mcp.Tool{Name: "zapier_read_action", InputSchema: inputSchema},
	}
}

// requireNoRefs asserts that no "$ref" survives anywhere in the schema
// fragment, since fantasy cannot carry "$defs" to providers.
func requireNoRefs(t *testing.T, node any) {
	t.Helper()
	switch v := node.(type) {
	case map[string]any:
		require.NotContains(t, v, "$ref")
		for _, child := range v {
			requireNoRefs(t, child)
		}
	case []any:
		for _, item := range v {
			requireNoRefs(t, item)
		}
	}
}

// TestInfoInlinesDollarDefsRefs reproduces the Zapier MCP schema shape that
// made OpenAI-compatible providers reject requests with "Pointer
// '/$defs/__schema0' does not exist".
func TestInfoInlinesDollarDefsRefs(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"inputs": map[string]any{"$ref": "#/$defs/__schema0"},
		},
		"required": []any{"inputs"},
		"$defs": map[string]any{
			"__schema0": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instructions": map[string]any{"type": "string"},
				},
				"required": []any{"instructions"},
			},
		},
	}

	info := newMCPToolWithSchema(schema).Info()
	require.Equal(t, "mcp_zapier_zapier_read_action", info.Name)
	require.Equal(t, []string{"inputs"}, info.Required)
	requireNoRefs(t, info.Parameters)

	inputs, ok := info.Parameters["inputs"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", inputs["type"])
	props, ok := inputs["properties"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, props, "instructions")
}

// TestInfoInlinesNestedRefsInsideAnyOf covers references nested inside
// container keywords and definitions referencing other definitions.
func TestInfoInlinesNestedRefsInsideAnyOf(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"properties": map[string]any{
			"filter": map[string]any{
				"anyOf": []any{
					map[string]any{"$ref": "#/$defs/__schema0"},
					map[string]any{"type": "null"},
				},
			},
		},
		"$defs": map[string]any{
			"__schema0": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"options": map[string]any{"$ref": "#/$defs/__schema1"},
				},
			},
			"__schema1": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	}

	info := newMCPToolWithSchema(schema).Info()
	requireNoRefs(t, info.Parameters)

	filter, ok := info.Parameters["filter"].(map[string]any)
	require.True(t, ok)
	anyOf, ok := filter["anyOf"].([]any)
	require.True(t, ok)
	require.Len(t, anyOf, 2)

	first, ok := anyOf[0].(map[string]any)
	require.True(t, ok)
	props, ok := first["properties"].(map[string]any)
	require.True(t, ok)
	options, ok := props["options"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "array", options["type"])
}

// TestInfoDropsUnresolvableRefs ensures missing and external pointers are
// replaced by a typed permissive schema instead of reaching providers as
// dangling JSON Pointers or type-less empty objects.
func TestInfoDropsUnresolvableRefs(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"properties": map[string]any{
			"missing":  map[string]any{"$ref": "#/$defs/nope"},
			"external": map[string]any{"$ref": "https://mcp.zapier.com/schema/x"},
			"plain":    map[string]any{"type": "string"},
		},
	}

	info := newMCPToolWithSchema(schema).Info()
	requireNoRefs(t, info.Parameters)

	for _, name := range []string{"missing", "external"} {
		degraded, ok := info.Parameters[name].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "object", degraded["type"])
		require.Equal(t, true, degraded["additionalProperties"])
	}

	plain, ok := info.Parameters["plain"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "string", plain["type"])
}

// TestInfoBreaksRefCycles ensures self- and mutually-recursive definitions
// terminate: the first expansion proceeds, the cyclic one is cut.
func TestInfoBreaksRefCycles(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"properties": map[string]any{
			"a": map[string]any{"$ref": "#/$defs/a"},
		},
		"$defs": map[string]any{
			"a": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"b": map[string]any{"$ref": "#/$defs/a"},
				},
			},
		},
	}

	info := newMCPToolWithSchema(schema).Info()
	requireNoRefs(t, info.Parameters)

	outer, ok := info.Parameters["a"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", outer["type"])
	props, ok := outer["properties"].(map[string]any)
	require.True(t, ok)
	// The cyclic reference is cut: b degrades to a typed permissive schema.
	inner, ok := props["b"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", inner["type"])
	require.Equal(t, true, inner["additionalProperties"])
}

// TestInfoPreservesSiblingsAlongsideRef covers JSON Schema 2020-12 keywords
// that apply next to "$ref": inlining must not discard them.
func TestInfoPreservesSiblingsAlongsideRef(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"properties": map[string]any{
			"query": map[string]any{
				"$ref":        "#/$defs/__schema0",
				"description": "Search query",
				"default":     5,
			},
		},
		"$defs": map[string]any{
			"__schema0": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string"},
				},
			},
		},
	}

	info := newMCPToolWithSchema(schema).Info()
	requireNoRefs(t, info.Parameters)

	query, ok := info.Parameters["query"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "Search query", query["description"])
	// The clone round-trips through JSON, so numbers arrive as float64;
	// the serialized schema bytes are identical either way.
	require.Equal(t, float64(5), query["default"])
	require.Equal(t, "object", query["type"])
}

// TestInfoDoesNotMutateRegistrySchema guards the process-global MCP tool
// registry: Info() must leave the shared input schema untouched, since
// coordinators and server handlers read it concurrently.
func TestInfoDoesNotMutateRegistrySchema(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"properties": map[string]any{
			"inputs": map[string]any{"$ref": "#/$defs/__schema0"},
		},
		"$defs": map[string]any{
			"__schema0": map[string]any{"type": "object"},
		},
	}

	newMCPToolWithSchema(schema).Info()

	registryProps, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	registryInputs, ok := registryProps["inputs"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "#/$defs/__schema0", registryInputs["$ref"])
	require.NotContains(t, registryInputs, "type")
}

// TestInfoClonesSharedDefsPerReference ensures two properties referencing
// the same definition expand into independent copies.
func TestInfoClonesSharedDefsPerReference(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"properties": map[string]any{
			"first":  map[string]any{"$ref": "#/$defs/__schema0"},
			"second": map[string]any{"$ref": "#/$defs/__schema0"},
		},
		"$defs": map[string]any{
			"__schema0": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string"},
				},
			},
		},
	}

	info := newMCPToolWithSchema(schema).Info()
	requireNoRefs(t, info.Parameters)

	first, ok := info.Parameters["first"].(map[string]any)
	require.True(t, ok)
	second, ok := info.Parameters["second"].(map[string]any)
	require.True(t, ok)
	first["marker"] = true
	require.NotContains(t, second, "marker")
}
