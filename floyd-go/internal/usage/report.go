package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ToolStat is the per-tool outcome record used by the report.
type ToolStat struct {
	Calls    int            `json:"calls"`
	OK       int            `json:"ok"`
	Failed   int            `json:"failed"`
	Retries  int            `json:"retries"`
	Errors   map[string]int `json:"errors,omitempty"`
	AvgMS    int64          `json:"avg_ms,omitempty"`
	duration int64
}

// Summary is the aggregate a human or a training job reads first.
type Summary struct {
	Events       int                  `json:"events"`
	Sessions     int                  `json:"sessions"`
	Prompts      int                  `json:"prompts"`
	Assistant    int                  `json:"assistant"`
	ToolCalls    int                  `json:"tool_calls"`
	ToolResults  int                  `json:"tool_results"`
	OK           int                  `json:"ok"`
	Failed       int                  `json:"failed"`
	Unknown      int                  `json:"unknown"`
	ErrorClasses map[string]int       `json:"error_classes,omitempty"`
	Signals      map[string]int       `json:"signals,omitempty"`
	Tools        map[string]*ToolStat `json:"tools,omitempty"`
	First        string               `json:"first,omitempty"`
	Last         string               `json:"last,omitempty"`
}

// Summarize rolls events into the aggregate.
func Summarize(events []Event) Summary {
	summary := Summary{
		Events:       len(events),
		ErrorClasses: map[string]int{},
		Signals:      map[string]int{},
		Tools:        map[string]*ToolStat{},
	}
	sessions := map[string]bool{}
	for _, event := range events {
		sessions[event.Session] = true
		if summary.First == "" || event.TS < summary.First {
			summary.First = event.TS
		}
		if event.TS > summary.Last {
			summary.Last = event.TS
		}
		for signal := range event.Signals {
			summary.Signals[signal]++
		}
		switch event.Kind {
		case KindPrompt:
			summary.Prompts++
		case KindAssistant:
			summary.Assistant++
		case KindToolCall:
			summary.ToolCalls++
			stat := summary.tool(event.Tool)
			stat.Calls++
		case KindToolResult:
			summary.ToolResults++
			stat := summary.tool(event.Tool)
			switch {
			case event.OK == nil:
				summary.Unknown++
			case *event.OK:
				summary.OK++
				stat.OK++
			default:
				summary.Failed++
				stat.Failed++
			}
			if event.ErrorClass != "" {
				summary.ErrorClasses[event.ErrorClass]++
				if stat.Errors == nil {
					stat.Errors = map[string]int{}
				}
				stat.Errors[event.ErrorClass]++
			}
			if event.Attempt > 1 {
				stat.Retries++
			}
			if event.DurationMS > 0 {
				stat.duration += event.DurationMS
			}
		}
	}
	summary.Sessions = len(sessions)
	for _, stat := range summary.Tools {
		if stat.Calls > 0 {
			stat.AvgMS = stat.duration / int64(stat.Calls)
		}
		stat.duration = 0
	}
	return summary
}

func (s *Summary) tool(name string) *ToolStat {
	if name == "" {
		name = "unknown"
	}
	if s.Tools[name] == nil {
		s.Tools[name] = &ToolStat{}
	}
	return s.Tools[name]
}

// Text renders the summary for a terminal.
func (s Summary) Text() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Events: %d across %d sessions\n", s.Events, s.Sessions)
	fmt.Fprintf(&sb, "Prompts: %d   Assistant replies: %d\n", s.Prompts, s.Assistant)
	fmt.Fprintf(&sb, "Tool calls: %d   results: %d   ok: %d   failed: %d   unknown: %d\n",
		s.ToolCalls, s.ToolResults, s.OK, s.Failed, s.Unknown)
	if s.First != "" {
		fmt.Fprintf(&sb, "Span: %s -> %s\n", s.First, s.Last)
	}
	if len(s.Signals) > 0 {
		sb.WriteString("Signals: ")
		keys := sortedKeys(s.Signals)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", key, s.Signals[key]))
		}
		sb.WriteString(strings.Join(parts, "  "))
		sb.WriteString("\n")
	}
	if len(s.ErrorClasses) > 0 {
		sb.WriteString("Error classes: ")
		keys := sortedKeys(s.ErrorClasses)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", key, s.ErrorClasses[key]))
		}
		sb.WriteString(strings.Join(parts, "  "))
		sb.WriteString("\n")
	}
	if len(s.Tools) > 0 {
		sb.WriteString("\nTools (calls / failed / retries)\n")
		names := make([]string, 0, len(s.Tools))
		for name := range s.Tools {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			stat := s.Tools[name]
			fmt.Fprintf(&sb, "  %-24s %5d / %4d / %3d\n", name, stat.Calls, stat.Failed, stat.Retries)
		}
	}
	return sb.String()
}

// WriteSummary persists the aggregate next to the event log.
func WriteSummary(path string, summary Summary) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o640)
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
