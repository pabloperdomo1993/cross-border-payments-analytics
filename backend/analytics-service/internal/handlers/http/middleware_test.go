package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus/testutil"

	httphandler "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/handlers/http"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/metrics"
)

// histogramSampleCount reads how many observations a HistogramVec's
// series has recorded, so tests can assert WithMetrics observed a
// request duration without depending on the actual duration value.
func histogramSampleCount(t *testing.T, method, pattern string) uint64 {
	t.Helper()
	observer := metrics.HTTPRequestDuration.WithLabelValues(method, pattern)
	metric, ok := observer.(interface{ Write(*dto.Metric) error })
	if !ok {
		t.Fatalf("observer does not implement Write; cannot inspect sample count")
	}
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("failed to collect histogram metric: %v", err)
	}
	return m.GetHistogram().GetSampleCount()
}

func TestWithMetrics_RecordsRequestByPattern(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /widgets/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := httphandler.WithMetrics(mux)

	requestsCounter := metrics.HTTPRequestsTotal.WithLabelValues("GET", "GET /widgets/{id}", "200")
	requestsBefore := testutil.ToFloat64(requestsCounter)
	durationBefore := histogramSampleCount(t, "GET", "GET /widgets/{id}")

	req := httptest.NewRequest("GET", "/widgets/abc-123", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	requestsAfter := testutil.ToFloat64(requestsCounter)
	if requestsAfter-requestsBefore != 1 {
		t.Errorf("expected http_requests_total{method=GET,pattern=GET /widgets/{id},status=200} to increment by 1, went from %v to %v", requestsBefore, requestsAfter)
	}

	durationAfter := histogramSampleCount(t, "GET", "GET /widgets/{id}")
	if durationAfter-durationBefore != 1 {
		t.Errorf("expected http_request_duration_seconds{method=GET,pattern=GET /widgets/{id}} to record 1 observation, went from %d to %d", durationBefore, durationAfter)
	}
}

func TestWithMetrics_RecordsUnmatchedRouteWith404Status(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /widgets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := httphandler.WithMetrics(mux)

	counter := metrics.HTTPRequestsTotal.WithLabelValues("GET", "unmatched", "404")
	before := testutil.ToFloat64(counter)

	req := httptest.NewRequest("GET", "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	after := testutil.ToFloat64(counter)
	if after-before != 1 {
		t.Errorf("expected http_requests_total{method=GET,pattern=unmatched,status=404} to increment by 1, went from %v to %v", before, after)
	}
}
