// Package domain contains the core business types for the payments
// service. It has no dependency on any database driver, messaging system,
// HTTP framework, or other infrastructure concern.
package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TransactionID uniquely identifies a Transaction.
//
// It is a validated string rather than a type from an external UUID
// library. Adding a dependency such as google/uuid to the domain layer
// would couple business logic to a specific ID-generation implementation
// for no real benefit here: the domain only needs to know "this looks
// like a UUID and isn't empty", not how to generate or parse one.
// Generation stays the responsibility of whatever layer creates
// transactions (the application layer), which is free to use any
// strategy and simply pass the resulting string in.
type TransactionID string

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Validate reports whether id is a non-empty, UUID-shaped identifier.
func (id TransactionID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return &ValidationError{Fields: map[string]string{"id": "must not be empty"}}
	}
	if !uuidPattern.MatchString(string(id)) {
		return &ValidationError{Fields: map[string]string{"id": "must be a valid UUID"}}
	}
	return nil
}

// CountryCode is an ISO 3166-1 alpha-2 country code (e.g. "CO", "US").
//
// Validation here only checks the two-uppercase-letter shape, not
// membership in the actual ISO 3166-1 list. Embedding and maintaining
// the full country list is a data/lookup concern better suited to a
// dedicated reference-data package once it's actually needed; the domain
// type still prevents obviously malformed values (empty, lowercase,
// wrong length) from flowing through the system.
type CountryCode string

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

// Validate reports whether c has the shape of an ISO 3166-1 alpha-2 code.
func (c CountryCode) Validate() error {
	if !countryCodePattern.MatchString(string(c)) {
		return fmt.Errorf("must be a 2-letter ISO 3166-1 country code, got %q", string(c))
	}
	return nil
}

// CurrencyCode is an ISO 4217 currency code (e.g. "USD", "COP").
//
// As with CountryCode, this validates shape (three uppercase letters)
// rather than membership in the full ISO 4217 list.
type CurrencyCode string

var currencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)

// Validate reports whether c has the shape of an ISO 4217 currency code.
func (c CurrencyCode) Validate() error {
	if !currencyCodePattern.MatchString(string(c)) {
		return fmt.Errorf("must be a 3-letter ISO 4217 currency code, got %q", string(c))
	}
	return nil
}

// moneyScale is the number of decimal digits Money strings are expected
// to carry (2, matching the minor unit of most currencies, e.g. cents).
const moneyScale = 2

// Money represents a monetary amount in the minor unit of its currency
// (e.g. cents for USD, centavos for COP), stored as an integer.
//
// float32/float64 are avoided deliberately: binary floating-point cannot
// represent most decimal fractions exactly, and repeated arithmetic on
// monetary values would accumulate rounding error. Representing the
// amount as a whole number of minor units keeps arithmetic exact using
// plain integer math and matches how most payment providers (and ISO
// 4217 itself) express amounts. The API accepts/returns Money as a
// decimal string (e.g. "4000000.00") so JSON encoding never touches
// float64 either.
type Money int64

// ParseMoney parses a decimal string with at most moneyScale fractional
// digits (e.g. "4000000.00") into a Money value.
func ParseMoney(s string) (Money, error) {
	units, err := parseFixedPoint(s, moneyScale)
	if err != nil {
		return 0, fmt.Errorf("invalid amount %q: %w", s, err)
	}
	return Money(units), nil
}

// String renders the amount back as a decimal string, e.g. "4000000.00".
func (m Money) String() string {
	return formatFixedPoint(int64(m), moneyScale)
}

// Validate reports whether the amount is greater than zero.
func (m Money) Validate() error {
	if m <= 0 {
		return fmt.Errorf("must be greater than zero")
	}
	return nil
}

// fxRateScaleDigits is the number of decimal digits FXRate strings carry.
const fxRateScaleDigits = 8

// FXRateScale is the fixed-point scale applied to FXRate values. An
// FXRate of 1 * FXRateScale represents an exchange rate of 1.00000000.
const FXRateScale = 100_000_000

// FXRate represents a foreign-exchange rate as a fixed-point integer
// scaled by FXRateScale (8 decimal digits of precision).
//
// Unlike Money, an FX rate isn't naturally expressed in "minor units" of
// a single currency — it's a ratio between two currencies and often
// needs more decimal precision than either currency's minor unit allows
// (e.g. 1 USD = 3901.42357 COP, or the reverse: 0.0002626). float64 is
// still avoided for the same reason as Money: exact, reproducible
// arithmetic matters for financial calculations. A fixed-point integer
// scaled by FXRateScale keeps rate arithmetic exact with plain int64
// math, at the cost of a fixed precision ceiling (8 decimal digits). If
// a future requirement needs arbitrary precision, math/big.Rat (stdlib)
// or a decimal library such as shopspring/decimal would be the natural
// upgrade path.
type FXRate int64

// ParseFXRate parses a decimal string with at most fxRateScaleDigits
// fractional digits (e.g. "0.0002626") into an FXRate value.
func ParseFXRate(s string) (FXRate, error) {
	units, err := parseFixedPoint(s, fxRateScaleDigits)
	if err != nil {
		return 0, fmt.Errorf("invalid fx_rate %q: %w", s, err)
	}
	return FXRate(units), nil
}

// String renders the rate back as a decimal string, e.g. "0.00026260".
func (r FXRate) String() string {
	return formatFixedPoint(int64(r), fxRateScaleDigits)
}

// Validate reports whether the rate is greater than zero.
func (r FXRate) Validate() error {
	if r <= 0 {
		return fmt.Errorf("must be greater than zero")
	}
	return nil
}

// parseFixedPoint parses a decimal string such as "123.45" into an
// integer number of units at the given number of fractional digits
// (scale), without ever going through a floating-point representation.
func parseFixedPoint(s string, scale int) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("must not be empty")
	}

	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}

	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if intPart == "" || (hasFrac && fracPart == "") {
		return 0, fmt.Errorf("must be a decimal number")
	}
	if len(fracPart) > scale {
		return 0, fmt.Errorf("must not have more than %d decimal digits", scale)
	}
	fracPart += strings.Repeat("0", scale-len(fracPart))

	digits := intPart + fracPart
	units, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("must be a decimal number")
	}
	if negative {
		units = -units
	}
	return units, nil
}

// formatFixedPoint is the inverse of parseFixedPoint.
func formatFixedPoint(units int64, scale int) string {
	negative := units < 0
	if negative {
		units = -units
	}

	digits := strconv.FormatInt(units, 10)
	for len(digits) <= scale {
		digits = "0" + digits
	}
	intPart := digits[:len(digits)-scale]
	fracPart := digits[len(digits)-scale:]

	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s%s.%s", sign, intPart, fracPart)
}

// Provider identifies the external payment/FX provider handling a
// transaction (e.g. "provider_a"). It is intentionally a lightweight
// string type for now — a dedicated provider entity/model is out of
// scope until the domain actually needs provider-specific behavior.
type Provider string

// Validate reports whether the provider is non-empty.
func (p Provider) Validate() error {
	if strings.TrimSpace(string(p)) == "" {
		return fmt.Errorf("must not be empty")
	}
	return nil
}

// TransactionStatus represents the lifecycle state of a Transaction.
type TransactionStatus string

const (
	StatusPending   TransactionStatus = "pending"
	StatusCompleted TransactionStatus = "completed"
	StatusFailed    TransactionStatus = "failed"
)

// Validate reports whether s is one of the known transaction statuses.
func (s TransactionStatus) Validate() error {
	switch s {
	case StatusPending, StatusCompleted, StatusFailed:
		return nil
	default:
		return fmt.Errorf("invalid transaction status %q", string(s))
	}
}

// allowedTransitions is the transaction status state machine. Modeling
// it as a simple adjacency map (rather than a state-machine library)
// keeps the rules explicit and easy to read while still making invalid
// transitions, such as completed -> failed, structurally impossible to
// perform through the Transaction API.
var allowedTransitions = map[TransactionStatus][]TransactionStatus{
	StatusPending:   {StatusCompleted, StatusFailed},
	StatusCompleted: {},
	StatusFailed:    {},
}

func (s TransactionStatus) canTransitionTo(target TransactionStatus) bool {
	for _, allowed := range allowedTransitions[s] {
		if allowed == target {
			return true
		}
	}
	return false
}

// Transaction represents a single cross-border payment moving funds from
// a source country/currency to a destination country/currency through an
// external provider.
type Transaction struct {
	ID                  TransactionID
	SourceCountry       CountryCode
	DestinationCountry  CountryCode
	SourceCurrency      CurrencyCode
	DestinationCurrency CurrencyCode
	Amount              Money
	FXRate              FXRate
	Provider            Provider
	Status              TransactionStatus
	CreatedAt           time.Time
}

// NewTransactionParams carries the inputs required to create a new
// Transaction. CreatedAt is optional; if zero, the current time (UTC) is
// used.
type NewTransactionParams struct {
	ID                  TransactionID
	SourceCountry       CountryCode
	DestinationCountry  CountryCode
	SourceCurrency      CurrencyCode
	DestinationCurrency CurrencyCode
	Amount              Money
	FXRate              FXRate
	Provider            Provider
	CreatedAt           time.Time
}

// NewTransaction validates params and constructs a new Transaction in
// StatusPending. It returns a *ValidationError, with one entry per
// invalid field, if any field fails domain validation.
func NewTransaction(p NewTransactionParams) (*Transaction, error) {
	fields := map[string]string{}

	if err := p.ID.Validate(); err != nil {
		fields["id"] = err.Error()
	}
	if err := p.SourceCountry.Validate(); err != nil {
		fields["source_country"] = err.Error()
	}
	if err := p.DestinationCountry.Validate(); err != nil {
		fields["destination_country"] = err.Error()
	}
	if err := p.SourceCurrency.Validate(); err != nil {
		fields["source_currency"] = err.Error()
	}
	if err := p.DestinationCurrency.Validate(); err != nil {
		fields["destination_currency"] = err.Error()
	}
	if err := p.Amount.Validate(); err != nil {
		fields["amount"] = err.Error()
	}
	if err := p.FXRate.Validate(); err != nil {
		fields["fx_rate"] = err.Error()
	}
	if err := p.Provider.Validate(); err != nil {
		fields["provider"] = err.Error()
	}

	if len(fields) > 0 {
		return nil, &ValidationError{Fields: fields}
	}

	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	return &Transaction{
		ID:                  p.ID,
		SourceCountry:       p.SourceCountry,
		DestinationCountry:  p.DestinationCountry,
		SourceCurrency:      p.SourceCurrency,
		DestinationCurrency: p.DestinationCurrency,
		Amount:              p.Amount,
		FXRate:              p.FXRate,
		Provider:            p.Provider,
		Status:              StatusPending,
		CreatedAt:           createdAt,
	}, nil
}

// transitionTo moves the transaction to target if, and only if, that
// transition is allowed from the current status.
func (t *Transaction) transitionTo(target TransactionStatus) error {
	if !t.Status.canTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, t.Status, target)
	}
	t.Status = target
	return nil
}

// MarkCompleted transitions the transaction from pending to completed.
func (t *Transaction) MarkCompleted() error {
	return t.transitionTo(StatusCompleted)
}

// MarkFailed transitions the transaction from pending to failed.
func (t *Transaction) MarkFailed() error {
	return t.transitionTo(StatusFailed)
}
