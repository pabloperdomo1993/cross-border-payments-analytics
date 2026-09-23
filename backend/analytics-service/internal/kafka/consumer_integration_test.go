//go:build integration

// A single, focused integration test against REAL Kafka + REAL
// ClickHouse together, covering the critical flow: a real
// payments.processed message is consumed by the actual ConsumerHandler
// and lands as a real row in ClickHouse, queryable through the same
// repository the HTTP API uses. Run with:
//
//	docker compose up -d kafka kafka-init clickhouse
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

	clickhousedriver "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/domain"
	kafkapkg "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/kafka"
	chrepo "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/repository/clickhouse"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestIntegration_PaymentsProcessed_ReachesAnalyticsService_And_LandsInClickHouse(t *testing.T) {
	brokerAddrs := strings.Split(envOr("TEST_KAFKA_BROKERS", "localhost:29092"), ",")

	producerCfg := sarama.NewConfig()
	producerCfg.Producer.Return.Successes = true
	producerCfg.Net.DialTimeout = 3 * time.Second
	producer, err := sarama.NewSyncProducer(brokerAddrs, producerCfg)
	if err != nil {
		t.Skipf("skipping: cannot reach test Kafka at %v (run `docker compose up -d kafka kafka-init` first): %v", brokerAddrs, err)
	}
	defer producer.Close()

	chAddr := envOr("TEST_CLICKHOUSE_ADDR", "localhost:9000")
	conn, err := clickhousedriver.Open(&clickhousedriver.Options{
		Addr: []string{chAddr},
		Auth: clickhousedriver.Auth{
			Database: envOr("TEST_CLICKHOUSE_DATABASE", "analytics"),
			Username: envOr("TEST_CLICKHOUSE_USER", "default"),
			Password: envOr("TEST_CLICKHOUSE_PASSWORD", "analytics"),
		},
	})
	if err != nil {
		t.Fatalf("open clickhouse: %v", err)
	}
	defer conn.Close()
	{
		pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := conn.Ping(pingCtx); err != nil {
			t.Skipf("skipping: cannot reach test ClickHouse at %s (run `docker compose up -d clickhouse` first): %v", chAddr, err)
		}
	}
	repo := chrepo.NewAnalyticsRepository(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	handler := kafkapkg.NewConsumerHandler(repo, kafkapkg.BatchConfig{
		MaxSize:     1,
		MaxInterval: 200 * time.Millisecond,
	}, discardLogger())

	groupID := fmt.Sprintf("it-analytics-%d", time.Now().UnixNano())
	go func() {
		_ = kafkapkg.RunConsumerGroup(ctx, brokerAddrs, groupID, []string{"payments.processed"}, handler, discardLogger())
	}()

	// Let the consumer group join before producing (fresh group,
	// OffsetOldest per this service's config — see internal/kafka's
	// runner — so it would eventually catch up regardless, but this
	// keeps the test's own message the most recent, deterministic one
	// to look for).
	time.Sleep(3 * time.Second)

	provider := fmt.Sprintf("integration_test_kafka_%d", time.Now().UnixNano())
	txID := fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", time.Now().UnixNano()%1_000_000_000_000)
	outcome := domain.PaymentOutcome{
		TransactionID:       txID,
		SourceCountry:       "BR",
		DestinationCountry:  "US",
		SourceCurrency:      "BRL",
		DestinationCurrency: "USD",
		SourceAmount:        "250.00",
		DestinationAmount:   "50.00",
		FXRate:              "0.2",
		Provider:            provider,
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
		Status:              domain.StatusCompleted,
	}
	payload, err := json.Marshal(outcome)
	if err != nil {
		t.Fatalf("marshal outcome: %v", err)
	}

	if _, _, err := producer.SendMessage(&sarama.ProducerMessage{
		Topic: "payments.processed",
		Key:   sarama.StringEncoder(txID),
		Value: sarama.ByteEncoder(payload),
	}); err != nil {
		t.Fatalf("publish payments.processed: %v", err)
	}

	deadline := time.After(15 * time.Second)
	for {
		results, err := repo.Corridors(context.Background(), domain.Filter{Provider: provider})
		if err != nil {
			t.Fatalf("query corridors: %v", err)
		}
		if len(results) == 1 && results[0].SourceCountry == "BR" && results[0].DestinationCountry == "US" {
			if results[0].TransactionCount != 1 || results[0].TotalVolume != "250.00" {
				t.Fatalf("unexpected aggregated result: %+v", results[0])
			}
			return // success
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for the payments.processed message to land in ClickHouse (got %d matching rows)", len(results))
		default:
		}
	}
}
