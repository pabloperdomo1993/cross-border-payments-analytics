// Package outbox implements the publishing half of the transactional
// outbox pattern: a relay that polls for pending events (written
// atomically with their payment by internal/repository/mariadb) and
// publishes them to Kafka.
//
// Delivery semantics: at-least-once, not exactly-once. An event is only
// marked published after a successful Kafka publish, so a transient
// Kafka failure never loses an event — it just stays pending and is
// retried on the next poll. The one accepted trade-off: if the relay
// crashes in the narrow window after a successful publish but before
// the local "mark published" write, the event is republished on
// restart, producing a duplicate payments.created message. Building
// full exactly-once delivery (e.g. a two-phase commit with Kafka) is
// out of scope here — this is a documented limitation, not an oversight.
package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/metrics"
)

// Repository is the subset of repository.TransactionRepository the
// relay depends on, declared here so it's testable with a fake.
type Repository interface {
	FetchPendingOutboxEvents(ctx context.Context, limit int) ([]*domain.OutboxEvent, error)
	MarkOutboxEventPublished(ctx context.Context, id string, publishedAt time.Time) error
}

// Publisher is the subset of kafka.Producer the relay depends on.
type Publisher interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
}

// Config controls the relay's polling behavior.
type Config struct {
	Topic        string
	PollInterval time.Duration
	BatchSize    int
}

// Relay polls for pending outbox events and publishes them to Kafka.
type Relay struct {
	repo      Repository
	publisher Publisher
	cfg       Config
	logger    *slog.Logger
}

// NewRelay builds a Relay.
func NewRelay(repo Repository, publisher Publisher, cfg Config, logger *slog.Logger) *Relay {
	return &Relay{repo: repo, publisher: publisher, cfg: cfg, logger: logger}
}

// Run polls on a ticker until ctx is cancelled. It never returns an
// error itself — publish failures are logged and left for the next
// poll, which is the entire point of the outbox (a transient Kafka
// outage must not lose an event, or block the caller that created it).
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.tick(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (r *Relay) tick(ctx context.Context) {
	events, err := r.repo.FetchPendingOutboxEvents(ctx, r.cfg.BatchSize)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to fetch pending outbox events", slog.String("error", err.Error()))
		return
	}

	for _, ev := range events {
		r.publishOne(ctx, ev)
	}
}

func (r *Relay) publishOne(ctx context.Context, ev *domain.OutboxEvent) {
	if err := r.publisher.Publish(ctx, r.cfg.Topic, []byte(ev.AggregateID), []byte(ev.Payload)); err != nil {
		metrics.OutboxEventsPublishErrorsTotal.Inc()
		r.logger.WarnContext(ctx, "failed to publish outbox event; will retry on next poll",
			slog.String("event_id", ev.ID), slog.String("error", err.Error()))
		return
	}

	publishedAt := time.Now().UTC()
	if err := r.repo.MarkOutboxEventPublished(ctx, ev.ID, publishedAt); err != nil {
		// Published successfully but failed to record it locally: see
		// the package doc for why this is the one documented duplicate
		// -delivery trade-off, not a bug.
		metrics.OutboxEventsPublishErrorsTotal.Inc()
		r.logger.ErrorContext(ctx, "published event but failed to mark it published; it will be re-published on the next poll",
			slog.String("event_id", ev.ID), slog.String("error", err.Error()))
		return
	}

	metrics.OutboxEventsPublishedTotal.Inc()
	r.logger.InfoContext(ctx, "outbox event published",
		slog.String("event_id", ev.ID), slog.String("aggregate_id", ev.AggregateID))
}
