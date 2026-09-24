package http

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewRouter builds the complete HTTP handler for payments-service,
// wiring routes to their handlers and wrapping everything with request
// logging and metrics middleware. corsAllowedOrigin is the frontend's
// origin, allowed to call this API directly from the browser (see
// WithCORS).
func NewRouter(txHandler *TransactionHandler, healthHandler *HealthHandler, logger *slog.Logger, corsAllowedOrigin string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/transactions", txHandler.Create)
	mux.HandleFunc("GET /api/v1/transactions/{id}", txHandler.Get)
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.HandleFunc("GET /ready", healthHandler.Ready)
	mux.Handle("GET /metrics", promhttp.Handler())

	return WithCORS(corsAllowedOrigin, WithLogging(logger, WithMetrics(mux)))
}
