package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/config"
)

// clearEnv ensures each test starts from defaults regardless of the
// developer's shell environment.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"KAFKA_BROKERS", "KAFKA_CONSUMER_GROUP",
		"KAFKA_TOPIC_PAYMENTS_CREATED", "KAFKA_TOPIC_PAYMENTS_PROCESSED", "KAFKA_TOPIC_PAYMENTS_DLQ",
		"WORKER_COUNT", "WORKER_QUEUE_SIZE",
		"PAYMENT_PROCESSING_TIMEOUT", "PAYMENT_MAX_RETRIES", "PAYMENT_RETRY_BACKOFF",
		"METRICS_PORT",
	} {
		t.Setenv(key, "")
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WorkerCount != 10 {
		t.Errorf("expected default WorkerCount 10, got %d", cfg.WorkerCount)
	}
	if cfg.WorkerQueueSize != 100 {
		t.Errorf("expected default WorkerQueueSize 100, got %d", cfg.WorkerQueueSize)
	}
	if cfg.ProcessingTimeout != 5*time.Second {
		t.Errorf("expected default ProcessingTimeout 5s, got %v", cfg.ProcessingTimeout)
	}
	if len(cfg.KafkaBrokers) != 1 || cfg.KafkaBrokers[0] != "localhost:9092" {
		t.Errorf("expected default KafkaBrokers [localhost:9092], got %v", cfg.KafkaBrokers)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("WORKER_COUNT", "25")
	t.Setenv("WORKER_QUEUE_SIZE", "500")
	t.Setenv("PAYMENT_PROCESSING_TIMEOUT", "2s")
	t.Setenv("KAFKA_BROKERS", "broker1:9092, broker2:9092")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WorkerCount != 25 {
		t.Errorf("expected WorkerCount 25, got %d", cfg.WorkerCount)
	}
	if cfg.WorkerQueueSize != 500 {
		t.Errorf("expected WorkerQueueSize 500, got %d", cfg.WorkerQueueSize)
	}
	if cfg.ProcessingTimeout != 2*time.Second {
		t.Errorf("expected ProcessingTimeout 2s, got %v", cfg.ProcessingTimeout)
	}
	if len(cfg.KafkaBrokers) != 2 || cfg.KafkaBrokers[0] != "broker1:9092" || cfg.KafkaBrokers[1] != "broker2:9092" {
		t.Errorf("expected two trimmed brokers, got %v", cfg.KafkaBrokers)
	}
}

func TestLoad_InvalidValues(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"zero worker count", map[string]string{"WORKER_COUNT": "0"}},
		{"negative worker count", map[string]string{"WORKER_COUNT": "-1"}},
		{"non-numeric worker count", map[string]string{"WORKER_COUNT": "abc"}},
		{"zero queue size", map[string]string{"WORKER_QUEUE_SIZE": "0"}},
		{"negative queue size", map[string]string{"WORKER_QUEUE_SIZE": "-5"}},
		{"zero processing timeout", map[string]string{"PAYMENT_PROCESSING_TIMEOUT": "0s"}},
		{"invalid processing timeout", map[string]string{"PAYMENT_PROCESSING_TIMEOUT": "not-a-duration"}},
		{"negative max retries", map[string]string{"PAYMENT_MAX_RETRIES": "-1"}},
		{"zero retry backoff", map[string]string{"PAYMENT_RETRY_BACKOFF": "0s"}},
		{"blank kafka brokers", map[string]string{"KAFKA_BROKERS": "   "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected an error for %v, got nil", tt.env)
			}
		})
	}
}

func TestLoad_MultipleInvalidValuesReportedTogether(t *testing.T) {
	clearEnv(t)
	t.Setenv("WORKER_COUNT", "0")
	t.Setenv("WORKER_QUEUE_SIZE", "-1")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "WORKER_COUNT") || !strings.Contains(msg, "WORKER_QUEUE_SIZE") {
		t.Errorf("expected error to mention both invalid fields, got: %s", msg)
	}
}
