package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// runOutput owns the machine-readable envelope. Errors never share stdout with
// human diagnostics, and a result event is emitted only after the run returns.
type runOutput struct {
	out       io.Writer
	format    string
	sessionID string
	started   time.Time
	content   strings.Builder
	err       error
}

func newRunOutput(out io.Writer, format string) (*runOutput, error) {
	switch format {
	case "text", "json", "stream-json":
	default:
		return nil, fmt.Errorf("unknown output format %q; use text, json, or stream-json", format)
	}
	return &runOutput{out: out, format: format, started: time.Now()}, nil
}

func (o *runOutput) emit(v any) error {
	if o.err != nil {
		return o.err
	}
	o.err = json.NewEncoder(o.out).Encode(v)
	return o.err
}

func (o *runOutput) SetSession(id string) {
	o.sessionID = id
	if o.format == "stream-json" {
		_ = o.emit(map[string]any{"type": "system", "subtype": "init", "session_id": id})
	}
}

func (o *runOutput) Write(p []byte) (int, error) {
	if o.err != nil {
		return 0, o.err
	}
	if o.format == "text" {
		return o.out.Write(p)
	}
	o.content.Write(p)
	if o.format == "stream-json" {
		if err := o.emit(map[string]any{"type": "text_delta", "session_id": o.sessionID, "text": string(p)}); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (o *runOutput) Finish(runErr error) error {
	if o.format == "text" {
		return o.err
	}
	subtype := "success"
	detail := ""
	if runErr != nil {
		subtype = "error"
		detail = runErr.Error()
	}
	return o.emit(struct {
		Type       string `json:"type"`
		Subtype    string `json:"subtype"`
		SessionID  string `json:"session_id"`
		Result     string `json:"result"`
		IsError    bool   `json:"is_error"`
		Error      string `json:"error,omitempty"`
		DurationMS int64  `json:"duration_ms"`
	}{"result", subtype, o.sessionID, strings.TrimSuffix(o.content.String(), "\n"), runErr != nil, detail, time.Since(o.started).Milliseconds()})
}
