package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FromEngine reads every desktop/web engine session under home and returns
// their events. Wire logs are append-only JSON lines; a malformed line is
// skipped rather than aborting the scan.
func FromEngine(home string) ([]Event, error) {
	if home == "" {
		return nil, nil
	}
	root := filepath.Join(home, "sessions")
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() && entry.Name() == "wire.jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan engine sessions: %w", err)
	}
	sort.Strings(files)
	events := make([]Event, 0, len(files)*32)
	for _, file := range files {
		collected, err := fromWireFile(file)
		if err != nil {
			return nil, err
		}
		events = append(events, collected...)
	}
	return events, nil
}

func fromWireFile(path string) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open wire log: %w", err)
	}
	defer file.Close()

	sessionID := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(path))))
	agent := filepath.Base(filepath.Dir(path))
	info, _ := file.Stat()
	fallback := time.Now().UTC()
	if info != nil {
		fallback = info.ModTime().UTC()
	}

	events := make([]Event, 0, 64)
	ordinal := 0
	turn := 0
	turnID := "0"
	attempts := map[string]int{}
	callAttempts := map[string]int{}
	callNames := map[string]string{}

	next := func(kind, callID string) Event {
		event := newEvent("desktop", sessionID, kind, ordinal)
		event.ID = eventKey("desktop", sessionID, kind, callID, turnID, ordinal)
		ordinal++
		event.Turn = turnID
		event.Agent = agent
		event.TS = fallback.Format(time.RFC3339Nano)
		return event
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var envelope map[string]any
		if err := json.Unmarshal(line, &envelope); err != nil {
			continue
		}
		at := timeFrom(envelope["time"], fallback)
		switch getString(envelope, "type") {
		case "context.append_message":
			msg, _ := envelope["message"].(map[string]any)
			if msg == nil {
				continue
			}
			text := contentText(msg["content"])
			switch getString(msg, "role") {
			case "user":
				turn++
				turnID = fmt.Sprintf("%d", turn)
				attempts = map[string]int{}
				event := next(KindPrompt, "")
				event.TS = at.Format(time.RFC3339Nano)
				event.TextChars = len(text)
				events = append(events, event)
			case "assistant":
				if text == "" {
					continue
				}
				event := next(KindAssistant, "")
				event.TS = at.Format(time.RFC3339Nano)
				event.TextChars = len(text)
				events = append(events, event)
			}

		case "context.append_loop_event":
			inner, _ := envelope["event"].(map[string]any)
			if inner == nil {
				continue
			}
			eventAt := timeFrom(inner["time"], at)
			if id := getString(inner, "turnId"); id != "" {
				turnID = id
			}
			switch getString(inner, "type") {
			case "tool.call":
				name := getString(inner, "name")
				if name == "" {
					continue
				}
				rawArgs := string(mustJSON(inner["args"]))
				digest := Digest(rawArgs)
				key := name + "|" + digest
				attempts[key]++
				callID := getString(inner, "toolCallId")
				if callID == "" {
					callID = getString(inner, "uuid")
				}
				callAttempts[callID] = attempts[key]
				callNames[callID] = name
				event := next(KindToolCall, callID)
				event.TS = eventAt.Format(time.RFC3339Nano)
				event.Tool = name
				event.CallID = callID
				event.Attempt = attempts[key]
				event.ArgsDigest = digest
				event.ArgsSummary = SummarizeText(rawArgs, 160)
				if step := getInt(inner, "step"); step > 0 {
					event.Step = step
				}
				if attempts[key] > 1 {
					event.Signals = map[string]any{"repeated_call": true}
				}
				events = append(events, event)

			case "tool.result":
				result, _ := inner["result"].(map[string]any)
				output := getString(result, "output")
				isError := getBool(result, "isError")
				okResult, class, code, hasCode := Classify(output, isError)
				callID := getString(inner, "toolCallId")
				event := next(KindToolResult, callID)
				event.TS = eventAt.Format(time.RFC3339Nano)
				event.CallID = callID
				event.Tool = callNames[callID]
				event.OK = &okResult
				event.ErrorClass = class
				if hasCode {
					event.ExitCode = &code
				}
				event.OutputBytes = len(output)
				event.OutputDigest = Digest(output)
				if attempt := callAttempts[callID]; attempt > 1 {
					event.Attempt = attempt
					event.Signals = map[string]any{"retry": true}
				}
				events = append(events, event)

			case "turn.ended":
				reason := getString(inner, "reason")
				event := next(KindTurnEnd, "")
				event.TS = eventAt.Format(time.RFC3339Nano)
				event.DurationMS = int64(getInt(inner, "durationMs"))
				event.Signals = map[string]any{"reason": reason}
				events = append(events, event)
			}

		case "turn.ended":
			reason := getString(envelope, "reason")
			event := next(KindTurnEnd, "")
			event.TS = at.Format(time.RFC3339Nano)
			event.DurationMS = int64(getInt(envelope, "durationMs"))
			event.Signals = map[string]any{"reason": reason}
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read wire log: %w", err)
	}
	return events, nil
}

func contentText(value any) string {
	parts, ok := value.([]any)
	if !ok {
		if text, ok := value.(string); ok {
			return text
		}
		return ""
	}
	var sb strings.Builder
	for _, part := range parts {
		partMap, _ := part.(map[string]any)
		if getString(partMap, "type") == "text" {
			sb.WriteString(getString(partMap, "text"))
		}
	}
	return sb.String()
}

func timeFrom(value any, fallback time.Time) time.Time {
	switch typed := value.(type) {
	case float64:
		return unixTime(int64(typed))
	case int64:
		return unixTime(typed)
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, typed); err == nil {
			return parsed.UTC()
		}
	}
	return fallback
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func getString(source map[string]any, key string) string {
	if value, ok := source[key].(string); ok {
		return value
	}
	return ""
}

func getInt(source map[string]any, key string) int {
	switch typed := source[key].(type) {
	case float64:
		return int(typed)
	case int64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	}
	return 0
}

func getBool(source map[string]any, key string) bool {
	if value, ok := source[key].(bool); ok {
		return value
	}
	return false
}
