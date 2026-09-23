package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	httphandler "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/handlers/http"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/domain"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeRepo struct {
	corridors  []domain.CorridorVolume
	currencies []domain.CurrencyVolume
	countries  []domain.CountryVolume
	providers  []domain.ProviderStats
	timeSeries []domain.TimeSeriesPoint
	err        error

	lastFilter    domain.Filter
	lastDirection domain.CountryDirection
	lastInterval  domain.TimeInterval
}

func (f *fakeRepo) Corridors(ctx context.Context, filter domain.Filter) ([]domain.CorridorVolume, error) {
	f.lastFilter = filter
	return f.corridors, f.err
}
func (f *fakeRepo) Currencies(ctx context.Context, filter domain.Filter) ([]domain.CurrencyVolume, error) {
	f.lastFilter = filter
	return f.currencies, f.err
}
func (f *fakeRepo) Countries(ctx context.Context, filter domain.Filter, direction domain.CountryDirection) ([]domain.CountryVolume, error) {
	f.lastFilter = filter
	f.lastDirection = direction
	return f.countries, f.err
}
func (f *fakeRepo) Providers(ctx context.Context, filter domain.Filter) ([]domain.ProviderStats, error) {
	f.lastFilter = filter
	return f.providers, f.err
}
func (f *fakeRepo) TimeSeries(ctx context.Context, filter domain.Filter, interval domain.TimeInterval) ([]domain.TimeSeriesPoint, error) {
	f.lastFilter = filter
	f.lastInterval = interval
	return f.timeSeries, f.err
}

func TestAnalyticsHandler_Corridors_Valid(t *testing.T) {
	repo := &fakeRepo{corridors: []domain.CorridorVolume{
		{SourceCountry: "CO", DestinationCountry: "US", TransactionCount: 10, TotalVolume: "1000.00"},
	}}
	handler := httphandler.NewAnalyticsHandler(repo, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/corridors?from=2026-09-01&to=2026-09-30", nil)
	rec := httptest.NewRecorder()

	handler.Corridors(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.lastFilter.From.IsZero() || repo.lastFilter.To.IsZero() {
		t.Errorf("expected from/to to be parsed into the filter, got %+v", repo.lastFilter)
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	corridors, ok := got["corridors"].([]any)
	if !ok || len(corridors) != 1 {
		t.Fatalf("expected 1 corridor in response, got %v", got)
	}
}

func TestAnalyticsHandler_Corridors_InvalidFilter(t *testing.T) {
	handler := httphandler.NewAnalyticsHandler(&fakeRepo{}, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/corridors?source_country=colombia", nil)
	rec := httptest.NewRecorder()

	handler.Corridors(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAnalyticsHandler_Corridors_InvalidDateRange(t *testing.T) {
	handler := httphandler.NewAnalyticsHandler(&fakeRepo{}, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/corridors?from=2026-09-30&to=2026-09-01", nil)
	rec := httptest.NewRecorder()

	handler.Corridors(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for from > to, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAnalyticsHandler_Corridors_EmptyResult(t *testing.T) {
	handler := httphandler.NewAnalyticsHandler(&fakeRepo{corridors: []domain.CorridorVolume{}}, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/corridors", nil)
	rec := httptest.NewRecorder()

	handler.Corridors(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200 for an empty dataset, got %d: %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	corridors, ok := got["corridors"].([]any)
	if !ok || len(corridors) != 0 {
		t.Fatalf("expected an empty (not null) corridors array, got %v", got["corridors"])
	}
}

func TestAnalyticsHandler_Countries_DefaultsToSource(t *testing.T) {
	repo := &fakeRepo{countries: []domain.CountryVolume{{Country: "CO", TransactionCount: 5, TotalVolume: "500.00"}}}
	handler := httphandler.NewAnalyticsHandler(repo, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/countries", nil)
	rec := httptest.NewRecorder()

	handler.Countries(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.lastDirection != domain.CountryDirectionSource {
		t.Errorf("expected default direction %q, got %q", domain.CountryDirectionSource, repo.lastDirection)
	}
}

func TestAnalyticsHandler_Countries_ByDestination(t *testing.T) {
	repo := &fakeRepo{}
	handler := httphandler.NewAnalyticsHandler(repo, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/countries?by=destination", nil)
	rec := httptest.NewRecorder()

	handler.Countries(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.lastDirection != domain.CountryDirectionDestination {
		t.Errorf("expected direction %q, got %q", domain.CountryDirectionDestination, repo.lastDirection)
	}
}

func TestAnalyticsHandler_Countries_InvalidDirection(t *testing.T) {
	handler := httphandler.NewAnalyticsHandler(&fakeRepo{}, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/countries?by=sideways", nil)
	rec := httptest.NewRecorder()

	handler.Countries(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for an invalid 'by' value, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAnalyticsHandler_Providers(t *testing.T) {
	repo := &fakeRepo{providers: []domain.ProviderStats{
		{Provider: "provider_a", TotalTransactions: 100, CompletedTransactions: 90, FailedTransactions: 10, FailureRate: 0.1},
	}}
	handler := httphandler.NewAnalyticsHandler(repo, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/providers?provider=provider_a&status=completed", nil)
	rec := httptest.NewRecorder()

	handler.Providers(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.lastFilter.Provider != "provider_a" || repo.lastFilter.Status != "completed" {
		t.Errorf("expected provider/status filters to be forwarded, got %+v", repo.lastFilter)
	}
}

func TestAnalyticsHandler_Providers_InvalidStatus(t *testing.T) {
	handler := httphandler.NewAnalyticsHandler(&fakeRepo{}, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/providers?status=bogus", nil)
	rec := httptest.NewRecorder()

	handler.Providers(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for an invalid status, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAnalyticsHandler_TimeSeries_DefaultsToDay(t *testing.T) {
	repo := &fakeRepo{timeSeries: []domain.TimeSeriesPoint{{PeriodStart: "2026-09-01", TransactionCount: 3, TotalVolume: "300.00"}}}
	handler := httphandler.NewAnalyticsHandler(repo, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/timeseries", nil)
	rec := httptest.NewRecorder()

	handler.TimeSeries(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.lastInterval != domain.TimeIntervalDay {
		t.Errorf("expected default interval %q, got %q", domain.TimeIntervalDay, repo.lastInterval)
	}
}

func TestAnalyticsHandler_TimeSeries_InvalidInterval(t *testing.T) {
	handler := httphandler.NewAnalyticsHandler(&fakeRepo{}, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/analytics/timeseries?interval=hour", nil)
	rec := httptest.NewRecorder()

	handler.TimeSeries(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for an unsupported interval, got %d: %s", rec.Code, rec.Body.String())
	}
}
