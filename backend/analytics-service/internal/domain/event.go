// Package domain contains analytics-service's own small domain: the
// wire shape it consumes from Kafka and the result types its analytics
// queries produce. It is intentionally independent from
// payments-service/payment-processor's domain packages (Go internal
// packages aren't importable across modules anyway, and each service
// owning its own wire-level types is the established boundary in this
// monorepo, not accidental duplication).
package domain

// PaymentOutcome is the payload consumed from the payments.processed
// (and payments.dlq) Kafka topics. Monetary fields and FXRate are
// decimal strings, never float64, matching every other service's wire
// convention in this system.
type PaymentOutcome struct {
	TransactionID       string `json:"id"`
	SourceCountry       string `json:"source_country"`
	DestinationCountry  string `json:"destination_country"`
	SourceCurrency      string `json:"source_currency"`
	DestinationCurrency string `json:"destination_currency"`
	SourceAmount        string `json:"source_amount"`
	DestinationAmount   string `json:"destination_amount"`
	FXRate              string `json:"fx_rate"`
	Provider            string `json:"provider"`
	CreatedAt           string `json:"created_at"`
	Status              string `json:"status"`
	Reason              string `json:"reason,omitempty"`
}

// Statuses a PaymentOutcome can report — mirrors payments-service's
// TransactionStatus values (redeclared locally rather than imported
// across module boundaries, per the established convention).
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// CorridorVolume is one row of the "volume by corridor" analytic.
type CorridorVolume struct {
	SourceCountry      string `json:"source_country"`
	DestinationCountry string `json:"destination_country"`
	TransactionCount   uint64 `json:"transaction_count"`
	TotalVolume        string `json:"total_volume"`
}

// CurrencyVolume is one row of the "volume by currency" analytic.
type CurrencyVolume struct {
	Currency         string `json:"currency"`
	TransactionCount uint64 `json:"transaction_count"`
	TotalVolume      string `json:"total_volume"`
}

// CountryVolume is one row of the "volume by country" analytic (either
// source-side or destination-side, selected by the Filter.CountryBy).
type CountryVolume struct {
	Country          string `json:"country"`
	TransactionCount uint64 `json:"transaction_count"`
	TotalVolume      string `json:"total_volume"`
}

// ProviderStats is one row of the provider performance analytic.
type ProviderStats struct {
	Provider              string  `json:"provider"`
	TotalTransactions     uint64  `json:"total_transactions"`
	CompletedTransactions uint64  `json:"completed_transactions"`
	FailedTransactions    uint64  `json:"failed_transactions"`
	FailureRate           float64 `json:"failure_rate"`
}

// TimeSeriesPoint is one row of the payment-volume time series.
type TimeSeriesPoint struct {
	PeriodStart      string `json:"period_start"`
	TransactionCount uint64 `json:"transaction_count"`
	TotalVolume      string `json:"total_volume"`
}
