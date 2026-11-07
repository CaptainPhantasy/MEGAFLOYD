package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLocalStorePersistsQueriesAndTimestampStatistics(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "stats.db")
	s, err := NewLocalStore(path)
	require.NoError(t, err)
	require.NoError(t, s.Record(EventCLIStart, map[string]any{"project": "fixture"}))
	require.NoError(t, s.Record(EventCommandSuccess, map[string]any{"count": 2}))
	require.NoError(t, s.Record(EventCLIStart, map[string]any{"project": "second"}))
	require.NoError(t, s.Close())
	s, err = NewLocalStore(path)
	require.NoError(t, err)
	defer s.Close()
	events, err := s.Query(context.Background(), EventCLIStart, 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.JSONEq(t, `{"project":"second"}`, string(events[0].Properties))
	stats, err := s.GetStats(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.TotalEvents)
	require.Equal(t, int64(2), stats.EventsByType[EventCLIStart])
	require.False(t, stats.FirstEvent.IsZero())
	require.False(t, stats.LastEvent.IsZero())
	require.False(t, stats.LastEvent.Before(stats.FirstEvent))
	require.Error(t, s.Record(EventAIError, map[string]any{"bad": make(chan int)}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Query(ctx, "", 0)
	require.Error(t, err)
}

func TestTelemetryOptOutAndHTTPDelivery(t *testing.T) {
	t.Parallel()
	received := make(chan []Event, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []Event
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(400)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			w.WriteHeader(400)
			return
		}
		received <- events
		w.WriteHeader(204)
	}))
	defer server.Close()
	disabled := NewManager()
	disabled.Initialize(Config{Enabled: false, Endpoint: server.URL})
	disabled.Track(EventCLIStart, nil)
	require.NoError(t, disabled.Flush(context.Background()))
	select {
	case <-received:
		t.Fatal("opt-out emitted telemetry")
	default:
	}
	m := NewManager()
	m.Initialize(Config{Enabled: true, ClientID: "test-client", Endpoint: server.URL, AdditionalData: map[string]any{"cli_version": "test"}})
	props := map[string]any{"command": "version"}
	m.Track(EventCommandRun, props)
	require.Equal(t, map[string]any{"command": "version"}, props, "tracking must not modify caller data")
	require.NoError(t, m.Flush(context.Background()))
	events := <-received
	require.Len(t, events, 1)
	require.Equal(t, EventCommandRun, events[0].Type)
	require.Equal(t, "test-client", events[0].Client.ID)
	require.Equal(t, "test", events[0].Client.Version)
	_, err := time.Parse(time.RFC3339, events[0].Timestamp)
	require.NoError(t, err)
}

func TestTelemetryReportsTransportFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	m := NewManager()
	m.Initialize(Config{Enabled: true, Endpoint: server.URL})
	m.Track(EventCLIStart, nil)
	require.ErrorContains(t, m.Flush(context.Background()), "503")
}
