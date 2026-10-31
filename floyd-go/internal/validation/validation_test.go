package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInputValidationBoundaries(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"https://example.com/path?q=1", "http://localhost:8080"} {
		require.True(t, IsURL(value), value)
	}
	for _, value := range []string{"https:", "http:/missing-host", "https:///path", "javascript:alert(1)", "/relative"} {
		require.False(t, IsURL(value), value)
	}
	require.True(t, IsEmail("person@example.com"))
	require.False(t, IsEmail("not-an-address"))
	require.False(t, IsNonEmptyString(" \t\n"))
	require.True(t, IsNonEmptyString(" hello "))
	for _, value := range []string{"../secret", "safe/../../secret", "file;cmd", ""} {
		require.False(t, IsValidPath(value), value)
	}
	require.True(t, IsValidFilePath("src/main.go"))
	require.False(t, IsValidFilePath("src/"))
	require.True(t, IsValidDirectoryPath("src/"))
	require.True(t, IsValidJSON(`{"ok":[1,true,null]}`))
	require.False(t, IsValidJSON(`{"ok":`))
	require.False(t, IsValidJSON(`{} {}`))
}
