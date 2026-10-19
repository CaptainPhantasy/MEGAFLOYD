package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

type EventType string

const (
	EventCLIStart       EventType = "cli_start"
	EventCLIExit        EventType = "cli_exit"
	EventCommandRun     EventType = "command_execute"
	EventCommandSuccess EventType = "command_success"
	EventCommandError   EventType = "command_error"
	EventAIRequest      EventType = "ai_request"
	EventAIResponse     EventType = "ai_response"
	EventAIError        EventType = "ai_error"
	EventAuthSuccess    EventType = "auth_success"
	EventAuthError      EventType = "auth_error"
)

type Event struct {
	Type       EventType      `json:"type"`
	Timestamp  string         `json:"timestamp"`
	Properties map[string]any `json:"properties"`
	Client     ClientInfo     `json:"client"`
}

type ClientInfo struct {
	Version   string `json:"version"`
	ID        string `json:"id"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

type Config struct {
	Enabled        bool
	ClientID       string
	Endpoint       string
	AdditionalData map[string]any
}

type Manager struct {
	config      Config
	initialized bool
	queue       []Event
	mu          sync.Mutex
	httpClient  *http.Client
}

func NewManager() *Manager {
	return &Manager{
		config: Config{
			Enabled:  true,
			Endpoint: "https://telemetry.legacyai.com/floyd-code/events",
		},
		queue:      []Event{},
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (m *Manager) Initialize(config Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.initialized {
		return
	}
	if config.ClientID != "" {
		m.config.ClientID = config.ClientID
	}
	if config.Endpoint != "" {
		m.config.Endpoint = config.Endpoint
	}
	if config.AdditionalData != nil {
		m.config.AdditionalData = config.AdditionalData
	}
	if !config.Enabled {
		m.config.Enabled = false
	}
	if m.config.ClientID == "" {
		m.config.ClientID = generateClientID()
	}
	m.initialized = true
}

func (m *Manager) Track(eventType EventType, properties map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.initialized || !m.config.Enabled {
		return
	}

	if properties == nil {
		properties = map[string]any{}
	}
	for k, v := range m.config.AdditionalData {
		properties[k] = v
	}

	version := "unknown"
	if value, ok := m.config.AdditionalData["cli_version"].(string); ok && value != "" {
		version = value
	}

	event := Event{
		Type:       eventType,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Properties: properties,
		Client: ClientInfo{
			Version:   version,
			ID:        m.config.ClientID,
			GoVersion: runtime.Version(),
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
		},
	}

	m.queue = append(m.queue, event)
}

func (m *Manager) Flush(ctx context.Context) error {
	m.mu.Lock()
	if !m.config.Enabled || len(m.queue) == 0 {
		m.mu.Unlock()
		return nil
	}
	payload, err := json.Marshal(m.queue)
	m.queue = []Event{}
	m.mu.Unlock()
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.config.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("telemetry request failed: %s", resp.Status)
	}
	return nil
}

func (m *Manager) FlushSync() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Flush(ctx); err != nil {
		slog.Debug("Telemetry flush failed", "error", err)
	}
}

func generateClientID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "anonymous"
	}
	return hex.EncodeToString(buf)
}

func DefaultAdditionalData() map[string]any {
	version := os.Getenv("FLOYD_VERSION")
	if version == "" {
		version = "unknown"
	}
	return map[string]any{
		"cli_version": version,
	}
}
