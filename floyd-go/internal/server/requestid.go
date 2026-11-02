package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/legacy-ai/floyd/internal/metrics"
)

// requestIDKey is the context key holding the per-request ID.
type requestIDKey struct{}

// requestIDHeader is the standard request correlation header. An
// incoming value is preserved so upstream clients (the desktop shell,
// tailscale fan-out, tests) can follow one request across hops.
const requestIDHeader = "X-Request-ID"

// metricsRegistry holds process-wide server counters. The registry is
// dependency-free and rendered at /v1/metrics in the Prometheus text
// exposition format.
var metricsRegistry = metrics.NewRegistry()

// requestID returns the ID assigned to the request by
// [Server.requestIDHandler], or an empty string before the middleware
// runs.
func requestID(r *http.Request) string {
	if v, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// newRequestID returns a random 128-bit hex identifier. It falls back
// to the empty string only if the system RNG fails, in which case the
// request proceeds without a correlation ID rather than failing.
func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(buf[:])
}

// requestIDHandler assigns a request ID (honoring an incoming
// X-Request-ID header), returns it in the response header, and makes
// it available to handlers and log records through the request
// context.
func (s *Server) requestIDHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(
			context.WithValue(r.Context(), requestIDKey{}, id),
		))
	})
}

// metricsHandler counts responses per route pattern and status class,
// then serves the registry at GET /v1/metrics in the Prometheus text
// exposition format.
func (s *Server) metricsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/metrics" {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			_, _ = w.Write([]byte(metricsRegistry.Render()))
			return
		}
		lrw := &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lrw, r)
		pattern := r.Pattern
		if pattern == "" {
			pattern = "unmatched"
		}
		metricsRegistry.AddCounter(
			"megafloyd_server_http_requests_total",
			"Total HTTP requests handled by the server.",
			[]string{"route", "code"},
			[]string{pattern, lrw.statusCodeLabel()},
			1,
		)
	})
}

// metricsResponseWriter records the status code written by the inner
// handler so the metrics counter can label it.
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (w *metricsResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *metricsResponseWriter) statusCodeLabel() string {
	return strconv.Itoa(w.statusCode)
}

// Flush forwards to the underlying writer so streaming handlers (SSE
// event endpoints) keep working through the wrapper. The underlying
// chain can nest several non-flushing wrappers, so the search walks
// Unwrap until it finds a [http.Flusher].
func (w *metricsResponseWriter) Flush() {
	for writer := w.ResponseWriter; writer != nil; {
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
			return
		}
		unwrapper, ok := writer.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return
		}
		writer = unwrapper.Unwrap()
	}
}

// Unwrap lets http.ResponseController reach Flush, Hijack, and other
// extended interfaces on the underlying writer.
func (w *metricsResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
