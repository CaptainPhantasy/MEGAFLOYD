// Package metrics implements a minimal, dependency-free metrics
// registry that renders counters in the Prometheus text exposition
// format (version 0.0.4). It intentionally avoids prometheus/client_golang
// so the server binary keeps its dependency surface small; the rendered
// output is scrapeable by any Prometheus-compatible collector.
package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Registry stores counters and renders them in the Prometheus text
// exposition format. All methods are safe for concurrent use.
type Registry struct {
	mu       sync.Mutex
	counters map[string]*counter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{counters: make(map[string]*counter)}
}

// counter is a named, labeled counter series.
type counter struct {
	help   string
	labels []string
	values map[string]float64
}

func labelKey(labelValues []string) string {
	return strings.Join(labelValues, "\x00")
}

// AddCounter declares a counter series with a help string and fixed
// label names, then increments it by v for the given label values.
// The number of label values must match the number of label names.
// Label values are escaped when rendered.
func (r *Registry) AddCounter(name, help string, labelNames, labelValues []string, v float64) {
	if len(labelNames) != len(labelValues) {
		panic("metrics: label name/value length mismatch")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.counters[name]
	if !ok {
		c = &counter{
			help:   help,
			labels: append([]string(nil), labelNames...),
			values: make(map[string]float64),
		}
		r.counters[name] = c
	}
	c.values[labelKey(labelValues)] += v
}

// Render returns all counters in the Prometheus text exposition
// format. Series are sorted by name, then by label values, so the
// output is deterministic for the same observations.
func (r *Registry) Render() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	names := make([]string, 0, len(r.counters))
	for name := range r.counters {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		c := r.counters[name]
		fmt.Fprintf(&b, "# HELP %s %s\n", name, escapeHelp(c.help))
		fmt.Fprintf(&b, "# TYPE %s counter\n", name)
		keys := make([]string, 0, len(c.values))
		for key := range c.values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			b.WriteString(name)
			if len(c.labels) > 0 {
				b.WriteString(renderLabels(c.labels, strings.Split(key, "\x00")))
			}
			b.WriteString(" ")
			b.WriteString(strconv.FormatFloat(c.values[key], 'g', -1, 64))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderLabels(names, values []string) string {
	parts := make([]string, 0, len(names))
	for i, name := range names {
		parts = append(parts, fmt.Sprintf(
			`%s="%s"`, name, escapeLabelValue(values[i]),
		))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// escapeHelp escapes backslashes and newlines in HELP strings as
// required by the exposition format.
func escapeHelp(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`)
	return r.Replace(s)
}

// escapeLabelValue escapes backslashes, double quotes, and newlines
// in label values as required by the exposition format.
func escapeLabelValue(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(s)
}

// Snapshot returns a copy of the current counter values keyed by
// "name{label=value,...}". Intended for tests.
func (r *Registry) Snapshot() map[string]float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]float64, len(r.counters))
	for name, c := range r.counters {
		for key, v := range c.values {
			out[name+"{"+key+"}"] = v
		}
	}
	return out
}

// Reset removes all counters. Intended for tests.
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters = make(map[string]*counter)
}
