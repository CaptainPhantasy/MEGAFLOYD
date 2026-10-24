// Package logo renders a Floyd wordmark in a stylized way.
package logo

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/legacy-ai/floyd/internal/ui/styles"
)

// letterform represents a letterform. It can be stretched horizontally by
// a given amount via the boolean argument.
type letterform func(bool) string

const diag = `╱`

// Opts are the options for rendering the Floyd title art.
type Opts struct {
	FieldColor   color.Color // diagonal lines
	TitleColorA  color.Color // left gradient ramp point
	TitleColorB  color.Color // right gradient ramp point
	CharmColor   color.Color // Product label text color
	VersionColor color.Color // version text color
	Width        int         // width of the rendered logo, used for truncation
	Hyper        bool        // whether it is Floyd or Hyperfloyd

	// When true, stretch a random letterform on each render. Has no effect in
	// compact mode. Mainly for testing. In production you will want to cache
	// the stretched letterform to keep the logo from jittering on resize.
	Unstable bool
}

// Render renders the Floyd logo. Set the argument to true to render the narrow
// version, intended for use in a sidebar.
//
// The compact argument determines whether it renders compact for the sidebar
// or wider for the main pane.
func Render(base lipgloss.Style, version string, compact bool, o Opts) string {
	label := "MegaFloyd"
	fg := func(c color.Color, s string) string {
		return lipgloss.NewStyle().Foreground(c).Render(s)
	}
	// The visible wordmark is independent of the model provider.
	floyd := "MegaFloyd"

	floydWidth := max(24, lipgloss.Width(floyd))
	b := new(strings.Builder)
	for r := range strings.SplitSeq(floyd, "\n") {
		fmt.Fprintln(b, styles.ApplyBoldForegroundGrad(base, r, o.TitleColorA, o.TitleColorB))
	}
	floyd = b.String()

	// Product label and version.
	metaRowGap := 1
	maxVersionWidth := floydWidth - lipgloss.Width(label) - metaRowGap
	version = ansi.Truncate(version, maxVersionWidth, "…") // truncate version if too long.

	gap := max(0, floydWidth-lipgloss.Width(label)-lipgloss.Width(version))
	metaRow := fg(o.CharmColor, label) + strings.Repeat(" ", gap) + fg(o.VersionColor, version)

	// Join the meta row and big Floyd title.
	floyd = strings.TrimSpace(metaRow + "\n" + floyd)

	// Narrow version. If this is Hyperfloyd, this is also a stacked version.
	if compact {
		field := fg(o.FieldColor, strings.Repeat(diag, floydWidth))
		return strings.Join([]string{field, field, floyd, field, ""}, "\n")
	}

	fieldHeight := lipgloss.Height(floyd)

	// Left field.
	const leftWidth = 6
	leftFieldRow := fg(o.FieldColor, strings.Repeat(diag, leftWidth))
	leftField := new(strings.Builder)
	for range fieldHeight {
		fmt.Fprintln(leftField, leftFieldRow)
	}

	// Right field.
	rightWidth := max(15, o.Width-floydWidth-leftWidth-2) // 2 for the gap.
	const stepDownAt = 0
	rightField := new(strings.Builder)
	for i := range fieldHeight {
		width := rightWidth
		if i >= stepDownAt {
			width = rightWidth - (i - stepDownAt)
		}
		fmt.Fprint(rightField, fg(o.FieldColor, strings.Repeat(diag, width)), "\n")
	}

	// Return the wide version.
	const hGap = " "
	logo := lipgloss.JoinHorizontal(lipgloss.Top, leftField.String(), hGap, floyd, hGap, rightField.String())
	if o.Width > 0 {
		// Truncate the logo to the specified width.
		lines := strings.Split(logo, "\n")
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, o.Width, "")
		}
		logo = strings.Join(lines, "\n")
	}
	return logo
}

// SmallRender renders a smaller version of the Floyd logo, suitable for
// smaller windows or sidebar usage.
func SmallRender(t *styles.Styles, width int, o Opts) string {
	name := "MegaFloyd"
	title := t.Logo.SmallCharm.Render(name)
	remainingWidth := width - lipgloss.Width(title) - 1 // 1 for the space after the name
	if remainingWidth > 0 {
		lines := strings.Repeat("╱", remainingWidth)
		title = fmt.Sprintf("%s %s", title, t.Logo.SmallDiagonals.Render(lines))
	}
	return title
}
