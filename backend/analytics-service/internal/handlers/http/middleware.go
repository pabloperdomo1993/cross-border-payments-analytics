// Package http contains the HTTP transport layer for analytics-service.
package http

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type correlationIDKey struct{}

const correlationIDHeader = "X-Request-ID"

// CorrelationID extracts the correlation/request ID stored in ctx by the
// logging middleware, if any.
func CorrelationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationIDKey{}).(string)
	return id
}

func newCorrelationID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return fmt.Sprintf("%x", b)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// WithLogging returns middleware that assigns/propagates a correlation
// ID and logs each request's method, path, status, and duration.
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
