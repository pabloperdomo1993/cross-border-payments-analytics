package domain_test

import (
	"testing"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/domain"
)

func TestParseFilter_Empty(t *testing.T) {
	f, err := domain.ParseFilter("", "", "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.From.IsZero() || !f.To.IsZero() {
		t.Errorf("expected zero from/to for an empty filter, got %+v", f)
	}
}

func TestParseFilter_Valid(t *testing.T) {
	f, err := domain.ParseFilter("2026-09-01", "2026-09-30", "CO", "US", "COP", "USD", "provider_a", "completed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.SourceCountry != "CO" || f.DestinationCountry != "US" {
		t.Errorf("expected country filters to be set, got %+v", f)
	}
	if f.SourceCurrency != "COP" || f.DestinationCurrency != "USD" {
		t.Errorf("expected currency filters to be set, got %+v", f)
	}
	if f.Provider != "provider_a" || f.Status != "completed" {
		t.Errorf("expected provider/status filters to be set, got %+v", f)
	}
	// "to" should be exclusive of the day after (whole-day inclusive semantics).
	if f.To.Day() != 1 {
		t.Errorf("expected 'to' to roll over to the next day for inclusive range, got %v", f.To)
	}
}

func TestParseFilter_Invalid(t *testing.T) {
	tests := []struct {
		name      string
		from      string
		to        string
		srcCty    string
		dstCty    string
		srcCur    string
		dstCur    string
		status    string
		wantField string
	}{
		{"bad from date", "not-a-date", "", "", "", "", "", "", "from"},
		{"bad to date", "", "not-a-date", "", "", "", "", "", "to"},
		{"from after to", "2026-09-30", "2026-09-01", "", "", "", "", "", "to"},
		{"lowercase source country", "", "", "co", "", "", "", "", "source_country"},
		{"wrong length destination country", "", "", "", "USA", "", "", "", "destination_country"},
		{"lowercase source currency", "", "", "", "", "cop", "", "", "source_currency"},
		{"wrong length destination currency", "", "", "", "", "", "USDX", "", "destination_currency"},
		{"invalid status", "", "", "", "", "", "", "bogus", "status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.ParseFilter(tt.from, tt.to, tt.srcCty, tt.dstCty, tt.srcCur, tt.dstCur, "", tt.status)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			verr, ok := domain.AsValidationError(err)
			if !ok {
				t.Fatalf("expected *ValidationError, got %T: %v", err, err)
			}
			if _, ok := verr.Fields[tt.wantField]; !ok {
				t.Errorf("expected validation error on field %q, got fields %v", tt.wantField, verr.Fields)
			}
		})
	}
}

func TestParseCountryDirection(t *testing.T) {
	tests := []struct {
		input   string
		want    domain.CountryDirection
		wantErr bool
	}{
		{"", domain.CountryDirectionSource, false},
		{"source", domain.CountryDirectionSource, false},
		{"destination", domain.CountryDirectionDestination, false},
		{"DESTINATION", domain.CountryDirectionDestination, false},
		{"sideways", "", true},
	}

	for _, tt := range tests {
		got, err := domain.ParseCountryDirection(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("input %q: expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("input %q: unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("input %q: expected %q, got %q", tt.input, tt.want, got)
		}
	}
}

func TestParseTimeInterval(t *testing.T) {
	tests := []struct {
		input   string
		want    domain.TimeInterval
		wantErr bool
	}{
		{"", domain.TimeIntervalDay, false},
		{"day", domain.TimeIntervalDay, false},
		{"DAY", domain.TimeIntervalDay, false},
		{"hour", "", true},
		{"month", "", true},
	}

	for _, tt := range tests {
		got, err := domain.ParseTimeInterval(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("input %q: expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("input %q: unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("input %q: expected %q, got %q", tt.input, tt.want, got)
		}
	}
}
