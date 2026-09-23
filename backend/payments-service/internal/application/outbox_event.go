package application

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

// EventTypePaymentCreated is the outbox event type published to the
// payments.created Kafka topic when a Transaction is created.
const EventTypePaymentCreated = "payment.created"

// paymentCreatedPayload is the exact wire shape payment-processor
// expects on payments.created (see that service's
// internal/domain.PaymentEvent) — kept here, not shared, since the two
// services are independent Go modules with no shared library by
// design; this is the one place payments-service needs to know that
// shape.
type paymentCreatedPayload struct {
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

// newPaymentCreatedOutboxEvent builds the OutboxEvent to be written
// atomically alongside tx, carrying the exact JSON payments.created
// will publish.
func newPaymentCreatedOutboxEvent(eventID string, tx *domain.Transaction) (*domain.OutboxEvent, error) {
	payload := paymentCreatedPayload{
		TransactionID:       string(tx.ID),
		SourceCountry:       string(tx.SourceCountry),
		DestinationCountry:  string(tx.DestinationCountry),
		SourceCurrency:      string(tx.SourceCurrency),
		DestinationCurrency: string(tx.DestinationCurrency),
		SourceAmount:        tx.SourceAmount.String(),
		DestinationAmount:   tx.DestinationAmount.String(),
		FXRate:              tx.FXRate.String(),
		Provider:            string(tx.Provider),
		CreatedAt:           tx.CreatedAt.Format(time.RFC3339),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payment.created payload: %w", err)
	}

	return domain.NewOutboxEvent(eventID, string(tx.ID), EventTypePaymentCreated, string(payloadJSON), tx.CreatedAt)
}
