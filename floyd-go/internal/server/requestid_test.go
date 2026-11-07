package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestIDGeneratedAndEchoed(t *testing.T) {
	t.Parallel()
	s := &Server{}
	var seen string
	handler := s.requestIDHandler(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			seen = requestID(r)
		},
	))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
	require.NotEmpty(t, seen, "handlers must see a generated request ID")
	require.Len(t, seen, 32, "generated IDs are 128-bit hex")
	require.Equal(t, seen, rec.Header().Get("X-Request-ID"))
}

func TestRequestIDPreservesIncomingHeader(t *testing.T) {
	t.Parallel()
	s := &Server{}
	var seen string
	handler := s.requestIDHandler(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			seen = requestID(r)
		},
	))
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("X-Request-ID", "corr-42")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, "corr-42", seen)
	require.Equal(t, "corr-42", rec.Header().Get("X-Request-ID"))
}

func TestMetricsEndpointRendersRegistry(t *testing.T) {
	t.Parallel()
	s := &Server{}
	handler := s.metricsHandler(http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	// The registry may be empty depending on parallel test ordering;
	// any rendered output must still be exposition-format comments or
	// samples, never binary or HTML.
	if body := rec.Body.String(); body != "" {
		require.Contains(t, body, "# ")
	}
}

func TestMetricsHandlerCountsRequests(t *testing.T) {
	t.Parallel()
	s := &Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := s.metricsHandler(mux)

	for range 3 {
		handler.ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "/v1/health", nil),
		)
	}
	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/v1/nope", nil),
	)

	snapshot := metricsRegistry.Snapshot()
	require.EqualValues(t, 3,
		snapshot["megafloyd_server_http_requests_total{GET /v1/health\x00200}"],
	)
	require.EqualValues(t, 1,
		snapshot["megafloyd_server_http_requests_total{unmatched\x00404}"],
	)
	metricsRegistry.Reset()
}

func TestMetricsResponseWriterRecordsStatus(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	lrw := &metricsResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}
	lrw.WriteHeader(http.StatusTeapot)
	require.Equal(t, http.StatusTeapot, lrw.statusCode)
	require.Equal(t, "418", lrw.statusCodeLabel())
	// A second WriteHeader must not override the first recorded code.
	lrw.WriteHeader(http.StatusOK)
	require.Equal(t, http.StatusTeapot, lrw.statusCode)
}
