// Package usage turns what a session actually did into learning data: one
// normalized event per prompt, assistant reply, tool call and tool result,
// plus the signals that explain a failure (bad arguments, repeats, re-asks).
// It reads the CLI's message store and the desktop engine's wire logs; it
// never changes either of them.
package usage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SchemaVersion is written on every event so readers can evolve safely.
const SchemaVersion = 1

// Kinds of events. One event is one thing that happened.
const (
	KindPrompt     = "prompt"
	KindAssistant  = "assistant"
	KindToolCall   = "tool.call"
	KindToolResult = "tool.result"
	KindTurnEnd    = "turn.end"
	KindSessionEnd = "session.end"
)

// Event is one normalized record. Optional fields are omitted when empty so
// the file stays readable and cheap to filter.
type Event struct {
	V            int            `json:"v"`
	ID           string         `json:"id"`
	TS           string         `json:"ts"`
	Source       string         `json:"source"`
	Session      string         `json:"session"`
	Agent        string         `json:"agent,omitempty"`
	Turn         string         `json:"turn,omitempty"`
	Step         int            `json:"step,omitempty"`
	Kind         string         `json:"kind"`
	Tool         string         `json:"tool,omitempty"`
	CallID       string         `json:"call_id,omitempty"`
	OK           *bool          `json:"ok,omitempty"`
	ErrorClass   string         `json:"error_class,omitempty"`
	ExitCode     *int           `json:"exit_code,omitempty"`
	DurationMS   int64          `json:"duration_ms,omitempty"`
	Attempt      int            `json:"attempt,omitempty"`
	ArgsDigest   string         `json:"args_digest,omitempty"`
	ArgsSummary  string         `json:"args_summary,omitempty"`
	OutputDigest string         `json:"output_digest,omitempty"`
	OutputBytes  int            `json:"output_bytes,omitempty"`
	TextChars    int            `json:"text_chars,omitempty"`
	Signals      map[string]any `json:"signals,omitempty"`
}

var exitCodePattern = regexp.MustCompile(`(?i)exit (?:code|status)[: ]+(\d+)`)

// Digest returns a short, stable fingerprint for a piece of text.
func Digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

// Summarize collapses whitespace and truncates to at most n characters.
func SummarizeText(text string, n int) string {
	collapsed := strings.Join(strings.Fields(text), " ")
	if len(collapsed) <= n {
		return collapsed
	}
	if n <= 1 {
		return collapsed[:n]
	}
	return collapsed[:n-1] + "…"
}

// Classify returns the outcome of a tool result: whether it succeeded, an
// error class for learning, and an exit code when the text carries one.
func Classify(text string, isError bool) (ok bool, class string, code int, hasCode bool) {
	lower := strings.ToLower(text)
	if match := exitCodePattern.FindStringSubmatch(text); match != nil {
		if parsed, err := strconv.Atoi(match[1]); err == nil {
			code, hasCode = parsed, true
		}
	}
	if hasCode && code != 0 {
		return false, "nonzero_exit", code, hasCode
	}
	switch {
	case strings.Contains(lower, "command not found"),
		strings.Contains(lower, "no such file or directory"),
		strings.Contains(lower, "executable file not found"):
		return false, "not_found", code, hasCode
	case strings.Contains(lower, "permission denied"),
		strings.Contains(lower, "operation not permitted"):
		return false, "permission", code, hasCode
	case strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "timed out"),
		strings.Contains(lower, "timeout"):
		return false, "timeout", code, hasCode
	case strings.Contains(lower, "syntax error"),
		strings.Contains(lower, "compile error"),
		strings.Contains(lower, "undefined:"),
		strings.Contains(lower, "cannot find package"):
		return false, "build", code, hasCode
	case strings.Contains(lower, "test failed"),
		strings.Contains(lower, "--- fail:"),
		strings.Contains(lower, "fail\t"):
		return false, "test", code, hasCode
	case isError:
		return false, "tool_error", code, hasCode
	}
	return true, "", code, hasCode
}

// eventKey builds the stable identity used for de-duplication across runs.
func eventKey(source, session, kind, callID, turn string, ordinal int) string {
	raw := fmt.Sprintf("%s|%s|%s|%s|%s|%d", source, session, kind, callID, turn, ordinal)
	return Digest(raw)
}

func newEvent(source, session, kind string, ordinal int) Event {
	return Event{
		V:       SchemaVersion,
		ID:      eventKey(source, session, kind, "", "", ordinal),
		Source:  source,
		Session: session,
		Kind:    kind,
		TS:      time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// unixTime accepts seconds or milliseconds and returns local-free UTC.
func unixTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value < 1_000_000_000_000 {
		return time.Unix(value, 0).UTC()
	}
	return time.UnixMilli(value).UTC()
}

func stamp(at time.Time) string {
	if at.IsZero() {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return at.UTC().Format(time.RFC3339Nano)
}

// Write appends events that are not already in path and returns how many were
// written. The file is append-only and line-oriented; a reader that hits a
// torn final line skips it.
func Write(path string, events []Event) (int, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return 0, fmt.Errorf("create telemetry directory: %w", err)
	}
	seen, err := existingIDs(path)
	if err != nil {
		return 0, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return 0, fmt.Errorf("open telemetry log: %w", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	written := 0
	for _, event := range events {
		if seen[event.ID] {
			continue
		}
		if event.V == 0 {
			event.V = SchemaVersion
		}
		if event.TS == "" {
			event.TS = stamp(time.Time{})
		}
		if err := encoder.Encode(event); err != nil {
			return written, fmt.Errorf("write telemetry event: %w", err)
		}
		seen[event.ID] = true
		written++
	}
	return written, nil
}

func existingIDs(path string) (map[string]bool, error) {
	seen := map[string]bool{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return seen, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read telemetry log: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err == nil && event.ID != "" {
			seen[event.ID] = true
		}
	}
	return seen, nil
}

// Load reads every event in path, skipping unreadable lines.
func Load(path string) ([]Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0, 1024)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	return events, nil
}
