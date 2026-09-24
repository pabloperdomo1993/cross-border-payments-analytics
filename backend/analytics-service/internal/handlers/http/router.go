package http

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewRouter builds the complete HTTP handler for analytics-service.
// corsAllowedOrigin is the frontend's origin, allowed to call this API
// directly from the browser (see WithCORS).
func NewRouter(analyticsHandler *AnalyticsHandler, healthHandler *HealthHandler, logger *slog.Logger, corsAllowedOrigin string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/analytics/corridors", analyticsHandler.Corridors)
	mux.HandleFunc("GET /api/v1/analytics/currencies", analyticsHandler.Currencies)
	mux.HandleFunc("GET /api/v1/analytics/countries", analyticsHandler.Countries)
	mux.HandleFunc("GET /api/v1/analytics/providers", analyticsHandler.Providers)
	mux.HandleFunc("GET /api/v1/analytics/timeseries", analyticsHandler.TimeSeries)
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.HandleFunc("GET /ready", healthHandler.Ready)
	mux.Handle("GET /metrics", promhttp.Handler())

	return WithCORS(corsAllowedOrigin, WithLogging(logger, WithMetrics(mux)))
}
