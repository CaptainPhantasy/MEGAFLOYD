package csync_test

import (
	"reflect"
	"sync"
	"testing"

	"github.com/invopop/jsonschema"
	"github.com/legacy-ai/floyd/internal/csync"
	"github.com/stretchr/testify/require"
)

func TestMapJSONSchemaValueAndPointer(t *testing.T) {
	t.Parallel()

	for name, typ := range map[string]reflect.Type{
		"value":   reflect.TypeFor[csync.Map[string, int]](),
		"pointer": reflect.TypeFor[*csync.Map[string, int]](),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			reflector := jsonschema.Reflector{DoNotReference: true}
			schema := reflector.ReflectFromType(typ)
			require.Equal(t, "object", schema.Type)
			require.NotNil(t, schema.AdditionalProperties)
			require.Equal(t, "integer", schema.AdditionalProperties.Type)
		})
	}
}

func TestMapJSONSchemaAliasConcurrentWrites(t *testing.T) {
	t.Parallel()

	m := csync.NewMap[string, int]()
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Go(func() {
		<-start
		for i := range 1000 {
			m.Set("value", i)
		}
	})
	close(start)
	for range 1000 {
		require.IsType(t, map[string]int{}, m.JSONSchemaAlias())
	}
	workers.Wait()
	value, ok := m.Get("value")
	require.True(t, ok)
	require.Equal(t, 999, value)
}
