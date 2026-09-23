package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewOutboxEvent_Valid(t *testing.T) {
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ev, err := NewOutboxEvent("evt-1", "tx-1", "payment.created", `{"id":"tx-1"}`, createdAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Status != OutboxStatusPending {
		t.Errorf("expected status %q, got %q", OutboxStatusPending, ev.Status)
	}
	if !ev.CreatedAt.Equal(createdAt) {
		t.Errorf("expected CreatedAt %v, got %v", createdAt, ev.CreatedAt)
	}
	if ev.PublishedAt != nil {
		t.Errorf("expected PublishedAt to be nil for a pending event")
	}
}

func TestNewOutboxEvent_DefaultsCreatedAt(t *testing.T) {
	before := time.Now().UTC()
	ev, err := NewOutboxEvent("evt-1", "tx-1", "payment.created", `{}`, time.Time{})
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.CreatedAt.Before(before) || ev.CreatedAt.After(after) {
		t.Errorf("expected CreatedAt to default to now(), got %v", ev.CreatedAt)
	}
}

func TestNewOutboxEvent_Validation(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		aggregateID string
		eventType   string
		payload     string
		wantField   string
	}{
		{"empty id", "", "tx-1", "payment.created", "{}", "id"},
		{"empty aggregate id", "evt-1", "", "payment.created", "{}", "aggregate_id"},
		{"empty event type", "evt-1", "tx-1", "", "{}", "event_type"},
		{"empty payload", "evt-1", "tx-1", "payment.created", "", "payload"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOutboxEvent(tt.id, tt.aggregateID, tt.eventType, tt.payload, time.Time{})
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			verr, ok := AsValidationError(err)
			if !ok {
				t.Fatalf("expected *ValidationError, got %T: %v", err, err)
			}
			if _, ok := verr.Fields[tt.wantField]; !ok {
				t.Errorf("expected validation error on field %q, got fields %v", tt.wantField, verr.Fields)
			}
		})
	}
}

func TestOutboxEvent_MarkPublished(t *testing.T) {
	ev, err := NewOutboxEvent("evt-1", "tx-1", "payment.created", `{}`, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	publishedAt := time.Now().UTC()
	if err := ev.MarkPublished(publishedAt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Status != OutboxStatusPublished {
		t.Errorf("expected status %q, got %q", OutboxStatusPublished, ev.Status)
	}
	if ev.PublishedAt == nil || !ev.PublishedAt.Equal(publishedAt) {
		t.Errorf("expected PublishedAt %v, got %v", publishedAt, ev.PublishedAt)
	}
}

func TestOutboxEvent_MarkPublished_AlreadyPublished(t *testing.T) {
	ev, err := NewOutboxEvent("evt-1", "tx-1", "payment.created", `{}`, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ev.MarkPublished(time.Now().UTC()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = ev.MarkPublished(time.Now().UTC())
	if err == nil {
		t.Fatal("expected error marking an already-published event as published again")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}
