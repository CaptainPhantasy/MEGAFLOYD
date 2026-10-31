package swagger

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/swaggo/swag"
)

func TestPublishedAPISchemaHasUsableClientContracts(t *testing.T) {
	t.Parallel()
	raw, err := swag.ReadDoc()
	require.NoError(t, err)
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &schema))
	require.Equal(t, "2.0", schema["swagger"])
	paths := schema["paths"].(map[string]any)
	for path, method := range map[string]string{"/health": "get", "/version": "get", "/workspaces": "post", "/workspaces/{id}/sessions": "post", "/workspaces/{id}/questions/answer": "post", "/workspaces/{id}/permissions/grant": "post"} {
		op, ok := paths[path].(map[string]any)[method].(map[string]any)
		require.True(t, ok, path+" "+method)
		require.Contains(t, op, "responses")
	}
	definitions := schema["definitions"].(map[string]any)
	var checkRefs func(any)
	checkRefs = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				const prefix = "#/definitions/"
				require.GreaterOrEqual(t, len(ref), len(prefix))
				require.Equal(t, prefix, ref[:len(prefix)])
				require.Contains(t, definitions, ref[len(prefix):])
			}
			for _, child := range v {
				checkRefs(child)
			}
		case []any:
			for _, child := range v {
				checkRefs(child)
			}
		}
	}
	checkRefs(schema)
	artifact, err := os.ReadFile("swagger.json")
	require.NoError(t, err)
	var disk map[string]any
	require.NoError(t, json.Unmarshal(artifact, &disk))
	require.Equal(t, disk["paths"], schema["paths"])
	require.Equal(t, disk["definitions"], schema["definitions"])
}
