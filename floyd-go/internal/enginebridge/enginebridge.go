// Package enginebridge reads sessions produced by the Floyd Code web engine
// (kap-server) so the Go CLI can list and import them. It is strictly
// read-only against the canonical MEGAFLOYD engine store.
package enginebridge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/legacy-ai/floyd/internal/message"
)

// Session is one session entry from the engine's session index.
type Session struct {
	ID        string
	Title     string
	WorkDir   string
	Dir       string
	UpdatedAt time.Time
}

// Home resolves the canonical web store. Parent harness homes are never
// discovered implicitly; an explicit MEGAFLOYD_WEB_DATA override wins.
func Home() string {
	if dir := strings.TrimSpace(os.Getenv("MEGAFLOYD_WEB_DATA")); dir != "" {
		return dir
	}
	home, _ := megafloyd.HomeDir()
	return filepath.Join(home, "web")
}

type indexEntry struct {
	SessionID  string `json:"sessionId"`
	SessionDir string `json:"sessionDir"`
	WorkDir    string `json:"workDir"`
}

type stateFile struct {
	Title     string `json:"title"`
	Cwd       string `json:"cwd"`
	UpdatedAt int64  `json:"updatedAt"`
}

// List returns the engine sessions visible in home, newest first. Duplicate
// index entries keep their most recent state.
func List(home string) ([]Session, error) {
	file, err := os.Open(filepath.Join(home, "session_index.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("open engine session index: %w", err)
	}
	defer file.Close()

	byID := map[string]Session{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry indexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.SessionID == "" {
			continue
		}
		session := Session{
			ID:      entry.SessionID,
			Dir:     entry.SessionDir,
			WorkDir: entry.WorkDir,
		}
		if state, err := readState(filepath.Join(entry.SessionDir, "state.json")); err == nil {
			session.Title = state.Title
			if state.Cwd != "" {
				session.WorkDir = state.Cwd
			}
			if state.UpdatedAt > 0 {
				session.UpdatedAt = time.UnixMilli(state.UpdatedAt)
			}
		}
		if existing, ok := byID[session.ID]; ok && existing.UpdatedAt.After(session.UpdatedAt) {
			continue
		}
		byID[session.ID] = session
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read engine session index: %w", err)
	}

	sessions := make([]Session, 0, len(byID))
	for _, session := range byID {
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// Find returns one session by exact ID.
func Find(home, id string) (Session, error) {
	sessions, err := List(home)
	if err != nil {
		return Session{}, err
	}
	for _, session := range sessions {
		if session.ID == id {
			return session, nil
		}
	}
	return Session{}, fmt.Errorf("engine session %q not found in %s", id, home)
}

func readState(path string) (stateFile, error) {
	var state stateFile
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

// TranscriptMessage is one message reconstructed from the engine wire log.
type TranscriptMessage struct {
	ID        string
	Role      string
	Parts     []message.ContentPart
	CreatedAt time.Time
}

type wireEvent struct {
	Type    string `json:"type"`
	Message struct {
		Message struct {
			Role       string     `json:"role"`
			Content    []wirePart `json:"content"`
			ToolCallID string     `json:"toolCallId"`
		} `json:"message"`
		Meta struct {
			CreatedAt string `json:"createdAt"`
		} `json:"meta"`
	} `json:"message"`
}

type wirePart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Think    string `json:"think"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"imageUrl"`
}

// ReadTranscript parses agents/main/wire.jsonl into ordered messages. Only
// completed appended messages are used; loop events (tool calls) are not
// reconstructed. Malformed lines are skipped.
func ReadTranscript(sessionDir string) ([]TranscriptMessage, error) {
	path := filepath.Join(sessionDir, "agents", "main", "wire.jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open engine wire log: %w", err)
	}
	defer file.Close()

	var messages []TranscriptMessage
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 256*1024), 32*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, "agent.message.appended") {
			continue
		}
		var event wireEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil || event.Type != "agent.message.appended" {
			continue
		}
		wire := event.Message.Message
		parts := translateParts(wire.Role, wire.Content, wire.ToolCallID)
		if len(parts) == 0 {
			continue
		}
		created, _ := time.Parse(time.RFC3339Nano, event.Message.Meta.CreatedAt)
		messages = append(messages, TranscriptMessage{
			ID:        wire.ToolCallID,
			Role:      wire.Role,
			Parts:     parts,
			CreatedAt: created,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read engine wire log: %w", err)
	}
	return messages, nil
}

func translateParts(role string, parts []wirePart, toolCallID string) []message.ContentPart {
	translated := make([]message.ContentPart, 0, len(parts))
	var toolText strings.Builder
	for _, part := range parts {
		switch part.Type {
		case "text":
			if part.Text == "" {
				continue
			}
			if role == "tool" {
				toolText.WriteString(part.Text)
				continue
			}
			translated = append(translated, message.TextContent{Text: part.Text})
		case "think":
			if part.Think == "" {
				continue
			}
			translated = append(translated, message.ReasoningContent{Thinking: part.Think})
		case "image_url":
			if part.ImageURL.URL == "" {
				continue
			}
			translated = append(translated, message.ImageURLContent{URL: part.ImageURL.URL})
		}
	}
	if role == "tool" && toolCallID != "" && toolText.Len() > 0 {
		translated = append(translated, message.ToolResult{ToolCallID: toolCallID, Content: toolText.String()})
	}
	return translated
}
