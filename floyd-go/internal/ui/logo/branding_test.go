package logo

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"strings"
	"testing"
)

func TestCanonicalMegaFloydWordmarkAcrossProviders(t *testing.T) {
	st := styles.ThemeForProvider("")
	for _, hyper := range []bool{false, true} {
		opts := Opts{TitleColorA: st.Logo.TitleColorA, TitleColorB: st.Logo.TitleColorB, FieldColor: st.Logo.FieldColor, CharmColor: st.Logo.CharmColor, VersionColor: st.Logo.VersionColor, Width: 80, Hyper: hyper}
		for _, compact := range []bool{false, true} {
			got := ansi.Strip(Render(st.Logo.GradCanvas, "2026.10.24", compact, opts))
			if !strings.Contains(got, "MegaFloyd") || strings.Contains(got, "HYPERFLOYD") {
				t.Fatalf("wrong product wordmark: %q", got)
			}
			if !strings.Contains(got, "2026.10.24") {
				t.Fatalf("version disappeared: %q", got)
			}
		}
		if got := ansi.Strip(SmallRender(&st, 40, opts)); !strings.Contains(got, "MegaFloyd") {
			t.Fatalf("wrong compact wordmark: %q", got)
		}
	}
}
