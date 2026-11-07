package metrics

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryRendersPrometheusText(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.AddCounter(
		"test_requests_total", "Total test requests.",
		[]string{"route", "code"}, []string{"/v1/health", "200"}, 3,
	)
	out := r.Render()
	require.Contains(t, out, "# HELP test_requests_total Total test requests.\n")
	require.Contains(t, out, "# TYPE test_requests_total counter\n")
	require.Contains(t, out, `test_requests_total{route="/v1/health",code="200"} 3`+"\n")
}

func TestRegistryEscapesLabelValues(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.AddCounter(
		"test_escape_total", `Uses "quotes" and \backslash`,
		[]string{"label"}, []string{`he said "hi"\n`}, 1,
	)
	out := r.Render()
	require.Contains(t, out, `label="he said \"hi\"\\n"`)
	// HELP strings escape only backslashes and newlines, not quotes.
	require.Contains(t, out, `HELP test_escape_total Uses "quotes" and \\backslash`)
}

func TestRegistryAccumulatesAndOrdersOutput(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.AddCounter("b_total", "B series.", []string{"k"}, []string{"z"}, 1)
	r.AddCounter("a_total", "A series.", []string{"k"}, []string{"y"}, 1)
	r.AddCounter("a_total", "A series.", []string{"k"}, []string{"y"}, 2)
	out := r.Render()
	require.Contains(t, out, `a_total{k="y"} 3`)
	require.Contains(t, out, `b_total{k="z"} 1`)
	require.Less(
		t, strings.Index(out, "# HELP a_total"), strings.Index(out, "# HELP b_total"),
	)
}

func TestRegistryConcurrentUse(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 100 {
				r.AddCounter("c_total", "Concurrent.", []string{"w"}, []string{"x"}, 1)
			}
		}()
		go func() {
			defer wg.Done()
			_ = r.Render()
		}()
	}
	wg.Wait()
	require.EqualValues(t, 800, r.Snapshot()["c_total{x}"])
}

func TestRegistrySnapshotAndReset(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.AddCounter("d_total", "D.", []string{"k"}, []string{"v"}, 5)
	require.EqualValues(t, 5, r.Snapshot()["d_total{v}"])
	r.Reset()
	require.Empty(t, r.Snapshot())
}
