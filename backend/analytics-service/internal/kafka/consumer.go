// Package kafka wires analytics-service to Kafka: a consumer that
// batches payments.processed (and payments.dlq) messages and writes
// them to ClickHouse in bulk, matching ClickHouse's own recommended
// ingestion pattern.
package kafka

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/IBM/sarama"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/domain"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/metrics"
)

// Inserter is the subset of repository.AnalyticsRepository the consumer
// depends on, declared here so it can be tested with a fake.
type Inserter interface {
	InsertBatch(ctx context.Context, outcomes []domain.PaymentOutcome) error
}

// BatchConfig controls how aggressively the consumer batches messages
// before writing to ClickHouse.
type BatchConfig struct {
	MaxSize     int
	MaxInterval time.Duration
}

// ConsumerHandler implements sarama.ConsumerGroupHandler. Unlike
// payment-processor's handler, correctness here doesn't hinge on
// strict per-partition ordering — these are additive analytical facts,
// not a financial ledger — so messages are simply accumulated into a
// batch and flushed periodically or once the batch is full, then the
// whole batch's offsets are marked together. If ClickHouse insertion
// fails, none of the batch's offsets are marked, so every message in it
// is safely redelivered (at-least-once; duplicate analytical rows on
// redelivery are an accepted trade-off documented in the architecture
// notes, since exact analytics reconciliation is out of scope here).
type ConsumerHandler struct {
	inserter Inserter
	batch    BatchConfig
	logger   *slog.Logger
}

// NewConsumerHandler builds a ConsumerHandler.
func NewConsumerHandler(inserter Inserter, batch BatchConfig, logger *slog.Logger) *ConsumerHandler {
	return &ConsumerHandler{inserter: inserter, batch: batch, logger: logger}
}

// Setup implements sarama.ConsumerGroupHandler.
func (h *ConsumerHandler) Setup(sarama.ConsumerGroupSession) error { return nil }

// Cleanup implements sarama.ConsumerGroupHandler.
func (h *ConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

// ConsumeClaim implements sarama.ConsumerGroupHandler.
func (h *ConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	ticker := time.NewTicker(h.batch.MaxInterval)
	defer ticker.Stop()

	var outcomes []domain.PaymentOutcome
	var pending []*sarama.ConsumerMessage

	flush := func() {
		if len(outcomes) == 0 {
			return
		}
		metrics.ClickHouseInsertBatchesTotal.Inc()
		if err := h.inserter.InsertBatch(session.Context(), outcomes); err != nil {
			metrics.ClickHouseInsertErrorsTotal.Inc()
			h.logger.ErrorContext(session.Context(), "failed to insert analytics batch; messages will be redelivered",
				slog.Int("batch_size", len(outcomes)),
				slog.String("error", err.Error()),
			)
			outcomes = nil
			pending = nil
			return
		}
		for _, msg := range pending {
			session.MarkMessage(msg, "")
		}
		h.logger.InfoContext(session.Context(), "analytics batch inserted", slog.Int("batch_size", len(outcomes)))
		outcomes = nil
		pending = nil
	}

	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				flush()
				return nil
			}

			metrics.KafkaMessagesConsumedTotal.Inc()

			var outcome domain.PaymentOutcome
			if err := json.Unmarshal(msg.Value, &outcome); err != nil {
				h.logger.WarnContext(session.Context(), "skipping malformed analytics message",
					slog.String("error", err.Error()),
					slog.Int("partition", int(msg.Partition)),
					slog.Int64("offset", msg.Offset),
				)
				session.MarkMessage(msg, "")
				continue
			}

			outcomes = append(outcomes, outcome)
			pending = append(pending, msg)
			if len(outcomes) >= h.batch.MaxSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-session.Context().Done():
			return nil
		}
	}
}
