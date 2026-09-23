package kafka_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/domain"
	kafkapkg "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/kafka"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeSession struct {
	ctx context.Context

	mu     sync.Mutex
	marked []int64
}

func (s *fakeSession) Claims() map[string][]int32                                           { return nil }
func (s *fakeSession) MemberID() string                                                     { return "fake-member" }
func (s *fakeSession) GenerationID() int32                                                  { return 1 }
func (s *fakeSession) MarkOffset(topic string, partition int32, offset int64, meta string)  {}
func (s *fakeSession) Commit()                                                              {}
func (s *fakeSession) ResetOffset(topic string, partition int32, offset int64, meta string) {}
func (s *fakeSession) Context() context.Context                                             { return s.ctx }

func (s *fakeSession) MarkMessage(msg *sarama.ConsumerMessage, metadata string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marked = append(s.marked, msg.Offset)
}

func (s *fakeSession) markedOffsets() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int64, len(s.marked))
	copy(out, s.marked)
	return out
}

type fakeClaim struct {
	messages chan *sarama.ConsumerMessage
}

func newFakeClaim() *fakeClaim {
	return &fakeClaim{messages: make(chan *sarama.ConsumerMessage, 64)}
}

func (c *fakeClaim) Topic() string                            { return "payments.processed" }
func (c *fakeClaim) Partition() int32                         { return 0 }
func (c *fakeClaim) InitialOffset() int64                     { return 0 }
func (c *fakeClaim) HighWaterMarkOffset() int64               { return 0 }
func (c *fakeClaim) Messages() <-chan *sarama.ConsumerMessage { return c.messages }

type fakeInserter struct {
	mu    sync.Mutex
	calls [][]domain.PaymentOutcome
	err   error
}

func (f *fakeInserter) InsertBatch(ctx context.Context, outcomes []domain.PaymentOutcome) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	batch := append([]domain.PaymentOutcome(nil), outcomes...)
	f.calls = append(f.calls, batch)
	return nil
}

func (f *fakeInserter) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeInserter) totalRows() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, b := range f.calls {
		n += len(b)
	}
	return n
}

func sampleOutcome(id string) domain.PaymentOutcome {
	return domain.PaymentOutcome{
		TransactionID: id, SourceCountry: "CO", DestinationCountry: "US",
		SourceCurrency: "COP", DestinationCurrency: "USD",
		SourceAmount: "100.00", DestinationAmount: "50.00", FXRate: "0.01",
		Provider: "provider_a", CreatedAt: "2024-01-01T00:00:00Z", Status: domain.StatusCompleted,
	}
}

func TestConsumerHandler_FlushesOnBatchSize(t *testing.T) {
	inserter := &fakeInserter{}
	handler := kafkapkg.NewConsumerHandler(inserter, kafkapkg.BatchConfig{MaxSize: 2, MaxInterval: time.Hour}, discardLogger())

	session := &fakeSession{ctx: context.Background()}
	claim := newFakeClaim()

	done := make(chan error, 1)
	go func() { done <- handler.ConsumeClaim(session, claim) }()

	b1, _ := json.Marshal(sampleOutcome("tx-1"))
	b2, _ := json.Marshal(sampleOutcome("tx-2"))
	claim.messages <- &sarama.ConsumerMessage{Value: b1, Offset: 1}
	claim.messages <- &sarama.ConsumerMessage{Value: b2, Offset: 2}
	close(claim.messages)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim did not return")
	}

	if got := inserter.callCount(); got != 1 {
		t.Fatalf("expected exactly 1 batch insert call, got %d", got)
	}
	if got := inserter.totalRows(); got != 2 {
		t.Fatalf("expected 2 rows inserted, got %d", got)
	}
	if got := session.markedOffsets(); len(got) != 2 {
		t.Fatalf("expected 2 offsets marked, got %v", got)
	}
}

func TestConsumerHandler_FlushesOnInterval(t *testing.T) {
	inserter := &fakeInserter{}
	handler := kafkapkg.NewConsumerHandler(inserter, kafkapkg.BatchConfig{MaxSize: 1000, MaxInterval: 20 * time.Millisecond}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	session := &fakeSession{ctx: ctx}
	claim := newFakeClaim()

	done := make(chan error, 1)
	go func() { done <- handler.ConsumeClaim(session, claim) }()

	b1, _ := json.Marshal(sampleOutcome("tx-1"))
	claim.messages <- &sarama.ConsumerMessage{Value: b1, Offset: 1}

	deadline := time.After(2 * time.Second)
	for inserter.callCount() < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for interval-based flush")
		default:
		}
	}

	if got := session.markedOffsets(); len(got) != 1 {
		t.Fatalf("expected 1 offset marked after interval flush, got %v", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim did not return after cancellation")
	}
}

func TestConsumerHandler_DoesNotMarkOffsetsWhenInsertFails(t *testing.T) {
	inserter := &fakeInserter{err: errors.New("clickhouse unavailable")}
	handler := kafkapkg.NewConsumerHandler(inserter, kafkapkg.BatchConfig{MaxSize: 1, MaxInterval: time.Hour}, discardLogger())

	session := &fakeSession{ctx: context.Background()}
	claim := newFakeClaim()

	done := make(chan error, 1)
	go func() { done <- handler.ConsumeClaim(session, claim) }()

	b1, _ := json.Marshal(sampleOutcome("tx-1"))
	claim.messages <- &sarama.ConsumerMessage{Value: b1, Offset: 1}
	close(claim.messages)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim did not return")
	}

	if got := session.markedOffsets(); len(got) != 0 {
		t.Fatalf("expected no offsets marked when insert fails, got %v", got)
	}
}

func TestConsumerHandler_SkipsMalformedMessage(t *testing.T) {
	inserter := &fakeInserter{}
	handler := kafkapkg.NewConsumerHandler(inserter, kafkapkg.BatchConfig{MaxSize: 10, MaxInterval: time.Hour}, discardLogger())

	session := &fakeSession{ctx: context.Background()}
	claim := newFakeClaim()

	done := make(chan error, 1)
	go func() { done <- handler.ConsumeClaim(session, claim) }()

	claim.messages <- &sarama.ConsumerMessage{Value: []byte("not json"), Offset: 1}
	close(claim.messages)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim did not return")
	}

	if got := session.markedOffsets(); len(got) != 1 {
		t.Fatalf("expected the malformed message's offset to be marked (skipped, not redelivered forever), got %v", got)
	}
	if got := inserter.callCount(); got != 0 {
		t.Fatalf("expected no insert call for a malformed message, got %d", got)
	}
}
