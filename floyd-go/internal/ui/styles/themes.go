package styles

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/charmtone"
)

// Built-in theme names. "auto" follows the terminal background; "dark"
// and "light" pin the palette explicitly.
const (
	ThemeAuto  = "auto"
	ThemeDark  = "dark"
	ThemeLight = "light"
)

// ThemeInfo describes one selectable built-in theme.
type ThemeInfo struct {
	Name        string
	Description string
}

// ThemeNames returns the selectable themes in display order.
func ThemeNames() []ThemeInfo {
	return []ThemeInfo{
		{Name: ThemeAuto, Description: "Match the terminal background"},
		{Name: ThemeDark, Description: "Charmtone Pantera (dark)"},
		{Name: ThemeLight, Description: "MegaFloyd Daylight (light)"},
	}
}

// IsKnownTheme reports whether name resolves to a built-in theme.
func IsKnownTheme(name string) bool {
	_, err := ThemeByName(name)
	return err == nil
}

// NormalizeThemeName maps aliases to their canonical built-in name and
// rejects unknown names with an error listing the choices.
func NormalizeThemeName(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", ThemeDark, "charmtone-pantera", "charmtone":
		return ThemeDark, nil
	case ThemeLight, "floyd-daylight":
		return ThemeLight, nil
	case ThemeAuto:
		return ThemeAuto, nil
	default:
		return "", fmt.Errorf("unknown theme %q (available: auto, dark, light)", name)
	}
}

// ThemeByName resolves a theme name to its Styles. "auto" queries the
// terminal background once and falls back to dark when the query cannot
// be answered. Aliases keep older palette names working.
func ThemeByName(name string) (Styles, error) {
	canonical, err := NormalizeThemeName(name)
	if err != nil {
		return Styles{}, err
	}
	switch canonical {
	case ThemeLight:
		return FloydDaylight(), nil
	case ThemeAuto:
		if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
			return CharmtonePantera(), nil
		}
		return FloydDaylight(), nil
	default:
		return CharmtonePantera(), nil
	}
}

// ThemeForConfig resolves the configured theme, falling back to the
// provider-derived default when no theme is set or the stored name is
// no longer known.
func ThemeForConfig(name, providerID string) Styles {
	if strings.TrimSpace(name) != "" {
		if s, err := ThemeByName(name); err == nil {
			return s
		}
	}
	return ThemeForProvider(providerID)
}

// ThemeKeyForProvider returns a stable identifier for the theme
// associated with the given provider ID. Providers that share a theme
// yield the same key, so callers can cheaply detect when switching
// providers would not actually change the active theme and skip the
// expensive style rebuild. This is the single source of truth for the
// provider-to-theme mapping; [ThemeForProvider] builds on it.
func ThemeKeyForProvider(providerID string) string {
	switch providerID {
	case "hyper":
		return "hyper"
	default:
		return "default"
	}
}

// ThemeForProvider returns the Styles associated with the given provider
// ID. Unknown or empty provider IDs yield the default Charmtone Pantera
// theme.
func ThemeForProvider(providerID string) Styles {
	switch ThemeKeyForProvider(providerID) {
	case "hyper":
		return HyperfloydObsidiana()
	default:
		return CharmtonePantera()
	}
}

// CharmtonePantera returns the Charmtone dark theme. It's the default style
// for the UI.
func CharmtonePantera() Styles {
	s := quickStyle(quickStyleOpts{
		primary:   charmtone.Charple,
		secondary: charmtone.Dolly,
		accent:    charmtone.Bok,
		keyword:   charmtone.Blush,

		fgBase:       charmtone.Sash,
		fgMoreSubtle: charmtone.Squid,
		fgSubtle:     charmtone.Smoke,
		fgMostSubtle: charmtone.Oyster,

		onPrimary: charmtone.Butter,

		bgBase:         charmtone.Pepper,
		bgLeastVisible: charmtone.BBQ,
		bgLessVisible:  charmtone.Char,
		bgMostVisible:  charmtone.Iron,

		separator: charmtone.Char,

		destructive:       charmtone.Coral,
		error:             charmtone.Sriracha,
		warningSubtle:     charmtone.Zest,
		warning:           charmtone.Mustard,
		attention:         charmtone.Tang,
		busy:              charmtone.Citron,
		info:              charmtone.Malibu,
		infoMoreSubtle:    charmtone.Sardine,
		infoMostSubtle:    charmtone.Damson,
		success:           charmtone.Julep,
		successMoreSubtle: charmtone.Bok,
		successMostSubtle: charmtone.Guac,

		// ANSI 16-color palette for remapping raw terminal output
		// (e.g. bang-mode shell commands) onto legible Charmtone colors.
		ansiBlack:   charmtone.BBQ,
		ansiRed:     charmtone.Coral,
		ansiGreen:   charmtone.Guac,
		ansiYellow:  charmtone.Mustard,
		ansiBlue:    charmtone.Charple,
		ansiMagenta: charmtone.Dolly,
		ansiCyan:    charmtone.Malibu,
		ansiWhite:   charmtone.Smoke,

		ansiBrightBlack:   charmtone.Iron,
		ansiBrightRed:     charmtone.Tuna,
		ansiBrightGreen:   charmtone.Julep,
		ansiBrightYellow:  charmtone.Zest,
		ansiBrightBlue:    charmtone.Guppy,
		ansiBrightMagenta: charmtone.Blush,
		ansiBrightCyan:    charmtone.Sardine,
		ansiBrightWhite:   charmtone.Salt,
	})

	// Bang ! prompt overrides - use Salt/Hazy/Larple colors.
	s.Editor.PromptBangIconFocused = s.Editor.PromptBangIconFocused.
		Foreground(charmtone.Salt).
		Background(charmtone.Hazy)
	s.Editor.PromptBangDotsFocused = s.Editor.PromptBangDotsFocused.
		Foreground(charmtone.Hazy)
	s.Editor.PromptBangDotsBlurred = s.Editor.PromptBangDotsBlurred.
		Foreground(charmtone.Larple)

	// Shell bar/prompt overrides - use Charple/Iron/Hazy colors.
	s.Messages.ShellBarFocused = s.Messages.ShellBarFocused.
		BorderForeground(charmtone.Charple)
	s.Messages.ShellBarBlurred = s.Messages.ShellBarBlurred.
		BorderForeground(charmtone.Iron)
	s.Messages.ShellPrompt = s.Messages.ShellPrompt.
		Foreground(charmtone.Hazy)
	s.Messages.ShellPromptBlurred = s.Messages.ShellPromptBlurred.
		Foreground(charmtone.Hazy)

	return s
}

// HyperfloydObsidiana returns the Hyperfloyd dark theme.
func HyperfloydObsidiana() Styles {
	return CharmtonePantera()
}

// FloydDaylight returns the light theme. It maps the same semantic roles
// to Charmtone's light neutrals and darker accent shades so text keeps
// readable contrast on a white background.
func FloydDaylight() Styles {
	return quickStyle(quickStyleOpts{
		primary:   charmtone.Darple,
		secondary: charmtone.Prince,
		accent:    charmtone.Damson,
		keyword:   charmtone.Chili,

		fgBase:       charmtone.Pepper,
		fgMoreSubtle: charmtone.Char,
		fgSubtle:     charmtone.Iron,
		fgMostSubtle: charmtone.Oyster,

		onPrimary: charmtone.Salt,

		bgBase:         charmtone.Salt,
		bgLeastVisible: charmtone.Soda,
		bgLessVisible:  charmtone.Steep,
		bgMostVisible:  charmtone.Steam,

		separator: charmtone.Smoke,

		destructive:       charmtone.Pom,
		error:             charmtone.Pom,
		warningSubtle:     charmtone.Cumin,
		warning:           charmtone.Cumin,
		attention:         charmtone.Tang,
		busy:              charmtone.Pickle,
		info:              charmtone.Damson,
		infoMoreSubtle:    charmtone.Anchovy,
		infoMostSubtle:    charmtone.Damson,
		success:           charmtone.Pickle,
		successMoreSubtle: charmtone.Guac,
		successMostSubtle: charmtone.Zinc,

		ansiBlack:   charmtone.Pepper,
		ansiRed:     charmtone.Pom,
		ansiGreen:   charmtone.Pickle,
		ansiYellow:  charmtone.Cumin,
		ansiBlue:    charmtone.Darple,
		ansiMagenta: charmtone.Prince,
		ansiCyan:    charmtone.Damson,
		ansiWhite:   charmtone.Iron,

		ansiBrightBlack:   charmtone.Oyster,
		ansiBrightRed:     charmtone.Coral,
		ansiBrightGreen:   charmtone.Guac,
		ansiBrightYellow:  charmtone.Mustard,
		ansiBrightBlue:    charmtone.Charple,
		ansiBrightMagenta: charmtone.Orchid,
		ansiBrightCyan:    charmtone.Malibu,
		ansiBrightWhite:   charmtone.Squid,
	})
}
