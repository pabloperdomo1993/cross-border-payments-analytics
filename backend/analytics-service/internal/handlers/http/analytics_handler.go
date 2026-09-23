package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/domain"
)

// AnalyticsRepository is the subset of repository.AnalyticsRepository
// the handler depends on, declared here so it's easy to test with a
// lightweight fake instead of a real ClickHouse connection.
type AnalyticsRepository interface {
	Corridors(ctx context.Context, filter domain.Filter) ([]domain.CorridorVolume, error)
	Currencies(ctx context.Context, filter domain.Filter) ([]domain.CurrencyVolume, error)
	Countries(ctx context.Context, filter domain.Filter, direction domain.CountryDirection) ([]domain.CountryVolume, error)
	Providers(ctx context.Context, filter domain.Filter) ([]domain.ProviderStats, error)
	TimeSeries(ctx context.Context, filter domain.Filter, interval domain.TimeInterval) ([]domain.TimeSeriesPoint, error)
}

// AnalyticsHandler serves the /api/v1/analytics endpoints. It has no
// SQL in it — filter parsing/validation lives in internal/domain, and
// every query lives behind AnalyticsRepository.
type AnalyticsHandler struct {
	repo   AnalyticsRepository
	logger *slog.Logger
}

// NewAnalyticsHandler builds an AnalyticsHandler.
func NewAnalyticsHandler(repo AnalyticsRepository, logger *slog.Logger) *AnalyticsHandler {
	return &AnalyticsHandler{repo: repo, logger: logger}
}

// parseFilter extracts and validates the shared filter query parameters
// common to every analytics endpoint.
func parseFilter(r *http.Request) (domain.Filter, error) {
	q := r.URL.Query()
	return domain.ParseFilter(
		q.Get("from"), q.Get("to"),
		q.Get("source_country"), q.Get("destination_country"),
		q.Get("source_currency"), q.Get("destination_currency"),
		q.Get("provider"), q.Get("status"),
	)
}

// Corridors handles GET /api/v1/analytics/corridors.
func (h *AnalyticsHandler) Corridors(w http.ResponseWriter, r *http.Request) {
	filter, err := parseFilter(r)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}

	results, err := h.repo.Corridors(r.Context(), filter)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"corridors": results})
}

// Currencies handles GET /api/v1/analytics/currencies.
func (h *AnalyticsHandler) Currencies(w http.ResponseWriter, r *http.Request) {
	filter, err := parseFilter(r)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}

	results, err := h.repo.Currencies(r.Context(), filter)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"currencies": results})
}

// Countries handles GET /api/v1/analytics/countries.
func (h *AnalyticsHandler) Countries(w http.ResponseWriter, r *http.Request) {
	filter, err := parseFilter(r)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}

	direction, err := domain.ParseCountryDirection(r.URL.Query().Get("by"))
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}

	results, err := h.repo.Countries(r.Context(), filter, direction)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"countries": results, "by": direction})
}

// Providers handles GET /api/v1/analytics/providers.
func (h *AnalyticsHandler) Providers(w http.ResponseWriter, r *http.Request) {
	filter, err := parseFilter(r)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}

	results, err := h.repo.Providers(r.Context(), filter)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": results})
}

// TimeSeries handles GET /api/v1/analytics/timeseries.
func (h *AnalyticsHandler) TimeSeries(w http.ResponseWriter, r *http.Request) {
	filter, err := parseFilter(r)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}

	interval, err := domain.ParseTimeInterval(r.URL.Query().Get("interval"))
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}

	results, err := h.repo.TimeSeries(r.Context(), filter, interval)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"timeseries": results, "interval": interval})
}
