package http

import (
	"log/slog"
	"net/http"
)

// NewRouter builds the complete HTTP handler for analytics-service.
func NewRouter(analyticsHandler *AnalyticsHandler, healthHandler *HealthHandler, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/analytics/corridors", analyticsHandler.Corridors)
	mux.HandleFunc("GET /api/v1/analytics/currencies", analyticsHandler.Currencies)
	mux.HandleFunc("GET /api/v1/analytics/countries", analyticsHandler.Countries)
	mux.HandleFunc("GET /api/v1/analytics/providers", analyticsHandler.Providers)
	mux.HandleFunc("GET /api/v1/analytics/timeseries", analyticsHandler.TimeSeries)
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.HandleFunc("GET /ready", healthHandler.Ready)

	return WithLogging(logger, mux)
}
