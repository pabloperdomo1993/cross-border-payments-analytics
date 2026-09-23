//go:build integration

// A single, focused integration test against a REAL Kafka broker,
// covering the critical flow: a real payments.created message is
// consumed by the actual ConsumerHandler (bounded worker pool,
// production Processor, everything wired exactly as main.go wires it),
// and a real payments.processed message comes out the other side. Run
// with:
//
//	docker compose up -d kafka kafka-init
//	go test -tags=integration ./internal/kafka/...
package kafka_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/domain"
	kafkapkg "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/kafka"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/processing"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/workerpool"
)

func brokers(t *testing.T) []string {
	t.Helper()
	// :29092 is Kafka's EXTERNAL listener (see docker-compose.yml) —
	// the one advertised for host-based clients like this test; :9092
	// is only reachable from other containers.
	raw := envOr("TEST_KAFKA_BROKERS", "localhost:29092")

	cfg := sarama.NewConfig()
	cfg.Net.DialTimeout = 3 * time.Second
	client, err := sarama.NewClient(strings.Split(raw, ","), cfg)
	if err != nil {
		t.Skipf("skipping: cannot reach test Kafka at %s (run `docker compose up -d kafka kafka-init` first): %v", raw, err)
	}
	client.Close()

	return strings.Split(raw, ",")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// collectorHandler is a minimal sarama.ConsumerGroupHandler that just
// forwards every message's value to a channel, used to observe
// payments.processed without depending on analytics-service's own
// consumer code.
type collectorHandler struct {
	out chan []byte
}

func (h *collectorHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *collectorHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }
func (h *collectorHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			h.out <- msg.Value
			session.MarkMessage(msg, "")
		case <-session.Context().Done():
			return nil
		}
	}
}

func TestIntegration_PaymentsCreated_ReachesPaymentProcessor_And_PublishesProcessed(t *testing.T) {
	brokerAddrs := brokers(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1. Start a collector consumer group on payments.processed BEFORE
	// producing/consuming payments.created, so we don't miss the
	// resulting message. Fresh random group ID => OffsetNewest applies
	// (no prior committed offset to resume from).
	collector := &collectorHandler{out: make(chan []byte, 100)}
	collectorGroupID := fmt.Sprintf("it-collector-%d", time.Now().UnixNano())
	go func() {
		_ = kafkapkg.RunConsumerGroup(ctx, brokerAddrs, collectorGroupID, []string{"payments.processed"}, collector, discardLogger())
	}()

	// 2. Start the REAL ConsumerHandler on payments.created, wired the
	// same way cmd/processor/main.go wires it: bounded pool, production
	// Processor, real producer for the outcome.
	producer, err := kafkapkg.NewProducer(brokerAddrs)
	if err != nil {
		t.Fatalf("create producer: %v", err)
	}
	defer producer.Close()

	pool := workerpool.New(2, 4)
	defer pool.Shutdown()

	handler := kafkapkg.NewConsumerHandler(pool, processing.NewDefaultProcessor(), producer, kafkapkg.HandlerConfig{
		TopicPaymentsProcessed: "payments.processed",
		TopicPaymentsDLQ:       "payments.dlq",
		ProcessingTimeout:      5 * time.Second,
		MaxRetries:             1,
		RetryBackoff:           100 * time.Millisecond,
	}, discardLogger())

	processorGroupID := fmt.Sprintf("it-processor-%d", time.Now().UnixNano())
	go func() {
		_ = kafkapkg.RunConsumerGroup(ctx, brokerAddrs, processorGroupID, []string{"payments.created"}, handler, discardLogger())
	}()

	// Give both consumer groups a moment to join before producing —
	// otherwise a fresh-group OffsetNewest subscriber can join AFTER
	// the produce and miss it. This is the one place a short fixed
	// wait is more practical than a readiness signal, since consumer
	// group join completion isn't observable from the test.
	time.Sleep(3 * time.Second)

	event := domain.PaymentEvent{
		TransactionID:       fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", time.Now().UnixNano()%1_000_000_000_000),
		SourceCountry:       "CO",
		DestinationCountry:  "US",
		SourceCurrency:      "COP",
		DestinationCurrency: "USD",
		SourceAmount:        "100.00",
		DestinationAmount:   "0.03",
		FXRate:              "0.00025",
		Provider:            "provider_a",
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	if err := producer.Publish(ctx, "payments.created", []byte(event.TransactionID), payload); err != nil {
		t.Fatalf("publish payments.created: %v", err)
	}

	// payments.processed can also carry unrelated traffic (the actual
	// docker-compose payment-processor container, if running, consumes
	// payments.created independently and publishes its own outcomes to
	// the same topic) — filter for this test's own transaction id
	// rather than assuming the first message observed is ours.
	outcome, err := awaitOutcome(ctx, collector.out, event.TransactionID)
	if err != nil {
		t.Fatal(err)
	}

	if outcome.Status != domain.StatusCompleted {
		t.Errorf("expected status %q, got %q", domain.StatusCompleted, outcome.Status)
	}
}

// awaitOutcome reads from out until it finds a payments.processed
// message for wantID, ignoring any unrelated messages (from other
// traffic on the same topic), or returns an error once ctx is done.
func awaitOutcome(ctx context.Context, out <-chan []byte, wantID string) (domain.Outcome, error) {
	for {
		select {
		case raw := <-out:
			var outcome domain.Outcome
			if err := json.Unmarshal(raw, &outcome); err != nil {
				continue
			}
			if outcome.TransactionID == wantID {
				return outcome, nil
			}
		case <-ctx.Done():
			return domain.Outcome{}, fmt.Errorf("timed out waiting for payments.processed message for transaction %s", wantID)
		}
	}
}
