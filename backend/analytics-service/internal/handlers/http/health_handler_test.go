package http_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	httphandler "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/handlers/http"
)

// fakePinger lets health handler tests exercise the ready/not-ready
// paths without a real ClickHouse connection.
type fakePinger struct {
	err error
}

func (f *fakePinger) Ping(context.Context) error {
	return f.err
}

func TestHealthHandler_Health_AlwaysOK(t *testing.T) {
	handler := httphandler.NewHealthHandler(&fakePinger{err: errors.New("clickhouse unreachable")})

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

func TestHealthHandler_Ready_ClickHouseUp(t *testing.T) {
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

func TestHealthHandler_Ready_ClickHouseDown(t *testing.T) {
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
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Errorf("readiness response must not leak the raw error, got body: %s", rec.Body.String())
	}
}
