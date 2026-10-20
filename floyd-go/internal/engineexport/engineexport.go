// Package engineexport writes a CLI session into the Floyd Code engine's
// on-disk format so the desktop/web UI can list and open it. It only touches
// the engine home given by the caller; it never reads engine credentials.
package engineexport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/legacy-ai/floyd/internal/message"
)

// Message is one message to export.
type Message struct {
	Role      string
	Parts     []message.ContentPart
	CreatedAt time.Time
}

// Result describes an export.
type Result struct {
	EngineID string
	Dir      string
	Messages int
}

type wireContent struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Think string `json:"think,omitempty"`
	URL   string `json:"url,omitempty"`
}

type wireMessage struct {
	Role       string        `json:"role"`
	Content    []wireContent `json:"content"`
	ToolCallID string        `json:"toolCallId,omitempty"`
}

type wireEvent struct {
	Type    string `json:"type"`
	Message *struct {
		Message wireMessage `json:"message"`
		Meta    struct {
			Source    string `json:"source"`
			CreatedAt string `json:"createdAt"`
		} `json:"meta"`
	} `json:"message,omitempty"`
	ProtocolVersion string `json:"protocol_version,omitempty"`
	CreatedAt       int64  `json:"created_at,omitempty"`
	WorkspaceID     string `json:"workspaceId,omitempty"`
	RuntimeID       string `json:"runtimeId,omitempty"`
	AgentID         string `json:"agentId,omitempty"`
	Time            int64  `json:"time,omitempty"`
}

type stateFile struct {
	ID       string `json:"id"`
	Version  int    `json:"version"`
	Cwd      string `json:"cwd"`
	Archived bool   `json:"archived"`
	Agents   map[string]struct {
		Homedir string `json:"homedir"`
		Type    string `json:"type"`
	} `json:"agents"`
	Custom         map[string]any `json:"custom"`
	Title          string         `json:"title"`
	TitleKind      string         `json:"titleKind"`
	LastPrompt     string         `json:"lastPrompt"`
	LastTurnReason string         `json:"lastTurnReason"`
	CreatedAt      int64          `json:"createdAt"`
	UpdatedAt      int64          `json:"updatedAt"`
	IsCustomTitle  bool           `json:"isCustomTitle"`
}

var unsafe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// Export writes messages as an engine session under home. Exporting the same
// session ID twice replaces its files and never duplicates its index row.
func Export(home, sessionID, title, cwd string, messages []Message) (Result, error) {
	if home == "" {
		return Result{}, fmt.Errorf("engine home is required")
	}
	if sessionID == "" {
		return Result{}, fmt.Errorf("session id is required")
	}
	engineID := sessionID
	if !strings.HasPrefix(engineID, "session_") {
		engineID = "session_" + engineID
	}
	if title == "" {
		title = "Imported CLI session"
	}
	if cwd == "" {
		cwd, _ = os.UserHomeDir()
	}

	scope := scopeFor(cwd)
	dir := filepath.Join(home, "sessions", scope, engineID)
	agentDir := filepath.Join(dir, "agents", "main")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create engine session dir: %w", err)
	}

	now := time.Now().UnixMilli()
	created := now
	if len(messages) > 0 && !messages[0].CreatedAt.IsZero() {
		created = messages[0].CreatedAt.UnixMilli()
	}
	state := stateFile{
		ID:       engineID,
		Version:  2,
		Cwd:      cwd,
		Archived: false,
		Agents: map[string]struct {
			Homedir string `json:"homedir"`
			Type    string `json:"type"`
		}{"main": {Homedir: agentDir, Type: "main"}},
		Custom:         map[string]any{},
		Title:          title,
		TitleKind:      "replaceable",
		LastPrompt:     lastUserText(messages),
		LastTurnReason: "completed",
		CreatedAt:      created,
		UpdatedAt:      now,
		IsCustomTitle:  false,
	}
	if err := writeJSON(filepath.Join(dir, "state.json"), state); err != nil {
		return Result{}, err
	}

	wirePath := filepath.Join(agentDir, "wire.jsonl")
	file, err := os.Create(wirePath)
	if err != nil {
		return Result{}, fmt.Errorf("create wire log: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	write := func(event any) error { return encoder.Encode(event) }

	if err := write(wireEvent{Type: "metadata", ProtocolVersion: "1.5", CreatedAt: created}); err != nil {
		file.Close()
		return Result{}, err
	}
	if err := write(wireEvent{Type: "runtime.set_binding", WorkspaceID: scope, RuntimeID: "local", AgentID: "main", Time: created}); err != nil {
		file.Close()
		return Result{}, err
	}

	promptID := ""
	stepUUID := ""
	stepOpen := false
	turnIndex := 0
	ensureStep := func(turnID string, at int64) error {
		if stepOpen {
			return nil
		}
		stepOpen = true
		stepUUID = uuid.NewString()
		return write(map[string]any{
			"type":     "step.begin",
			"step":     1,
			"stepUuid": nil,
			"turnId":   turnID,
		})
	}
	closeTurn := func(at int64, turnID string) error {
		if stepOpen {
			if err := write(map[string]any{
				"type":     "step.end",
				"step":     1,
				"stepUuid": stepUUID,
				"turnId":   turnID,
			}); err != nil {
				return err
			}
			stepOpen = false
		}
		if promptID == "" {
			return nil
		}
		if err := write(map[string]any{
			"type":       "prompt.completed",
			"agentId":    "main",
			"promptId":   promptID,
			"finishedAt": time.UnixMilli(at).UTC().Format(time.RFC3339Nano),
			"reason":     "completed",
			"time":       at,
		}); err != nil {
			return err
		}
		if err := write(map[string]any{
			"type":       "turn.ended",
			"agentId":    "main",
			"turnId":     turnIndex,
			"reason":     "completed",
			"durationMs": 0,
			"time":       at,
		}); err != nil {
			return err
		}
		promptID = ""
		turnIndex++
		return nil
	}

	for index, msg := range messages {
		at := msg.CreatedAt
		if at.IsZero() {
			at = time.UnixMilli(now + int64(index))
		}
		millis := at.UnixMilli()
		turnID := strconv.Itoa(turnIndex)

		content := make([]wireContent, 0, len(msg.Parts))
		toolCallID := ""
		var toolText strings.Builder
		type pendingCall struct {
			ID   string
			Name string
			Args string
		}
		calls := make([]pendingCall, 0)
		for _, part := range msg.Parts {
			switch typed := part.(type) {
			case message.TextContent:
				if msg.Role == "tool" {
					toolText.WriteString(typed.Text)
					continue
				}
				if typed.Text != "" {
					content = append(content, wireContent{Type: "text", Text: typed.Text})
				}
			case message.ReasoningContent:
				if typed.Thinking != "" {
					content = append(content, wireContent{Type: "think", Think: typed.Thinking})
				}
			case message.ImageURLContent:
				if typed.URL != "" {
					content = append(content, wireContent{Type: "image_url", URL: typed.URL})
				}
			case message.ToolCall:
				calls = append(calls, pendingCall{ID: typed.ID, Name: typed.Name, Args: typed.Input})
			case message.ToolResult:
				toolCallID = typed.ToolCallID
				toolText.WriteString(typed.Content)
			}
		}
		if msg.Role == "tool" {
			content = append(content, wireContent{Type: "text", Text: toolText.String()})
		}
		if len(content) == 0 && len(calls) == 0 {
			continue
		}

		// Agent stream: what clients render in the transcript view.
		if len(content) > 0 {
			source := "llm"
			if msg.Role == "user" {
				source = "input"
			}
			event := wireEvent{Type: "agent.message.appended"}
			event.Message = &struct {
				Message wireMessage `json:"message"`
				Meta    struct {
					Source    string `json:"source"`
					CreatedAt string `json:"createdAt"`
				} `json:"meta"`
			}{}
			event.Message.Message = wireMessage{Role: msg.Role, Content: content, ToolCallID: toolCallID}
			event.Message.Meta.Source = source
			event.Message.Meta.CreatedAt = at.UTC().Format(time.RFC3339Nano)
			if err := write(event); err != nil {
				file.Close()
				return Result{}, err
			}
		}

		// Context stream: what the engine's history rebuild consumes.
		if msg.Role == "user" {
			if err := closeTurn(millis, turnID); err != nil {
				file.Close()
				return Result{}, err
			}
			promptID = "msg_" + uuid.NewString()
			text := ""
			for _, part := range content {
				if part.Type == "text" {
					text = part.Text
					break
				}
			}
			if err := write(map[string]any{
				"type":     "turn.prompt",
				"agentId":  "main",
				"input":    []map[string]any{{"type": "text", "text": text}},
				"origin":   map[string]any{"kind": "user"},
				"promptId": promptID,
				"turnId":   turnIndex,
				"time":     millis,
			}); err != nil {
				file.Close()
				return Result{}, err
			}
			if err := write(map[string]any{
				"type":    "context.append_message",
				"agentId": "main",
				"message": map[string]any{
					"role":      "user",
					"content":   content,
					"id":        promptID,
					"toolCalls": []any{},
					"origin":    map[string]any{"kind": "user"},
				},
				"time": millis,
			}); err != nil {
				file.Close()
				return Result{}, err
			}
			continue
		}

		for _, part := range content {
			if part.Type != "text" && part.Type != "think" {
				continue
			}
			if err := ensureStep(turnID, millis); err != nil {
				file.Close()
				return Result{}, err
			}
			if err := write(map[string]any{
				"type":    "context.append_loop_event",
				"agentId": "main",
				"event": map[string]any{
					"type":     "content.part",
					"step":     1,
					"stepUuid": stepUUID,
					"turnId":   turnID,
					"part":     part,
				},
				"time": millis,
			}); err != nil {
				file.Close()
				return Result{}, err
			}
		}
		for _, call := range calls {
			if err := ensureStep(turnID, millis); err != nil {
				file.Close()
				return Result{}, err
			}
			args := any(map[string]any{})
			if call.Args != "" {
				var parsed any
				if json.Unmarshal([]byte(call.Args), &parsed) == nil {
					args = parsed
				} else {
					args = map[string]any{"raw": call.Args}
				}
			}
			if err := write(map[string]any{
				"type":    "context.append_loop_event",
				"agentId": "main",
				"event": map[string]any{
					"type":       "tool.call",
					"uuid":       "call_" + uuid.NewString(),
					"step":       1,
					"stepUuid":   stepUUID,
					"turnId":     turnID,
					"toolCallId": call.ID,
					"name":       call.Name,
					"args":       args,
				},
				"time": millis,
			}); err != nil {
				file.Close()
				return Result{}, err
			}
		}
		if msg.Role == "tool" && toolCallID != "" {
			if err := write(map[string]any{
				"type":    "context.append_loop_event",
				"agentId": "main",
				"event": map[string]any{
					"type":       "tool.result",
					"toolCallId": toolCallID,
					"result":     map[string]any{"output": toolText.String()},
				},
				"time": millis,
			}); err != nil {
				file.Close()
				return Result{}, err
			}
		}
	}
	if err := closeTurn(now, strconv.Itoa(turnIndex)); err != nil {
		file.Close()
		return Result{}, err
	}
	if err := file.Close(); err != nil {
		return Result{}, err
	}

	if err := appendIndex(filepath.Join(home, "session_index.jsonl"), engineID, dir, cwd); err != nil {
		return Result{}, err
	}
	return Result{EngineID: engineID, Dir: dir, Messages: len(messages)}, nil
}

func scopeFor(cwd string) string {
	sum := sha256.Sum256([]byte(cwd))
	base := filepath.Base(cwd)
	base = unsafe.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "workspace"
	}
	return fmt.Sprintf("wd_%s_%s", strings.ToLower(base), hex.EncodeToString(sum[:4]))
}

func lastUserText(messages []Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role != "user" {
			continue
		}
		for _, part := range messages[index].Parts {
			if text, ok := part.(message.TextContent); ok {
				return text.Text
			}
		}
	}
	return ""
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func appendIndex(path, sessionID, dir, cwd string) error {
	seen := false
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, `"`+sessionID+`"`) {
				seen = true
				break
			}
		}
	}
	if seen {
		return nil
	}
	entry, err := json.Marshal(map[string]string{
		"sessionId":  sessionID,
		"sessionDir": dir,
		"workDir":    cwd,
	})
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open engine session index: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(entry, '\n')); err != nil {
		return err
	}
	return nil
}
