package processing

import (
	"context"
	"fmt"
	"regexp"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/domain"
)

// Processor applies business validation to a PaymentEvent and produces
// an Outcome. It has no Kafka dependency, so it's directly unit
// testable without any broker or fakes beyond a context.
type Processor interface {
	Process(ctx context.Context, event domain.PaymentEvent) (domain.Outcome, error)
}

// DefaultProcessor is payment-processor's business-rule implementation.
// There is no real external payment provider to call yet, so all
// failure modes here come from the event's own data, not a simulated
// downstream dependency — see internal/kafka's publish-retry loop for
// where a genuine transient (retryable) failure mode lives (publishing
// the outcome back to Kafka).
type DefaultProcessor struct{}

// NewDefaultProcessor builds a DefaultProcessor.
func NewDefaultProcessor() *DefaultProcessor {
	return &DefaultProcessor{}
}

var (
	countryCodePattern  = regexp.MustCompile(`^[A-Z]{2}$`)
	currencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)
	decimalPattern      = regexp.MustCompile(`^\d+(\.\d+)?$`)
)

// supportedCurrencies is a minimal allow-list demonstrating the
// "unsupported currency" non-retryable path; a real system would source
// this from configuration or a reference-data service.
var supportedCurrencies = map[string]bool{
	"USD": true, "COP": true, "MXN": true, "BRL": true, "EUR": true,
}

// Process validates event and returns the resulting Outcome. All
// returned errors are *ProcessingError so callers can branch on
// IsRetryable without inspecting sentinel types themselves.
func (p *DefaultProcessor) Process(ctx context.Context, event domain.PaymentEvent) (domain.Outcome, error) {
	select {
	case <-ctx.Done():
		return domain.Outcome{}, Retryable(ctx.Err())
	default:
	}

	if event.TransactionID == "" {
		return domain.Outcome{}, NonRetryable(fmt.Errorf("%w: missing transaction id", ErrInvalidPaymentData))
	}
	if !countryCodePattern.MatchString(event.SourceCountry) || !countryCodePattern.MatchString(event.DestinationCountry) {
		return domain.Outcome{}, NonRetryable(fmt.Errorf("%w: invalid country code", ErrInvalidPaymentData))
	}
	if !currencyCodePattern.MatchString(event.SourceCurrency) || !currencyCodePattern.MatchString(event.DestinationCurrency) {
		return domain.Outcome{}, NonRetryable(fmt.Errorf("%w: invalid currency code", ErrInvalidPaymentData))
	}
	if !supportedCurrencies[event.SourceCurrency] || !supportedCurrencies[event.DestinationCurrency] {
		return domain.Outcome{}, NonRetryable(fmt.Errorf("%w: %s/%s", ErrUnsupportedCurrency, event.SourceCurrency, event.DestinationCurrency))
	}
	if !isPositiveDecimal(event.Amount) {
		return domain.Outcome{}, NonRetryable(fmt.Errorf("%w: invalid amount %q", ErrInvalidPaymentData, event.Amount))
	}
	if !isPositiveDecimal(event.FXRate) {
		return domain.Outcome{}, NonRetryable(fmt.Errorf("%w: invalid fx_rate %q", ErrInvalidPaymentData, event.FXRate))
	}
	if event.Provider == "" {
		return domain.Outcome{}, NonRetryable(fmt.Errorf("%w: missing provider", ErrInvalidPaymentData))
	}

	return domain.Outcome{TransactionID: event.TransactionID, Status: domain.StatusCompleted}, nil
}

// isPositiveDecimal reports whether s is a non-negative decimal string
// with at least one non-zero digit. This intentionally never parses
// through float64 — even for this validation-only check — to stay
// consistent with the project's rule that monetary values never touch
// binary floating point.
func isPositiveDecimal(s string) bool {
	if !decimalPattern.MatchString(s) {
		return false
	}
	for _, r := range s {
		if r >= '1' && r <= '9' {
			return true
		}
	}
	return false
}
