package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/IBM/sarama"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/domain"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/metrics"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/processing"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/workerpool"
)

// Publisher is the subset of Producer the consumer handler depends on,
// declared here so tests can substitute a fake without a real broker.
type Publisher interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
}

// Pool is the subset of workerpool.Pool the consumer handler depends on.
type Pool interface {
	Execute(ctx context.Context, task workerpool.Task) error
}

// HandlerConfig carries the consumer handler's tunables.
type HandlerConfig struct {
	TopicPaymentsProcessed string
	TopicPaymentsDLQ       string
	ProcessingTimeout      time.Duration
	MaxRetries             int
	RetryBackoff           time.Duration
}

// ConsumerHandler implements sarama.ConsumerGroupHandler.
//
// Offset/partition strategy: Sarama invokes ConsumeClaim once per
// partition assigned to this consumer, each in its own goroutine, with
// messages for that partition delivered in order on claim.Messages().
// handleMessage submits each message to the shared bounded pool and
// BLOCKS until that job completes before this partition's loop reads
// its next message or marks any offset. This guarantees: (1) never more
// than one in-flight job per partition, so a later message can't be
// marked complete while an earlier one from the same partition is still
// failed/in-flight; (2) offsets are always marked in order, per
// partition. Concurrency comes from *different* partitions' ConsumeClaim
// goroutines each holding one job in the shared pool at the same time —
// true parallelism is therefore min(WORKER_COUNT, assigned partitions),
// which is a fundamental property of safely-ordered Kafka processing,
// not a limitation specific to this implementation.
type ConsumerHandler struct {
	pool      Pool
	processor processing.Processor
	publisher Publisher
	cfg       HandlerConfig
	logger    *slog.Logger
}

// NewConsumerHandler builds a ConsumerHandler.
func NewConsumerHandler(pool Pool, processor processing.Processor, publisher Publisher, cfg HandlerConfig, logger *slog.Logger) *ConsumerHandler {
	return &ConsumerHandler{pool: pool, processor: processor, publisher: publisher, cfg: cfg, logger: logger}
}

// Setup implements sarama.ConsumerGroupHandler.
func (h *ConsumerHandler) Setup(sarama.ConsumerGroupSession) error { return nil }

// Cleanup implements sarama.ConsumerGroupHandler.
func (h *ConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

// ConsumeClaim implements sarama.ConsumerGroupHandler. It is called once
// per assigned partition, in its own goroutine, by Sarama.
func (h *ConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			h.handleMessage(session, msg)
		case <-session.Context().Done():
			return nil
		}
	}
}

// handleMessage submits msg's processing as one job to the shared pool
// and waits for it to finish before returning, which is what keeps this
// partition's processing strictly sequential (see ConsumerHandler docs).
func (h *ConsumerHandler) handleMessage(session sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) {
	ctx := session.Context()

	err := h.pool.Execute(ctx, func(taskCtx context.Context) error {
		timeoutCtx, cancel := context.WithTimeout(taskCtx, h.cfg.ProcessingTimeout)
		defer cancel()
		return h.process(timeoutCtx, msg)
	})
	if err != nil {
		// Execute itself didn't complete (pool closed or ctx cancelled
		// before/while running) rather than the payment failing —
		// almost always because we're shutting down. Do not mark the
		// offset: at-least-once semantics mean this message will be
		// redelivered and retried by whichever consumer next owns this
		// partition, rather than being silently lost.
		h.logger.WarnContext(ctx, "message not marked; will be redelivered",
			slog.String("error", err.Error()),
			slog.Int("partition", int(msg.Partition)),
			slog.Int64("offset", msg.Offset),
		)
		return
	}

	session.MarkMessage(msg, "")
}

// process decodes, validates/processes, and publishes the outcome for a
// single message, retrying the validate+publish step a bounded number
// of times on retryable failures, and dead-lettering on any failure
// that survives those retries (or is non-retryable outright). It never
// returns a "the payment failed" error — a payment failure is a valid,
// fully-handled outcome (dead-lettered); only an inability to record
// that outcome at all (DLQ publish itself failing) is propagated, so
// the caller knows not to mark the offset.
func (h *ConsumerHandler) process(ctx context.Context, msg *sarama.ConsumerMessage) error {
	start := time.Now()
	defer func() { metrics.ProcessingDuration.Observe(time.Since(start).Seconds()) }()

	var event domain.PaymentEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		metrics.ProcessingFailedTotal.WithLabelValues("false").Inc()
		metrics.ProcessingTotal.WithLabelValues(domain.StatusFailed).Inc()
		return h.deadLetter(ctx, msg, event, processing.NonRetryable(fmt.Errorf("%w: %v", processing.ErrMalformedEvent, err)))
	}

	maxAttempts := h.cfg.MaxRetries + 1
	var outcome domain.Outcome
	procErr := processing.Retry(ctx, maxAttempts, h.cfg.RetryBackoff, func() error {
		var err error
		outcome, err = h.processor.Process(ctx, event)
		if err != nil {
			return err
		}

		payload, err := json.Marshal(outcome)
		if err != nil {
			return processing.NonRetryable(fmt.Errorf("marshal outcome: %w", err))
		}
		return h.publisher.Publish(ctx, h.cfg.TopicPaymentsProcessed, []byte(event.TransactionID), payload)
	})

	if procErr != nil {
		metrics.ProcessingFailedTotal.WithLabelValues(strconv.FormatBool(processing.IsRetryable(procErr))).Inc()
		metrics.ProcessingTotal.WithLabelValues(domain.StatusFailed).Inc()
		return h.deadLetter(ctx, msg, event, procErr)
	}

	metrics.ProcessingTotal.WithLabelValues(domain.StatusCompleted).Inc()
	return nil
}

// deadLetter publishes a failure outcome to the DLQ topic. Only if that
// publish itself fails is an error returned (so the offset won't be
// marked and the message is redelivered) — a classified processing
// failure that we successfully recorded to the DLQ is never treated as
// an unhandled/dropped message.
func (h *ConsumerHandler) deadLetter(ctx context.Context, msg *sarama.ConsumerMessage, event domain.PaymentEvent, cause error) error {
	outcome := domain.Outcome{
		TransactionID:       event.TransactionID,
		SourceCountry:       event.SourceCountry,
		DestinationCountry:  event.DestinationCountry,
		SourceCurrency:      event.SourceCurrency,
		DestinationCurrency: event.DestinationCurrency,
		SourceAmount:        event.SourceAmount,
		DestinationAmount:   event.DestinationAmount,
		FXRate:              event.FXRate,
		Provider:            event.Provider,
		CreatedAt:           event.CreatedAt,
		Status:              domain.StatusFailed,
		Reason:              cause.Error(),
	}
	payload, err := json.Marshal(outcome)
	if err != nil {
		payload = msg.Value
	}

	if err := h.publisher.Publish(ctx, h.cfg.TopicPaymentsDLQ, msg.Key, payload); err != nil {
		h.logger.ErrorContext(ctx, "failed to publish to DLQ; message will be redelivered",
			slog.String("cause", cause.Error()),
			slog.String("dlq_error", err.Error()),
			slog.Int("partition", int(msg.Partition)),
			slog.Int64("offset", msg.Offset),
		)
		return fmt.Errorf("dead-letter publish failed: %w", err)
	}

	h.logger.WarnContext(ctx, "payment dead-lettered",
		slog.String("cause", cause.Error()),
		slog.Int("partition", int(msg.Partition)),
		slog.Int64("offset", msg.Offset),
	)
	return nil
}

// RunConsumerGroup joins the consumer group and consumes topics with
// handler until ctx is cancelled. Sarama's Consume call returns after
// every rebalance, so this loops it — the standard usage pattern for
// sarama.ConsumerGroup.
func RunConsumerGroup(ctx context.Context, brokers []string, groupID string, topics []string, handler sarama.ConsumerGroupHandler, logger *slog.Logger) error {
	cfg := sarama.NewConfig()
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	cfg.Consumer.Return.Errors = true

	group, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return fmt.Errorf("create consumer group: %w", err)
	}
	defer group.Close()

	go func() {
		for consumeErr := range group.Errors() {
			logger.Error("consumer group error", slog.String("error", consumeErr.Error()))
		}
	}()

	for {
		if err := group.Consume(ctx, topics, handler); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}
			return fmt.Errorf("consume: %w", err)
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}
