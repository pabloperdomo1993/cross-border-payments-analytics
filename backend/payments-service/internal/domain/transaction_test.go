package domain

import (
	"errors"
	"testing"
	"time"
)

func validParams() NewTransactionParams {
	return NewTransactionParams{
		ID:                  TransactionID("550e8400-e29b-41d4-a716-446655440000"),
		SourceCountry:       CountryCode("CO"),
		DestinationCountry:  CountryCode("US"),
		SourceCurrency:      CurrencyCode("COP"),
		DestinationCurrency: CurrencyCode("USD"),
		Amount:              Money(400_000_000), // 4,000,000.00 COP in minor units
		FXRate:              FXRate(26260),      // 0.00026260
		Provider:            Provider("provider_a"),
		CreatedAt:           time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestNewTransaction_Valid(t *testing.T) {
	tx, err := NewTransaction(validParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tx.Status != StatusPending {
		t.Errorf("expected initial status %q, got %q", StatusPending, tx.Status)
	}
}

func TestNewTransaction_DefaultsCreatedAt(t *testing.T) {
	params := validParams()
	params.CreatedAt = time.Time{}

	before := time.Now().UTC()
	tx, err := NewTransaction(params)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tx.CreatedAt.Before(before) || tx.CreatedAt.After(after) {
		t.Errorf("expected CreatedAt to default to now(), got %v (window %v - %v)", tx.CreatedAt, before, after)
	}
}

func TestNewTransaction_Validation(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(p *NewTransactionParams)
		wantField string
	}{
		{"empty ID", func(p *NewTransactionParams) { p.ID = "" }, "id"},
		{"invalid ID format", func(p *NewTransactionParams) { p.ID = "not-a-uuid" }, "id"},
		{"invalid source country lowercase", func(p *NewTransactionParams) { p.SourceCountry = "co" }, "source_country"},
		{"invalid destination country wrong length", func(p *NewTransactionParams) { p.DestinationCountry = "USA" }, "destination_country"},
		{"empty source country", func(p *NewTransactionParams) { p.SourceCountry = "" }, "source_country"},
		{"invalid source currency lowercase", func(p *NewTransactionParams) { p.SourceCurrency = "cop" }, "source_currency"},
		{"invalid destination currency wrong length", func(p *NewTransactionParams) { p.DestinationCurrency = "USDD" }, "destination_currency"},
		{"zero amount", func(p *NewTransactionParams) { p.Amount = 0 }, "amount"},
		{"negative amount", func(p *NewTransactionParams) { p.Amount = -100 }, "amount"},
		{"zero fx rate", func(p *NewTransactionParams) { p.FXRate = 0 }, "fx_rate"},
		{"negative fx rate", func(p *NewTransactionParams) { p.FXRate = -1 }, "fx_rate"},
		{"empty provider", func(p *NewTransactionParams) { p.Provider = "" }, "provider"},
		{"blank provider", func(p *NewTransactionParams) { p.Provider = "   " }, "provider"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := validParams()
			tt.mutate(&params)

			_, err := NewTransaction(params)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			verr, ok := AsValidationError(err)
			if !ok {
				t.Fatalf("expected *ValidationError, got %T: %v", err, err)
			}
			if _, ok := verr.Fields[tt.wantField]; !ok {
				t.Errorf("expected validation error on field %q, got fields %v", tt.wantField, verr.Fields)
			}
		})
	}
}

func TestNewTransaction_MultipleInvalidFields(t *testing.T) {
	params := validParams()
	params.Amount = 0
	params.Provider = ""

	_, err := NewTransaction(params)
	verr, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if len(verr.Fields) != 2 {
		t.Errorf("expected 2 invalid fields, got %d: %v", len(verr.Fields), verr.Fields)
	}
}

func TestTransaction_StatusTransitions_Valid(t *testing.T) {
	tests := []struct {
		name    string
		action  func(tx *Transaction) error
		wantEnd TransactionStatus
	}{
		{"pending to completed", (*Transaction).MarkCompleted, StatusCompleted},
		{"pending to failed", (*Transaction).MarkFailed, StatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := NewTransaction(validParams())
			if err != nil {
				t.Fatalf("unexpected error building transaction: %v", err)
			}

			if err := tt.action(tx); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tx.Status != tt.wantEnd {
				t.Errorf("expected status %q, got %q", tt.wantEnd, tx.Status)
			}
		})
	}
}

func TestTransaction_StatusTransitions_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		start  TransactionStatus
		action func(tx *Transaction) error
	}{
		{"completed to failed", StatusCompleted, (*Transaction).MarkFailed},
		{"failed to completed", StatusFailed, (*Transaction).MarkCompleted},
		{"completed to completed", StatusCompleted, (*Transaction).MarkCompleted},
		{"failed to failed", StatusFailed, (*Transaction).MarkFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := NewTransaction(validParams())
			if err != nil {
				t.Fatalf("unexpected error building transaction: %v", err)
			}
			tx.Status = tt.start

			err = tt.action(tx)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("expected ErrInvalidTransition, got %v", err)
			}
			if tx.Status != tt.start {
				t.Errorf("status should remain unchanged after invalid transition, got %q", tx.Status)
			}
		})
	}
}

func TestParseMoney(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Money
		wantErr bool
	}{
		{"whole with cents", "4000000.00", 400_000_000, false},
		{"single fractional digit", "10.5", 1050, false},
		{"no fractional part", "10", 1000, false},
		{"too many decimals", "10.123", 0, true},
		{"empty", "", 0, true},
		{"not a number", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMoney(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

func TestMoney_String_RoundTrip(t *testing.T) {
	m, err := ParseMoney("4000000.00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := m.String(); got != "4000000.00" {
		t.Errorf("expected round-trip %q, got %q", "4000000.00", got)
	}
}

func TestParseFXRate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    FXRate
		wantErr bool
	}{
		{"typical rate", "0.0002626", 26260, false},
		{"whole rate", "1", FXRateScale, false},
		{"too many decimals", "0.123456789", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFXRate(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %d, got %d", tt.want, got)
			}
		})
	}
}
