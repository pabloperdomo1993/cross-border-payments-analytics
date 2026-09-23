package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/application"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

func validInput() application.CreateTransactionInput {
	return application.CreateTransactionInput{
		IdempotencyKey:      "idem-key-1",
		SourceCountry:       "CO",
		DestinationCountry:  "US",
		SourceCurrency:      "COP",
		DestinationCurrency: "USD",
		SourceAmount:        "4000000.00",
		FXRate:              "0.0002626",
		Provider:            "provider_a",
	}
}

func TestCreateTransaction_Valid(t *testing.T) {
	repo := newFakeRepository()
	uc := application.NewCreateTransaction(repo)

	tx, err := uc.Execute(context.Background(), validInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tx.Status != domain.StatusPending {
		t.Errorf("expected status %q, got %q", domain.StatusPending, tx.Status)
	}
	if tx.ID == "" {
		t.Error("expected a generated transaction ID")
	}
	if tx.DestinationAmount.String() != "1050.40" {
		t.Errorf("expected computed destination_amount 1050.40, got %s", tx.DestinationAmount.String())
	}
	if _, ok := repo.transactions[string(tx.ID)]; !ok {
		t.Error("expected transaction to be persisted in the repository")
	}

	if repo.lastEvent == nil {
		t.Fatal("expected an outbox event to be created alongside the transaction")
	}
	if repo.lastEvent.AggregateID != string(tx.ID) {
		t.Errorf("expected outbox event aggregate_id %q, got %q", tx.ID, repo.lastEvent.AggregateID)
	}
	if repo.lastEvent.EventType != application.EventTypePaymentCreated {
		t.Errorf("expected outbox event type %q, got %q", application.EventTypePaymentCreated, repo.lastEvent.EventType)
	}
	if repo.lastEvent.Status != domain.OutboxStatusPending {
		t.Errorf("expected outbox event status %q, got %q", domain.OutboxStatusPending, repo.lastEvent.Status)
	}
	if !strings.Contains(repo.lastEvent.Payload, string(tx.ID)) {
		t.Errorf("expected outbox event payload to reference transaction id %q, got %s", tx.ID, repo.lastEvent.Payload)
	}
}

func TestCreateTransaction_ContextCancellation(t *testing.T) {
	repo := newFakeRepository()
	uc := application.NewCreateTransaction(repo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := uc.Execute(ctx, validInput())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if len(repo.transactions) != 0 {
		t.Error("expected no transaction to be persisted when the context is already cancelled")
	}
}

func TestCreateTransaction_DuplicateIdempotencyKey(t *testing.T) {
	repo := newFakeRepository()
	repo.createErr = domain.ErrConflict
	uc := application.NewCreateTransaction(repo)

	_, err := uc.Execute(context.Background(), validInput())
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("expected error to wrap domain.ErrConflict for a duplicate idempotency key, got %v", err)
	}
}

func TestCreateTransaction_Validation(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(in *application.CreateTransactionInput)
		wantField string
	}{
		{"invalid amount", func(in *application.CreateTransactionInput) { in.SourceAmount = "0" }, "source_amount"},
		{"non-numeric amount", func(in *application.CreateTransactionInput) { in.SourceAmount = "abc" }, "source_amount"},
		{"invalid fx_rate", func(in *application.CreateTransactionInput) { in.FXRate = "0" }, "fx_rate"},
		{"non-numeric fx_rate", func(in *application.CreateTransactionInput) { in.FXRate = "abc" }, "fx_rate"},
		{"missing source country", func(in *application.CreateTransactionInput) { in.SourceCountry = "" }, "source_country"},
		{"invalid destination country", func(in *application.CreateTransactionInput) { in.DestinationCountry = "usa" }, "destination_country"},
		{"invalid source currency", func(in *application.CreateTransactionInput) { in.SourceCurrency = "xx" }, "source_currency"},
		{"invalid destination currency", func(in *application.CreateTransactionInput) { in.DestinationCurrency = "US" }, "destination_currency"},
		{"missing provider", func(in *application.CreateTransactionInput) { in.Provider = "" }, "provider"},
		{"missing idempotency key", func(in *application.CreateTransactionInput) { in.IdempotencyKey = "" }, "idempotency_key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			uc := application.NewCreateTransaction(repo)

			input := validInput()
			tt.mutate(&input)

			_, err := uc.Execute(context.Background(), input)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			verr, ok := domain.AsValidationError(err)
			if !ok {
				t.Fatalf("expected *domain.ValidationError, got %T: %v", err, err)
			}
			if _, ok := verr.Fields[tt.wantField]; !ok {
				t.Errorf("expected validation error on field %q, got fields %v", tt.wantField, verr.Fields)
			}
			if len(repo.transactions) != 0 {
				t.Error("expected no transaction to be persisted on validation failure")
			}
		})
	}
}

func TestCreateTransaction_RepositoryFailure(t *testing.T) {
	repo := newFakeRepository()
	repo.createErr = errRepositoryFailure
	uc := application.NewCreateTransaction(repo)

	_, err := uc.Execute(context.Background(), validInput())
	if !errors.Is(err, errRepositoryFailure) {
		t.Errorf("expected error to wrap repository failure, got %v", err)
	}
}
