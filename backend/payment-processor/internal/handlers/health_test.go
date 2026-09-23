package handlers_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/handlers"
)

// fakeKafkaPinger lets readiness tests exercise the up/down paths
// without a real Kafka broker.
type fakeKafkaPinger struct {
	err error
}

func (f fakeKafkaPinger) Ping() error {
	return f.err
}

func TestHealth_AlwaysOK(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	handlers.Health(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestReady_KafkaUp(t *testing.T) {
	ready := handlers.Ready(fakeKafkaPinger{})

	req := httptest.NewRequest("GET", "/ready", nil)
	rec := httptest.NewRecorder()

	ready(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"status":"ready"}` {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestReady_KafkaDown(t *testing.T) {
	ready := handlers.Ready(fakeKafkaPinger{err: errors.New("dial tcp: connection refused")})

	req := httptest.NewRequest("GET", "/ready", nil)
	rec := httptest.NewRecorder()

	ready(rec, req)

	if rec.Code != 503 {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"status":"unavailable"}` {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}
