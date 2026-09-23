package http_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	httphandler "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/handlers/http"
)

// fakePinger lets health handler tests exercise the ready/not-ready
// paths without a real database connection.
type fakePinger struct {
	err error
}

func (f *fakePinger) PingContext(context.Context) error {
	return f.err
}

func TestHealthHandler_Health_AlwaysOK(t *testing.T) {
	handler := httphandler.NewHealthHandler(&fakePinger{err: errors.New("db unreachable")})

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"status":"ok"}`+"\n" {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestHealthHandler_Ready_DatabaseUp(t *testing.T) {
	handler := httphandler.NewHealthHandler(&fakePinger{})

	req := httptest.NewRequest("GET", "/ready", nil)
	rec := httptest.NewRecorder()

	handler.Ready(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"status":"ready"}`+"\n" {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestHealthHandler_Ready_DatabaseDown(t *testing.T) {
	handler := httphandler.NewHealthHandler(&fakePinger{err: errors.New("connection refused")})

	req := httptest.NewRequest("GET", "/ready", nil)
	rec := httptest.NewRecorder()

	handler.Ready(rec, req)

	if rec.Code != 503 {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"status":"unavailable"}`+"\n" {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}
