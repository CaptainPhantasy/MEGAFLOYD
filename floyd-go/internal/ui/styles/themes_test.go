package styles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestThemeNamesCoverBuiltins(t *testing.T) {
	t.Parallel()

	names := make([]string, 0, len(ThemeNames()))
	for _, info := range ThemeNames() {
		names = append(names, info.Name)
		require.NotEmpty(t, info.Description)
	}
	require.Equal(t, []string{ThemeAuto, ThemeDark, ThemeLight}, names)
}

func TestThemesAreDistinct(t *testing.T) {
	t.Parallel()

	dark, err := ThemeByName(ThemeDark)
	require.NoError(t, err)
	light, err := ThemeByName(ThemeLight)
	require.NoError(t, err)

	require.NotEqual(t, dark, light)
	require.NotEqual(t, dark.Background, light.Background)
}

func TestThemeAliasesNormalize(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		"":                  ThemeDark,
		"dark":              ThemeDark,
		"charmtone-pantera": ThemeDark,
		"light":             ThemeLight,
		"floyd-daylight":    ThemeLight,
		"auto":              ThemeAuto,
		"DARK":              ThemeDark,
	} {
		got, err := NormalizeThemeName(input)
		require.NoError(t, err, input)
		require.Equal(t, want, got, input)
	}
}

func TestUnknownThemeIsRejected(t *testing.T) {
	t.Parallel()

	_, err := NormalizeThemeName("solarized")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown theme")
}
