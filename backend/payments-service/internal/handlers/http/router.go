package http

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewRouter builds the complete HTTP handler for payments-service,
// wiring routes to their handlers and wrapping everything with request
// logging and metrics middleware.
func NewRouter(txHandler *TransactionHandler, healthHandler *HealthHandler, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/transactions", txHandler.Create)
	mux.HandleFunc("GET /api/v1/transactions/{id}", txHandler.Get)
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.HandleFunc("GET /ready", healthHandler.Ready)
	mux.Handle("GET /metrics", promhttp.Handler())

	return WithLogging(logger, WithMetrics(mux))
}
