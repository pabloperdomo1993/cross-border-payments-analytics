// Package config loads and validates analytics-service's configuration
// from environment variables, following the same convention as
// payments-service and payment-processor.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds analytics-service's full runtime configuration.
type Config struct {
	HTTPPort string

	ClickHouseAddr     string
	ClickHouseDatabase string
	ClickHouseUser     string
	ClickHousePassword string

	KafkaBrokers           []string
	KafkaConsumerGroup     string
	TopicPaymentsProcessed string
	TopicPaymentsDLQ       string
	BatchMaxSize           int
	BatchMaxInterval       time.Duration
}

// Load reads configuration from environment variables, applying the
// defaults below, and validates it, returning a descriptive error
// listing every invalid value rather than failing on the first one.
func Load() (Config, error) {
	cfg := Config{
		HTTPPort:               getEnv("HTTP_PORT", "8082"),
		ClickHouseAddr:         getEnv("CLICKHOUSE_ADDR", "localhost:9000"),
		ClickHouseDatabase:     getEnv("CLICKHOUSE_DATABASE", "analytics"),
		ClickHouseUser:         getEnv("CLICKHOUSE_USER", "default"),
		ClickHousePassword:     os.Getenv("CLICKHOUSE_PASSWORD"),
		KafkaConsumerGroup:     getEnv("KAFKA_CONSUMER_GROUP", "analytics-service"),
		TopicPaymentsProcessed: getEnv("KAFKA_TOPIC_PAYMENTS_PROCESSED", "payments.processed"),
		TopicPaymentsDLQ:       getEnv("KAFKA_TOPIC_PAYMENTS_DLQ", "payments.dlq"),
		KafkaBrokers:           splitAndTrim(getEnv("KAFKA_BROKERS", "localhost:9092")),
	}

	var errs []string

	batchMaxSize, err := parseInt("ANALYTICS_BATCH_MAX_SIZE", "500")
	if err != nil {
		errs = append(errs, err.Error())
	} else if batchMaxSize <= 0 {
		errs = append(errs, "ANALYTICS_BATCH_MAX_SIZE must be > 0")
	}
	cfg.BatchMaxSize = batchMaxSize

	batchMaxInterval, err := parseDuration("ANALYTICS_BATCH_MAX_INTERVAL", "2s")
	if err != nil {
		errs = append(errs, err.Error())
	} else if batchMaxInterval <= 0 {
		errs = append(errs, "ANALYTICS_BATCH_MAX_INTERVAL must be > 0")
	}
	cfg.BatchMaxInterval = batchMaxInterval

	if len(cfg.KafkaBrokers) == 0 {
		errs = append(errs, "KAFKA_BROKERS must not be empty")
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseInt(key, fallback string) (int, error) {
	raw := getEnv(key, fallback)
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, raw)
	}
	return v, nil
}

func parseDuration(key, fallback string) (time.Duration, error) {
	raw := getEnv(key, fallback)
	v, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration (e.g. \"2s\"), got %q", key, raw)
	}
	return v, nil
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
