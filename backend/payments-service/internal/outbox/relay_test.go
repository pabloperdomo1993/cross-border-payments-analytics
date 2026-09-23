package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/outbox"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeRepo struct {
	mu        sync.Mutex
	events    map[string]*domain.OutboxEvent
	fetchErr  error
	markErr   error
	markCalls []string
}

func newFakeRepo(events ...*domain.OutboxEvent) *fakeRepo {
	m := map[string]*domain.OutboxEvent{}
	for _, ev := range events {
		m[ev.ID] = ev
	}
	return &fakeRepo{events: m}
}

func (f *fakeRepo) FetchPendingOutboxEvents(_ context.Context, limit int) ([]*domain.OutboxEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	var pending []*domain.OutboxEvent
	for _, ev := range f.events {
		if ev.Status == domain.OutboxStatusPending {
			pending = append(pending, ev)
		}
		if len(pending) >= limit {
			break
		}
	}
	return pending, nil
}

func (f *fakeRepo) MarkOutboxEventPublished(_ context.Context, id string, publishedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markCalls = append(f.markCalls, id)
	if f.markErr != nil {
		return f.markErr
	}
	ev, ok := f.events[id]
	if !ok {
		return errors.New("not found")
	}
	return ev.MarkPublished(publishedAt)
}

func (f *fakeRepo) eventStatus(id string) domain.OutboxEventStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.events[id].Status
}

type fakePublisher struct {
	mu    sync.Mutex
	calls []publishCall
	err   error
}

type publishCall struct {
	topic string
	key   string
	value string
}

func (p *fakePublisher) Publish(_ context.Context, topic string, key, value []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, publishCall{topic: topic, key: string(key), value: string(value)})
	return p.err
}

func (p *fakePublisher) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func newEvent(t *testing.T, id, aggregateID string) *domain.OutboxEvent {
	t.Helper()
	ev, err := domain.NewOutboxEvent(id, aggregateID, "payment.created", `{"id":"`+aggregateID+`"}`, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error building test event: %v", err)
	}
	return ev
}

func TestRelay_PublishesPendingEventAndMarksIt(t *testing.T) {
	ev := newEvent(t, "evt-1", "tx-1")
	repo := newFakeRepo(ev)
	pub := &fakePublisher{}
	relay := outbox.NewRelay(repo, pub, outbox.Config{Topic: "payments.created", PollInterval: 5 * time.Millisecond, BatchSize: 10}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go relay.Run(ctx)

	deadline := time.After(2 * time.Second)
	for pub.callCount() < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the relay to publish the pending event")
		default:
		}
	}
	cancel()

	if got := pub.callCount(); got != 1 {
		t.Fatalf("expected exactly 1 publish call, got %d", got)
	}
	pub.mu.Lock()
	call := pub.calls[0]
	pub.mu.Unlock()
	if call.topic != "payments.created" {
		t.Errorf("expected topic %q, got %q", "payments.created", call.topic)
	}
	if call.key != "tx-1" {
		t.Errorf("expected key %q (aggregate id), got %q", "tx-1", call.key)
	}

	if got := repo.eventStatus("evt-1"); got != domain.OutboxStatusPublished {
		t.Errorf("expected event to be marked published, got status %q", got)
	}
}

func TestRelay_PublishFailureLeavesEventPending(t *testing.T) {
	ev := newEvent(t, "evt-1", "tx-1")
	repo := newFakeRepo(ev)
	pub := &fakePublisher{err: errors.New("kafka unavailable")}
	relay := outbox.NewRelay(repo, pub, outbox.Config{Topic: "payments.created", PollInterval: 5 * time.Millisecond, BatchSize: 10}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	go relay.Run(ctx)

	deadline := time.After(1 * time.Second)
	for pub.callCount() < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for a publish attempt")
		default:
		}
	}
	cancel()

	if got := repo.eventStatus("evt-1"); got != domain.OutboxStatusPending {
		t.Errorf("expected event to remain pending after a publish failure, got status %q", got)
	}
	if len(repo.markCalls) != 0 {
		t.Errorf("expected MarkOutboxEventPublished not to be called after a publish failure, got %v", repo.markCalls)
	}
}

func TestRelay_StopsOnContextCancellation(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{}
	relay := outbox.NewRelay(repo, pub, outbox.Config{Topic: "payments.created", PollInterval: time.Millisecond, BatchSize: 10}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		relay.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
