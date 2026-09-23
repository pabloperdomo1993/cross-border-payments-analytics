package domain

import (
	"fmt"
	"strings"
	"time"
)

// OutboxEventStatus represents the lifecycle state of an OutboxEvent.
type OutboxEventStatus string

const (
	OutboxStatusPending   OutboxEventStatus = "pending"
	OutboxStatusPublished OutboxEventStatus = "published"
)

// OutboxEvent is a row in the transactional outbox: a business event
// that must be published to Kafka, written atomically alongside the
// Transaction it describes so the two can never disagree (a payment
// that exists without its corresponding event, or vice versa).
//
// The relay (internal/outbox) is the only thing that ever transitions
// a pending event to published — this type just carries the data.
type OutboxEvent struct {
	ID          string
	AggregateID string
	EventType   string
	Payload     string // JSON, already serialized by the application layer
	Status      OutboxEventStatus
	CreatedAt   time.Time
	PublishedAt *time.Time
}

// NewOutboxEvent validates and constructs a pending OutboxEvent.
func NewOutboxEvent(id, aggregateID, eventType, payload string, createdAt time.Time) (*OutboxEvent, error) {
	fields := map[string]string{}

	if strings.TrimSpace(id) == "" {
		fields["id"] = "must not be empty"
	}
	if strings.TrimSpace(aggregateID) == "" {
		fields["aggregate_id"] = "must not be empty"
	}
	if strings.TrimSpace(eventType) == "" {
		fields["event_type"] = "must not be empty"
	}
	if strings.TrimSpace(payload) == "" {
		fields["payload"] = "must not be empty"
	}

	if len(fields) > 0 {
		return nil, &ValidationError{Fields: fields}
	}

	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	return &OutboxEvent{
		ID:          id,
		AggregateID: aggregateID,
		EventType:   eventType,
		Payload:     payload,
		Status:      OutboxStatusPending,
		CreatedAt:   createdAt,
	}, nil
}

// MarkPublished transitions the event to published, recording when.
// It returns an error if the event isn't currently pending — an event
// is only ever published once.
func (e *OutboxEvent) MarkPublished(publishedAt time.Time) error {
	if e.Status != OutboxStatusPending {
		return fmt.Errorf("%w: outbox event %s is %s, not pending", ErrInvalidTransition, e.ID, e.Status)
	}
	e.Status = OutboxStatusPublished
	e.PublishedAt = &publishedAt
	return nil
}
