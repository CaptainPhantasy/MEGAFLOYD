package usage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// OTLP is the wire format Jaeger accepts. Shipping our events as traces gives
// one trace per session and one span per event, so a failure can be opened and
// read next to the call that caused it.

type otlpValue struct {
	StringValue *string `json:"stringValue,omitempty"`
	IntValue    *string `json:"intValue,omitempty"`
	BoolValue   *bool   `json:"boolValue,omitempty"`
}

type otlpAttribute struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpSpan struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	ParentSpanID      string          `json:"parentSpanId,omitempty"`
	Name              string          `json:"name"`
	Kind              int             `json:"kind"`
	StartTimeUnixNano string          `json:"startTimeUnixNano"`
	EndTimeUnixNano   string          `json:"endTimeUnixNano"`
	Attributes        []otlpAttribute `json:"attributes,omitempty"`
	Status            otlpStatus      `json:"status"`
}

type otlpStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

type otlpScopeSpans struct {
	Spans []otlpSpan `json:"spans"`
}

type otlpResourceSpans struct {
	Resource   otlpResource     `json:"resource"`
	ScopeSpans []otlpScopeSpans `json:"scopeSpans"`
}

type otlpResource struct {
	Attributes []otlpAttribute `json:"attributes"`
}

type otlpRequest struct {
	ResourceSpans []otlpResourceSpans `json:"resourceSpans"`
}

// BuildOTLP groups events into one trace per session.
func BuildOTLP(events []Event) otlpRequest {
	bySession := map[string][]Event{}
	sources := map[string]string{}
	for _, event := range events {
		bySession[event.Session] = append(bySession[event.Session], event)
		sources[event.Session] = event.Source
	}
	sessions := make([]string, 0, len(bySession))
	for session := range bySession {
		sessions = append(sessions, session)
	}
	sort.Strings(sessions)

	request := otlpRequest{}
	for _, session := range sessions {
		group := bySession[session]
		sort.Slice(group, func(i, j int) bool { return group[i].TS < group[j].TS })
		traceID := traceIDFor(session)
		rootID := spanIDFor(session + "|root")

		start, end := timeBounds(group)
		root := otlpSpan{
			TraceID:           traceID,
			SpanID:            rootID,
			Name:              "session " + session,
			Kind:              1,
			StartTimeUnixNano: strconv.FormatInt(start.UnixNano(), 10),
			EndTimeUnixNano:   strconv.FormatInt(end.UnixNano(), 10),
			Attributes: []otlpAttribute{
				stringAttr("session.id", session),
				stringAttr("session.source", sources[session]),
			},
			Status: otlpStatus{Code: 0},
		}

		callSpans := map[string]string{}
		spans := []otlpSpan{root}
		for index, event := range group {
			spanID := spanIDFor(event.ID + "|" + strconv.Itoa(index))
			if event.Kind == KindToolCall && event.CallID != "" {
				callSpans[event.CallID] = spanID
			}
			parent := rootID
			if event.Kind == KindToolResult && event.CallID != "" {
				if callSpan, ok := callSpans[event.CallID]; ok {
					parent = callSpan
				}
			}
			spans = append(spans, spanFromEvent(event, traceID, spanID, parent))
		}

		source := sources[session]
		if source == "" {
			source = "unknown"
		}
		request.ResourceSpans = append(request.ResourceSpans, otlpResourceSpans{
			Resource: otlpResource{Attributes: []otlpAttribute{
				stringAttr("service.name", "megafloyd-"+source),
			}},
			ScopeSpans: []otlpScopeSpans{{Spans: spans}},
		})
	}
	return request
}

func spanFromEvent(event Event, traceID, spanID, parent string) otlpSpan {
	at, _ := time.Parse(time.RFC3339Nano, event.TS)
	if at.IsZero() {
		at = time.Now().UTC()
	}
	end := at.Add(time.Millisecond)
	attributes := []otlpAttribute{
		stringAttr("event.kind", event.Kind),
		stringAttr("session.turn", event.Turn),
	}
	if event.Tool != "" {
		attributes = append(attributes, stringAttr("tool.name", event.Tool))
	}
	if event.CallID != "" {
		attributes = append(attributes, stringAttr("tool.call_id", event.CallID))
	}
	if event.Attempt > 0 {
		attributes = append(attributes, intAttr("tool.attempt", int64(event.Attempt)))
	}
	if event.ArgsDigest != "" {
		attributes = append(attributes, stringAttr("args.digest", event.ArgsDigest))
	}
	if event.ArgsSummary != "" {
		attributes = append(attributes, stringAttr("args.summary", event.ArgsSummary))
	}
	if event.OutputBytes > 0 {
		attributes = append(attributes, intAttr("output.bytes", int64(event.OutputBytes)))
	}
	if event.TextChars > 0 {
		attributes = append(attributes, intAttr("text.chars", int64(event.TextChars)))
	}
	if event.DurationMS > 0 {
		end = at.Add(time.Duration(event.DurationMS) * time.Millisecond)
	}
	status := otlpStatus{Code: 0}
	if event.OK != nil {
		attributes = append(attributes, boolAttr("tool.ok", *event.OK))
		if !*event.OK {
			status = otlpStatus{Code: 2, Message: event.ErrorClass}
		}
	}
	if event.ErrorClass != "" {
		attributes = append(attributes, stringAttr("error.class", event.ErrorClass))
	}
	if event.ExitCode != nil {
		attributes = append(attributes, intAttr("exit.code", int64(*event.ExitCode)))
	}
	for signal, value := range event.Signals {
		attributes = append(attributes, stringAttr("signal."+signal, fmt.Sprint(value)))
	}
	return otlpSpan{
		TraceID:           traceID,
		SpanID:            spanID,
		ParentSpanID:      parent,
		Name:              spanName(event),
		Kind:              1,
		StartTimeUnixNano: strconv.FormatInt(at.UnixNano(), 10),
		EndTimeUnixNano:   strconv.FormatInt(end.UnixNano(), 10),
		Attributes:        attributes,
		Status:            status,
	}
}

func spanName(event Event) string {
	switch event.Kind {
	case KindToolCall:
		return "call " + event.Tool
	case KindToolResult:
		return "result " + event.Tool
	case KindPrompt:
		return "prompt"
	case KindAssistant:
		return "reply"
	case KindTurnEnd:
		return "turn end"
	default:
		return event.Kind
	}
}

func stringAttr(key, value string) otlpAttribute {
	if value == "" {
		return otlpAttribute{Key: key, Value: otlpValue{StringValue: &value}}
	}
	return otlpAttribute{Key: key, Value: otlpValue{StringValue: &value}}
}

func intAttr(key string, value int64) otlpAttribute {
	text := strconv.FormatInt(value, 10)
	return otlpAttribute{Key: key, Value: otlpValue{IntValue: &text}}
}

func boolAttr(key string, value bool) otlpAttribute {
	return otlpAttribute{Key: key, Value: otlpValue{BoolValue: &value}}
}

func traceIDFor(session string) string {
	sum := sha256.Sum256([]byte("session|" + session))
	return hex.EncodeToString(sum[:16])
}

func spanIDFor(key string) string {
	sum := sha256.Sum256([]byte("span|" + key))
	return hex.EncodeToString(sum[:8])
}

func timeBounds(events []Event) (time.Time, time.Time) {
	start := time.Now().UTC()
	var end time.Time
	for _, event := range events {
		at, err := time.Parse(time.RFC3339Nano, event.TS)
		if err != nil {
			continue
		}
		if at.Before(start) {
			start = at
		}
		if at.After(end) {
			end = at
		}
	}
	if end.IsZero() {
		end = start
	}
	return start, end
}

// Ship sends events to a local OTLP/HTTP endpoint (Jaeger by default) and
// returns how many sessions were sent. It refuses non-local endpoints unless
// explicitly overridden, so telemetry never leaves the machine by accident.
func Ship(endpoint string, events []Event, allowRemote bool) (int, error) {
	if endpoint == "" {
		endpoint = "http://127.0.0.1:4318/v1/traces"
	}
	if !allowRemote && !isLocalEndpoint(endpoint) {
		return 0, fmt.Errorf("refusing to ship to non-local endpoint %q", endpoint)
	}
	request := BuildOTLP(events)
	body, err := json.Marshal(request)
	if err != nil {
		return 0, err
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("post traces: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return 0, fmt.Errorf("endpoint returned %s", response.Status)
	}
	return len(request.ResourceSpans), nil
}

func isLocalEndpoint(endpoint string) bool {
	for _, prefix := range []string{"http://127.0.0.1", "http://localhost", "http://[::1]"} {
		if len(endpoint) >= len(prefix) && endpoint[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
