// Package domain contains payment-processor's own small event model. It
// is intentionally independent from payments-service's domain package:
// Go internal packages aren't importable across modules anyway, and in
// a microservices split each service owning its own wire-level types is
// the expected boundary, not accidental duplication.
package domain

import "fmt"

// PaymentEvent is the payload payment-processor consumes from the
// payments.created topic. Monetary fields and FXRate are decimal
// strings (not float64) to avoid binary floating-point precision loss,
// matching how payments-service represents them on the wire.
type PaymentEvent struct {
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
}

// Outcome is the payload produced back to Kafka once a PaymentEvent has
// been handled, either to payments.processed (Status == "completed") or
// carried into payments.dlq alongside the failure reason.
//
// It carries the full payment dimensions (not just id+status) because
// analytics-service consumes payments.processed directly and needs
// enough data to aggregate by corridor/currency/provider/time without a
// second lookup — this is the Kafka contract's one real consumer today,
// so the contract is shaped for it.
type Outcome struct {
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

// Statuses an Outcome can report.
const (
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

func (e PaymentEvent) String() string {
	return fmt.Sprintf("PaymentEvent{id=%s, %s/%s -> %s/%s, provider=%s}",
		e.TransactionID, e.SourceCountry, e.SourceCurrency, e.DestinationCountry, e.DestinationCurrency, e.Provider)
}
