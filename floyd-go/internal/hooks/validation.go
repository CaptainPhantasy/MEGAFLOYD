package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
)

// ValidatePayload validates the native v1 envelope, not the individual tool's schema.
func ValidatePayload(data []byte) error {
	if err := uniqueJSON(data); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"version", "event", "event_id", "timestamp", "session_id", "cwd", "tool_name", "tool_input"} {
		if _, ok := fields[key]; !ok {
			return errors.New("missing hook payload field")
		}
	}
	var p Payload
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return err
	}
	if p.Version != 1 {
		return errors.New("unsupported hook payload version")
	}
	if canonical, err := config.NormalizeHookEvent(p.Event); err != nil || canonical != p.Event {
		return errors.New("unsupported hook event")
	}
	if p.EventID == "" {
		return errors.New("missing hook event ID")
	}
	if _, err := time.Parse(time.RFC3339Nano, p.Timestamp); err != nil {
		return errors.New("invalid hook timestamp")
	}
	if p.DurationMS < 0 {
		return errors.New("invalid hook duration")
	}
	if !json.Valid(p.ToolInput) {
		return errors.New("invalid tool input JSON")
	}
	return nil
}

// Empty success output is a valid command check. Structured required results must
// be unambiguous; legacy optional output retains its historical lenient parser.
func validateRequiredOutput(output string) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	if err := uniqueJSON([]byte(output)); err != nil {
		return err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &m); err != nil || m == nil {
		return errors.New("expected object")
	}
	if v, ok := m["version"]; ok {
		var n int
		if err := json.Unmarshal(v, &n); err != nil || n != 1 {
			return errors.New("unsupported output version")
		}
	}
	if hso, ok := m["hookSpecificOutput"]; ok {
		for _, key := range []string{"decision", "halt", "updated_input", "context", "reason"} {
			if _, exists := m[key]; exists {
				return errors.New("mixed hook output formats")
			}
		}

		var x struct {
			PermissionDecision       string          `json:"permissionDecision"`
			PermissionDecisionReason string          `json:"permissionDecisionReason"`
			UpdatedInput             json.RawMessage `json:"updatedInput"`
			AdditionalContext        string          `json:"additionalContext"`
		}
		if string(hso) == "null" {
			return errors.New("invalid hookSpecificOutput")
		}
		if err := json.Unmarshal(hso, &x); err != nil {
			return err
		}
		if err := validDecision(x.PermissionDecision); err != nil {
			return err
		}
		return validPatch(x.UpdatedInput)
	}
	var x struct {
		Decision     string          `json:"decision"`
		Halt         bool            `json:"halt"`
		Reason       string          `json:"reason"`
		Context      json.RawMessage `json:"context"`
		UpdatedInput json.RawMessage `json:"updated_input"`
	}
	if err := json.Unmarshal([]byte(output), &x); err != nil {
		return err
	}
	if err := validDecision(x.Decision); err != nil {
		return err
	}
	if len(x.Context) > 0 && string(x.Context) != "null" {
		var a string
		var b []string
		if json.Unmarshal(x.Context, &a) != nil && json.Unmarshal(x.Context, &b) != nil {
			return errors.New("invalid context")
		}
	}
	return validPatch(x.UpdatedInput)
}

func validDecision(d string) error {
	switch strings.ToLower(d) {
	case "", "none", "allow", "deny":
		return nil
	}
	return errors.New("unknown hook decision")
}

func validPatch(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	patch := rawToString(raw)
	if err := uniqueJSON([]byte(patch)); err != nil {
		return err
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal([]byte(patch), &obj) != nil || obj == nil {
		return errors.New("updated input must be an object")
	}
	return nil
}

func uniqueJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var visit func() error
	visit = func() error {
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return errors.New("duplicate or invalid JSON field")
				}
				seen[s] = true
				if err := visit(); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := visit(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := visit(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("multiple or invalid JSON values")
	}
	return nil
}
