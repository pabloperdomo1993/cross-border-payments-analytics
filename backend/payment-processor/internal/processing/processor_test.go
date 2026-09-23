package processing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/domain"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/processing"
)

func validEvent() domain.PaymentEvent {
	return domain.PaymentEvent{
		TransactionID:       "550e8400-e29b-41d4-a716-446655440000",
		SourceCountry:       "CO",
		DestinationCountry:  "US",
		SourceCurrency:      "COP",
		DestinationCurrency: "USD",
		Amount:              "4000000.00",
		FXRate:              "0.0002626",
		Provider:            "provider_a",
	}
}

func TestDefaultProcessor_Valid(t *testing.T) {
	p := processing.NewDefaultProcessor()

	outcome, err := p.Process(context.Background(), validEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Status != domain.StatusCompleted {
		t.Errorf("expected status %q, got %q", domain.StatusCompleted, outcome.Status)
	}
	if outcome.TransactionID != validEvent().TransactionID {
		t.Errorf("expected outcome to carry the transaction id")
	}
}

func TestDefaultProcessor_NonRetryableErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(e *domain.PaymentEvent)
	}{
		{"missing transaction id", func(e *domain.PaymentEvent) { e.TransactionID = "" }},
		{"invalid source country", func(e *domain.PaymentEvent) { e.SourceCountry = "col" }},
		{"invalid destination country", func(e *domain.PaymentEvent) { e.DestinationCountry = "USA" }},
		{"invalid source currency format", func(e *domain.PaymentEvent) { e.SourceCurrency = "cop" }},
		{"unsupported currency", func(e *domain.PaymentEvent) { e.SourceCurrency = "XXX" }},
		{"zero amount", func(e *domain.PaymentEvent) { e.Amount = "0.00" }},
		{"non-numeric amount", func(e *domain.PaymentEvent) { e.Amount = "abc" }},
		{"negative amount", func(e *domain.PaymentEvent) { e.Amount = "-5.00" }},
		{"zero fx_rate", func(e *domain.PaymentEvent) { e.FXRate = "0" }},
		{"missing provider", func(e *domain.PaymentEvent) { e.Provider = "" }},
	}

	p := processing.NewDefaultProcessor()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validEvent()
			tt.mutate(&event)

			_, err := p.Process(context.Background(), event)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if processing.IsRetryable(err) {
				t.Errorf("expected non-retryable error, got retryable: %v", err)
			}
		})
	}
}

func TestDefaultProcessor_ContextCancelledIsRetryable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := processing.NewDefaultProcessor()
	_, err := p.Process(ctx, validEvent())
	if err == nil {
		t.Fatal("expected error for a cancelled context")
	}
	if !processing.IsRetryable(err) {
		t.Errorf("expected a cancelled context to be classified retryable, got: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected error to wrap context.Canceled, got: %v", err)
	}
}

func TestRetry_SucceedsWithoutRetryingOnSuccess(t *testing.T) {
	calls := 0
	err := processing.Retry(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 call on immediate success, got %d", calls)
	}
}

func TestRetry_StopsImmediatelyOnNonRetryable(t *testing.T) {
	calls := 0
	sentinel := processing.NonRetryable(errors.New("bad event"))
	err := processing.Retry(context.Background(), 5, time.Millisecond, func() error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the non-retryable error to be returned, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 call for a non-retryable error, got %d", calls)
	}
}

func TestRetry_RetriesUpToMaxAttempts(t *testing.T) {
	calls := 0
	err := processing.Retry(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return processing.Retryable(errors.New("transient"))
	})
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if calls != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", calls)
	}
}

func TestRetry_StopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := processing.Retry(ctx, 10, 50*time.Millisecond, func() error {
		calls++
		if calls == 1 {
			cancel()
		}
		return processing.Retryable(errors.New("transient"))
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("expected retry loop to stop after cancellation, got %d calls", calls)
	}
}
