// Package http contains the HTTP transport layer for payments-service:
// routing, request/response handling, and middleware. It depends on the
// application layer's use cases but never on the repository or database
// package directly.
package http

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/metrics"
)

type correlationIDKey struct{}

const correlationIDHeader = "X-Request-ID"

// CorrelationID extracts the correlation/request ID stored in ctx by the
// logging middleware, if any.
func CorrelationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationIDKey{}).(string)
	return id
}

// newCorrelationID generates a short random hex ID using only crypto/rand.
func newCorrelationID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return fmt.Sprintf("%x", b)
}

// statusRecorder wraps http.ResponseWriter to capture the status code
// written, since the standard library doesn't expose it after the fact.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// WithLogging returns middleware that assigns/propagates a correlation
// ID and logs each request's method, path, status, duration, and
// correlation ID via slog. It never logs request/response bodies, so no
// payment payload details are captured.
func WithLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get(correlationIDHeader)
		if correlationID == "" {
			correlationID = newCorrelationID()
		}
		w.Header().Set(correlationIDHeader, correlationID)

		ctx := context.WithValue(r.Context(), correlationIDKey{}, correlationID)
		r = r.WithContext(ctx)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()

		next.ServeHTTP(rec, r)

		logger.LogAttrs(r.Context(), slog.LevelInfo, "http_request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Duration("duration", time.Since(start)),
			slog.String("correlation_id", correlationID),
		)
	})
}

// WithCORS returns middleware that allows cross-origin requests from
// allowedOrigin. The frontend (served on its own port, e.g.
// http://localhost:5173) calls this API directly from the browser on a
// different port, which the browser blocks by default without these
// headers. Preflight OPTIONS requests are answered directly here, since
// the underlying mux has no OPTIONS routes registered for any endpoint.
func WithCORS(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// WithMetrics wraps mux, recording http_requests_total/duration labeled
// by route PATTERN (via mux.Handler, Go 1.22+'s ServeMux) rather than
// the raw request path, so a transaction id in the URL never becomes an
// unbounded-cardinality label value. mux is taken concretely (not as a
// plain http.Handler) specifically so this can call Handler(r) to learn
// the matched pattern before dispatching.
func WithMetrics(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		if pattern == "" {
			pattern = "unmatched"
		}

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()

		mux.ServeHTTP(rec, r)

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, pattern, strconv.Itoa(rec.status)).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, pattern).Observe(time.Since(start).Seconds())
	})
}
