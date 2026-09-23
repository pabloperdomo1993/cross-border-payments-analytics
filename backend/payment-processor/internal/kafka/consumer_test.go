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

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/domain"
	kafkapkg "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/kafka"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/processing"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/workerpool"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeSession is a minimal sarama.ConsumerGroupSession implementing
// only what ConsumerHandler actually uses (Context, MarkMessage), with
// MarkMessage calls recorded for assertions.
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

// fakeClaim is a minimal sarama.ConsumerGroupClaim backed by a plain
// channel the test feeds messages into.
type fakeClaim struct {
	messages chan *sarama.ConsumerMessage
}

func newFakeClaim() *fakeClaim {
	return &fakeClaim{messages: make(chan *sarama.ConsumerMessage, 16)}
}

func (c *fakeClaim) Topic() string                            { return "payments.created" }
func (c *fakeClaim) Partition() int32                         { return 0 }
func (c *fakeClaim) InitialOffset() int64                     { return 0 }
func (c *fakeClaim) HighWaterMarkOffset() int64               { return 0 }
func (c *fakeClaim) Messages() <-chan *sarama.ConsumerMessage { return c.messages }

// fakePublisher records every publish call and lets tests control
// success/failure per call (e.g. to simulate a transient publish
// failure that recovers on retry).
type fakePublisher struct {
	mu    sync.Mutex
	calls []publishCall
	fail  func(topic string) error // return non-nil to fail that call
}

type publishCall struct {
	topic string
	value []byte
}

func (p *fakePublisher) Publish(ctx context.Context, topic string, key, value []byte) error {
	p.mu.Lock()
	p.calls = append(p.calls, publishCall{topic: topic, value: append([]byte(nil), value...)})
	fail := p.fail
	p.mu.Unlock()

	if fail != nil {
		if err := fail(topic); err != nil {
			return err
		}
	}
	return nil
}

func (p *fakePublisher) callsTo(topic string) []publishCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []publishCall
	for _, c := range p.calls {
		if c.topic == topic {
			out = append(out, c)
		}
	}
	return out
}

// recordingProcessor records the order in which Process is invoked and
// lets a test block a specific call until released, to prove per
// -partition sequencing.
type recordingProcessor struct {
	mu      sync.Mutex
	started []string
	ended   []string

	blockFirst bool
	release    chan struct{}
}

func (p *recordingProcessor) Process(ctx context.Context, event domain.PaymentEvent) (domain.Outcome, error) {
	p.mu.Lock()
	p.started = append(p.started, event.TransactionID)
	shouldBlock := p.blockFirst && len(p.started) == 1
	p.mu.Unlock()

	if shouldBlock {
		<-p.release
	}

	p.mu.Lock()
	p.ended = append(p.ended, event.TransactionID)
	p.mu.Unlock()

	return domain.Outcome{TransactionID: event.TransactionID, Status: domain.StatusCompleted}, nil
}

func newTestHandler(pool kafkapkg.Pool, processor processing.Processor, publisher kafkapkg.Publisher) *kafkapkg.ConsumerHandler {
	return kafkapkg.NewConsumerHandler(pool, processor, publisher, kafkapkg.HandlerConfig{
		TopicPaymentsProcessed: "payments.processed",
		TopicPaymentsDLQ:       "payments.dlq",
		ProcessingTimeout:      time.Second,
		MaxRetries:             2,
		RetryBackoff:           time.Millisecond,
	}, discardLogger())
}

func encode(t *testing.T, event domain.PaymentEvent) []byte {
	t.Helper()
	b, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to encode test event: %v", err)
	}
	return b
}

// TestConsumerHandler_MarksOffsetOnlyAfterSuccessfulProcessing is the
// core Kafka-semantics test (section 8/11-G): the offset must not be
// marked before processing (here, publishing the outcome) succeeds.
func TestConsumerHandler_MarksOffsetOnlyAfterSuccessfulProcessing(t *testing.T) {
	pool := workerpool.New(2, 4)
	defer pool.Shutdown()

	publisher := &fakePublisher{}
	handler := newTestHandler(pool, processing.NewDefaultProcessor(), publisher)

	session := &fakeSession{ctx: context.Background()}
	claim := newFakeClaim()

	event := domain.PaymentEvent{
		TransactionID: "tx-1", SourceCountry: "CO", DestinationCountry: "US",
		SourceCurrency: "COP", DestinationCurrency: "USD",
		Amount: "100.00", FXRate: "0.01", Provider: "provider_a",
	}
	msg := &sarama.ConsumerMessage{Value: encode(t, event), Offset: 42, Partition: 0}

	done := make(chan error, 1)
	go func() { done <- handler.ConsumeClaim(session, claim) }()

	claim.messages <- msg
	close(claim.messages)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim did not return")
	}

	if got := session.markedOffsets(); len(got) != 1 || got[0] != 42 {
		t.Fatalf("expected offset 42 to be marked exactly once, got %v", got)
	}
	if calls := publisher.callsTo("payments.processed"); len(calls) != 1 {
		t.Fatalf("expected exactly 1 publish to payments.processed, got %d", len(calls))
	}
}

// TestConsumerHandler_NeverMarksOffsetWhenDLQPublishFails proves a
// message is never considered handled (offset marked) if we couldn't
// even record its failure — never silently discard.
func TestConsumerHandler_NeverMarksOffsetWhenDLQPublishFails(t *testing.T) {
	pool := workerpool.New(1, 1)
	defer pool.Shutdown()

	publisher := &fakePublisher{fail: func(topic string) error {
		return errors.New("broker unreachable")
	}}
	handler := newTestHandler(pool, processing.NewDefaultProcessor(), publisher)

	session := &fakeSession{ctx: context.Background()}
	claim := newFakeClaim()

	// Malformed event -> non-retryable -> straight to DLQ, but every
	// publish (including the DLQ one) fails in this test.
	msg := &sarama.ConsumerMessage{Value: []byte("not json"), Offset: 7, Partition: 0}

	done := make(chan error, 1)
	go func() { done <- handler.ConsumeClaim(session, claim) }()
	claim.messages <- msg
	close(claim.messages)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim did not return")
	}

	if got := session.markedOffsets(); len(got) != 0 {
		t.Fatalf("expected no offsets marked when DLQ publish fails, got %v", got)
	}
}

// TestConsumerHandler_SequentialWithinPartition proves the core offset
// safety property: two messages on the SAME partition are never
// in-flight at once — the second's processing does not start until the
// first's has ended — even though the pool has more than one worker
// available.
func TestConsumerHandler_SequentialWithinPartition(t *testing.T) {
	pool := workerpool.New(4, 8) // plenty of concurrency available
	defer pool.Shutdown()

	processor := &recordingProcessor{blockFirst: true, release: make(chan struct{})}
	publisher := &fakePublisher{}
	handler := newTestHandler(pool, processor, publisher)

	session := &fakeSession{ctx: context.Background()}
	claim := newFakeClaim()

	event1 := domain.PaymentEvent{TransactionID: "tx-1", SourceCountry: "CO", DestinationCountry: "US", SourceCurrency: "COP", DestinationCurrency: "USD", Amount: "1.00", FXRate: "0.01", Provider: "p"}
	event2 := domain.PaymentEvent{TransactionID: "tx-2", SourceCountry: "CO", DestinationCountry: "US", SourceCurrency: "COP", DestinationCurrency: "USD", Amount: "1.00", FXRate: "0.01", Provider: "p"}

	done := make(chan error, 1)
	go func() { done <- handler.ConsumeClaim(session, claim) }()

	claim.messages <- &sarama.ConsumerMessage{Value: encode(t, event1), Offset: 1, Partition: 0}
	claim.messages <- &sarama.ConsumerMessage{Value: encode(t, event2), Offset: 2, Partition: 0}

	// Give the claim loop a moment to have started tx-1 and be blocked
	// on it, then confirm tx-2 has NOT started yet.
	deadline := time.After(2 * time.Second)
	for {
		processor.mu.Lock()
		started := len(processor.started)
		processor.mu.Unlock()
		if started >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for tx-1 to start")
		default:
		}
	}

	processor.mu.Lock()
	startedSoFar := len(processor.started)
	processor.mu.Unlock()
	if startedSoFar != 1 {
		t.Fatalf("expected exactly 1 message to have started processing, got %d", startedSoFar)
	}

	close(processor.release)
	close(claim.messages)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim did not return")
	}

	processor.mu.Lock()
	defer processor.mu.Unlock()
	if len(processor.started) != 2 || processor.started[0] != "tx-1" || processor.started[1] != "tx-2" {
		t.Fatalf("expected tx-1 then tx-2 to start in order, got %v", processor.started)
	}
	if len(processor.ended) != 2 || processor.ended[0] != "tx-1" {
		t.Fatalf("expected tx-1 to finish before tx-2 started, got ended order %v", processor.ended)
	}

	if got := session.markedOffsets(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("expected offsets marked in order [1 2], got %v", got)
	}
}

// TestConsumerHandler_ShutdownDoesNotMarkInFlightMessage verifies that
// when the session context is cancelled mid-processing, the in-flight
// message's offset is not marked (so it's safely redelivered).
func TestConsumerHandler_ShutdownDoesNotMarkInFlightMessage(t *testing.T) {
	pool := workerpool.New(1, 1)
	defer pool.Shutdown()

	processingStarted := make(chan struct{})
	blockingProcessor := processingFunc(func(ctx context.Context, event domain.PaymentEvent) (domain.Outcome, error) {
		close(processingStarted)
		<-ctx.Done()
		return domain.Outcome{}, processing.Retryable(ctx.Err())
	})
	publisher := &fakePublisher{}
	handler := newTestHandler(pool, blockingProcessor, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	session := &fakeSession{ctx: ctx}
	claim := newFakeClaim()

	event := domain.PaymentEvent{TransactionID: "tx-1", SourceCountry: "CO", DestinationCountry: "US", SourceCurrency: "COP", DestinationCurrency: "USD", Amount: "1.00", FXRate: "0.01", Provider: "p"}
	claim.messages <- &sarama.ConsumerMessage{Value: encode(t, event), Offset: 1, Partition: 0}

	done := make(chan error, 1)
	go func() { done <- handler.ConsumeClaim(session, claim) }()

	// Deterministically wait for processing to actually start before
	// cancelling, like a real shutdown signal arriving mid-flight.
	select {
	case <-processingStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for processing to start")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim did not return after context cancellation")
	}

	if got := session.markedOffsets(); len(got) != 0 {
		t.Fatalf("expected no offsets marked for an in-flight message aborted by shutdown, got %v", got)
	}
}

type processingFunc func(ctx context.Context, event domain.PaymentEvent) (domain.Outcome, error)

func (f processingFunc) Process(ctx context.Context, event domain.PaymentEvent) (domain.Outcome, error) {
	return f(ctx, event)
}
