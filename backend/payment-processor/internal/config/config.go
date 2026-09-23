// Package config loads and validates payment-processor's configuration
// from environment variables, following the same env-var convention
// payments-service uses. Unlike payments-service's plain defaults, most
// values here have runtime constraints (positive counts, positive
// durations) that must be checked before the service starts, so Load
// returns an error a caller can fail fast on instead of a bare struct.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds payment-processor's full runtime configuration.
type Config struct {
	KafkaBrokers           []string
	KafkaConsumerGroup     string
	TopicPaymentsCreated   string
	TopicPaymentsProcessed string
	TopicPaymentsDLQ       string

	WorkerCount       int
	WorkerQueueSize   int
	ProcessingTimeout time.Duration
	MaxRetries        int
	RetryBackoff      time.Duration

	MetricsPort string
}

// Load reads configuration from environment variables, applying the
// defaults below, and validates it. It returns a descriptive error
// listing every invalid value rather than failing on the first one, so
// a misconfigured deployment can be fixed in one pass.
func Load() (Config, error) {
	cfg := Config{
		KafkaBrokers:           splitAndTrim(getEnv("KAFKA_BROKERS", "localhost:9092")),
		KafkaConsumerGroup:     getEnv("KAFKA_CONSUMER_GROUP", "payment-processor"),
		TopicPaymentsCreated:   getEnv("KAFKA_TOPIC_PAYMENTS_CREATED", "payments.created"),
		TopicPaymentsProcessed: getEnv("KAFKA_TOPIC_PAYMENTS_PROCESSED", "payments.processed"),
		TopicPaymentsDLQ:       getEnv("KAFKA_TOPIC_PAYMENTS_DLQ", "payments.dlq"),
		MetricsPort:            getEnv("METRICS_PORT", "9091"),
	}

	var errs []string

	workerCount, err := parseInt("WORKER_COUNT", "10")
	if err != nil {
		errs = append(errs, err.Error())
	} else if workerCount <= 0 {
		errs = append(errs, "WORKER_COUNT must be > 0")
	}
	cfg.WorkerCount = workerCount

	queueSize, err := parseInt("WORKER_QUEUE_SIZE", "100")
	if err != nil {
		errs = append(errs, err.Error())
	} else if queueSize <= 0 {
		errs = append(errs, "WORKER_QUEUE_SIZE must be > 0")
	}
	cfg.WorkerQueueSize = queueSize

	processingTimeout, err := parseDuration("PAYMENT_PROCESSING_TIMEOUT", "5s")
	if err != nil {
		errs = append(errs, err.Error())
	} else if processingTimeout <= 0 {
		errs = append(errs, "PAYMENT_PROCESSING_TIMEOUT must be > 0")
	}
	cfg.ProcessingTimeout = processingTimeout

	maxRetries, err := parseInt("PAYMENT_MAX_RETRIES", "3")
	if err != nil {
		errs = append(errs, err.Error())
	} else if maxRetries < 0 {
		errs = append(errs, "PAYMENT_MAX_RETRIES must be >= 0")
	}
	cfg.MaxRetries = maxRetries

	retryBackoff, err := parseDuration("PAYMENT_RETRY_BACKOFF", "200ms")
	if err != nil {
		errs = append(errs, err.Error())
	} else if retryBackoff <= 0 {
		errs = append(errs, "PAYMENT_RETRY_BACKOFF must be > 0")
	}
	cfg.RetryBackoff = retryBackoff

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
		return 0, fmt.Errorf("%s must be a valid duration (e.g. \"5s\"), got %q", key, raw)
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
