// Package domain contains payment-processor's own small event model. It
// is intentionally independent from payments-service's domain package:
// Go internal packages aren't importable across modules anyway, and in
// a microservices split each service owning its own wire-level types is
// the expected boundary, not accidental duplication.
package domain

import "fmt"

// PaymentEvent is the payload payment-processor consumes from the
// payments.created topic. Amount and FXRate are decimal strings (not
// float64) to avoid binary floating-point precision loss, matching how
// payments-service represents them on the wire.
type PaymentEvent struct {
	TransactionID       string `json:"id"`
	SourceCountry       string `json:"source_country"`
	DestinationCountry  string `json:"destination_country"`
	SourceCurrency      string `json:"source_currency"`
	DestinationCurrency string `json:"destination_currency"`
	Amount              string `json:"amount"`
	FXRate              string `json:"fx_rate"`
	Provider            string `json:"provider"`
}

// Outcome is the payload produced back to Kafka once a PaymentEvent has
// been handled, either to payments.processed (Status == "completed") or
// carried into payments.dlq alongside the failure reason.
type Outcome struct {
	TransactionID string `json:"id"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
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
