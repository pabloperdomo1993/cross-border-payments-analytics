package config_test

import (
	"testing"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/config"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"HTTP_PORT", "CLICKHOUSE_ADDR", "CLICKHOUSE_DATABASE", "CLICKHOUSE_USER", "CLICKHOUSE_PASSWORD",
		"KAFKA_BROKERS", "KAFKA_CONSUMER_GROUP", "KAFKA_TOPIC_PAYMENTS_PROCESSED", "KAFKA_TOPIC_PAYMENTS_DLQ",
		"ANALYTICS_BATCH_MAX_SIZE", "ANALYTICS_BATCH_MAX_INTERVAL",
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
	if cfg.HTTPPort != "8082" {
		t.Errorf("expected default HTTPPort 8082, got %s", cfg.HTTPPort)
	}
	if cfg.BatchMaxSize != 500 {
		t.Errorf("expected default BatchMaxSize 500, got %d", cfg.BatchMaxSize)
	}
	if cfg.BatchMaxInterval != 2*time.Second {
		t.Errorf("expected default BatchMaxInterval 2s, got %v", cfg.BatchMaxInterval)
	}
}

func TestLoad_InvalidValues(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"zero batch size", map[string]string{"ANALYTICS_BATCH_MAX_SIZE": "0"}},
		{"negative batch size", map[string]string{"ANALYTICS_BATCH_MAX_SIZE": "-1"}},
		{"non-numeric batch size", map[string]string{"ANALYTICS_BATCH_MAX_SIZE": "abc"}},
		{"zero batch interval", map[string]string{"ANALYTICS_BATCH_MAX_INTERVAL": "0s"}},
		{"invalid batch interval", map[string]string{"ANALYTICS_BATCH_MAX_INTERVAL": "nope"}},
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
