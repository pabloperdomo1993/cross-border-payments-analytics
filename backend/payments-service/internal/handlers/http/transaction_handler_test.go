package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	httphandler "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/handlers/http"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/application"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeCreateUseCase and fakeGetUseCase let handler tests exercise
// success/failure paths without a real use case or repository.
type fakeCreateUseCase struct {
	tx  *domain.Transaction
	err error
}

func (f *fakeCreateUseCase) Execute(context.Context, application.CreateTransactionInput) (*domain.Transaction, error) {
	return f.tx, f.err
}

type fakeGetUseCase struct {
	tx  *domain.Transaction
	err error
}

func (f *fakeGetUseCase) Execute(context.Context, string) (*domain.Transaction, error) {
	return f.tx, f.err
}

func sampleTransaction(t *testing.T) *domain.Transaction {
	t.Helper()
	amount, err := domain.ParseMoney("4000000.00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fxRate, err := domain.ParseFXRate("0.0002626")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tx, err := domain.NewTransaction(domain.NewTransactionParams{
		ID:                  "550e8400-e29b-41d4-a716-446655440000",
		IdempotencyKey:      "idem-key-1",
		SourceCountry:       "CO",
		DestinationCountry:  "US",
		SourceCurrency:      "COP",
		DestinationCurrency: "USD",
		SourceAmount:        amount,
		DestinationAmount:   amount.Multiply(fxRate),
		FXRate:              fxRate,
		Provider:            "provider_a",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return tx
}

func TestTransactionHandler_Create_Valid(t *testing.T) {
	tx := sampleTransaction(t)
	handler := httphandler.NewTransactionHandler(&fakeCreateUseCase{tx: tx}, &fakeGetUseCase{}, discardLogger())

	body := bytes.NewBufferString(`{
		"idempotency_key": "idem-key-1",
		"source_country": "CO",
		"destination_country": "US",
		"source_currency": "COP",
		"destination_currency": "USD",
		"source_amount": "4000000.00",
		"fx_rate": "0.0002626",
		"provider": "provider_a"
	}`)
	req := httptest.NewRequest("POST", "/api/v1/transactions", body)
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got["id"] != string(tx.ID) {
		t.Errorf("expected id %q, got %v", tx.ID, got["id"])
	}
	if got["source_amount"] != "4000000.00" {
		t.Errorf("expected source_amount as decimal string, got %v", got["source_amount"])
	}
	if got["destination_amount"] != "1050.40" {
		t.Errorf("expected destination_amount as decimal string, got %v", got["destination_amount"])
	}
}

func TestTransactionHandler_Create_ValidationError(t *testing.T) {
	handler := httphandler.NewTransactionHandler(
		&fakeCreateUseCase{err: &domain.ValidationError{Fields: map[string]string{"amount": "must be greater than zero"}}},
		&fakeGetUseCase{},
		discardLogger(),
	)

	req := httptest.NewRequest("POST", "/api/v1/transactions", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var got map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got["error"]["code"] != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR code, got %v", got["error"]["code"])
	}
}

func TestTransactionHandler_Create_InvalidJSON(t *testing.T) {
	handler := httphandler.NewTransactionHandler(&fakeCreateUseCase{}, &fakeGetUseCase{}, discardLogger())

	req := httptest.NewRequest("POST", "/api/v1/transactions", bytes.NewBufferString(`not json`))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTransactionHandler_Create_InternalError(t *testing.T) {
	handler := httphandler.NewTransactionHandler(
		&fakeCreateUseCase{err: domain.ErrRepository},
		&fakeGetUseCase{},
		discardLogger(),
	)

	req := httptest.NewRequest("POST", "/api/v1/transactions", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != 500 {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("repository")) {
		t.Errorf("internal error details must not leak to the client, got body: %s", rec.Body.String())
	}
}

func TestTransactionHandler_Get_Found(t *testing.T) {
	tx := sampleTransaction(t)
	handler := httphandler.NewTransactionHandler(&fakeCreateUseCase{}, &fakeGetUseCase{tx: tx}, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/transactions/"+string(tx.ID), nil)
	req.SetPathValue("id", string(tx.ID))
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTransactionHandler_Get_NotFound(t *testing.T) {
	handler := httphandler.NewTransactionHandler(&fakeCreateUseCase{}, &fakeGetUseCase{err: domain.ErrTransactionNotFound}, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/transactions/unknown", nil)
	req.SetPathValue("id", "unknown")
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
