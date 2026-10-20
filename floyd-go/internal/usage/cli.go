package usage

import (
	"fmt"
	"strings"

	"github.com/legacy-ai/floyd/internal/message"
)

// FromCLI converts one stored CLI session into events. Turns are counted by
// user prompts; a tool call that repeats the same arguments inside one turn is
// marked so repeated attempts are visible in the data.
func FromCLI(sessionID string, msgs []message.Message) []Event {
	source := "cli"
	events := make([]Event, 0, len(msgs)*2)
	ordinal := 0
	turn := 0
	var lastPromptWords map[string]bool
	previousTurnFailed := false

	attempts := map[string]int{}
	callAttempts := map[string]int{}
	callTurn := map[string]string{}
	callName := map[string]string{}

	next := func(kind, callID, turnID string) Event {
		event := newEvent(source, sessionID, kind, ordinal)
		event.ID = eventKey(source, sessionID, kind, callID, turnID, ordinal)
		ordinal++
		event.Turn = turnID
		return event
	}

	for _, msg := range msgs {
		at := stamp(unixTime(msg.CreatedAt))
		turnID := fmt.Sprintf("%d", turn)

		switch msg.Role {
		case message.User:
			text := textOf(msg.Parts)
			words := wordSet(text)
			turn++
			turnID = fmt.Sprintf("%d", turn)
			attempts = map[string]int{}
			event := next(KindPrompt, "", turnID)
			event.TS = at
			event.TextChars = len(text)
			event.Signals = map[string]any{}
			if previousTurnFailed && similarity(lastPromptWords, words) >= 0.6 {
				event.Signals["reask"] = true
			}
			if len(event.Signals) == 0 {
				event.Signals = nil
			}
			events = append(events, event)
			lastPromptWords = words
			previousTurnFailed = false

		case message.Assistant:
			text := textOf(msg.Parts)
			if text != "" {
				event := next(KindAssistant, "", turnID)
				event.TS = at
				event.TextChars = len(text)
				events = append(events, event)
			}
			for _, part := range msg.Parts {
				call, ok := part.(message.ToolCall)
				if !ok || call.Name == "" {
					continue
				}
				digest := Digest(call.Input)
				key := call.Name + "|" + digest
				attempts[key]++
				callAttempts[call.ID] = attempts[key]
				callTurn[call.ID] = turnID
				callName[call.ID] = call.Name
				event := next(KindToolCall, call.ID, turnID)
				event.TS = at
				event.Tool = call.Name
				event.CallID = call.ID
				event.Attempt = attempts[key]
				event.ArgsDigest = digest
				event.ArgsSummary = SummarizeText(call.Input, 160)
				if attempts[key] > 1 {
					event.Signals = map[string]any{"repeated_call": true}
				}
				events = append(events, event)
			}

		case message.Tool:
			for _, part := range msg.Parts {
				result, ok := part.(message.ToolResult)
				if !ok {
					continue
				}
				okResult, class, code, hasCode := Classify(result.Content, result.IsError)
				event := next(KindToolResult, result.ToolCallID, callTurn[result.ToolCallID])
				event.TS = at
				event.Tool = firstNonEmpty(result.Name, callName[result.ToolCallID])
				event.CallID = result.ToolCallID
				event.OK = &okResult
				event.ErrorClass = class
				if hasCode {
					event.ExitCode = &code
				}
				event.OutputBytes = len(result.Content)
				event.OutputDigest = Digest(result.Content)
				if attempt := callAttempts[result.ToolCallID]; attempt > 1 {
					event.Attempt = attempt
					event.Signals = map[string]any{"retry": true}
				}
				if !okResult {
					previousTurnFailed = true
					if event.Signals == nil {
						event.Signals = map[string]any{}
					}
					event.Signals["failed"] = class
				}
				events = append(events, event)
			}
		}
	}
	return events
}

func textOf(parts []message.ContentPart) string {
	var sb strings.Builder
	for _, part := range parts {
		if text, ok := part.(message.TextContent); ok {
			sb.WriteString(text.Text)
		}
	}
	return sb.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func wordSet(text string) map[string]bool {
	words := map[string]bool{}
	for _, word := range strings.Fields(strings.ToLower(text)) {
		word = strings.Trim(word, ".,:;!?()[]{}\"'`")
		if len(word) > 2 {
			words[word] = true
		}
	}
	return words
}

func similarity(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for word := range a {
		if b[word] {
			shared++
		}
	}
	union := len(a) + len(b) - shared
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}
