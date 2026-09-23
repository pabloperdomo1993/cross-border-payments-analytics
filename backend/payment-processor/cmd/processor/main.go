// Command processor runs the payment-processor Kafka consumer: it reads
// payments.created events, processes them through a bounded worker
// pool, and publishes the outcome to payments.processed or payments.dlq.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/config"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/kafka"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/metrics"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/processing"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/workerpool"
)

const shutdownTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("service exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	producer, err := kafka.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		return err
	}

	// consumeCtx governs the Kafka consumer loop only: cancelling it
	// stops new messages from being claimed, which is step one of the
	// documented shutdown sequence (stop consuming -> stop accepting
	// new jobs -> drain in-flight -> close resources).
	consumeCtx, stopConsuming := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopConsuming()

	pool := workerpool.New(cfg.WorkerCount, cfg.WorkerQueueSize)
	metrics.RegisterWorkerPoolGauges(pool)

	handler := kafka.NewConsumerHandler(pool, processing.NewDefaultProcessor(), producer, kafka.HandlerConfig{
		TopicPaymentsProcessed: cfg.TopicPaymentsProcessed,
		TopicPaymentsDLQ:       cfg.TopicPaymentsDLQ,
		ProcessingTimeout:      cfg.ProcessingTimeout,
		MaxRetries:             cfg.MaxRetries,
		RetryBackoff:           cfg.RetryBackoff,
	}, logger)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// A short-lived client connection is this service's equivalent
		// of a DB/ClickHouse ping elsewhere: it verifies Kafka is
		// actually reachable, not just that this process is alive.
		readyCfg := sarama.NewConfig()
		readyCfg.Net.DialTimeout = 2 * time.Second
		client, err := sarama.NewClient(cfg.KafkaBrokers, readyCfg)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}
		defer client.Close()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	metricsServer := &http.Server{
		Addr:              ":" + cfg.MetricsPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("metrics server listening", slog.String("addr", metricsServer.Addr))
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server error", slog.String("error", err.Error()))
		}
	}()

	consumeErrs := make(chan error, 1)
	go func() {
		topics := []string{cfg.TopicPaymentsCreated}
		logger.Info("starting kafka consumer",
			slog.Any("brokers", cfg.KafkaBrokers),
			slog.String("group", cfg.KafkaConsumerGroup),
			slog.Any("topics", topics),
			slog.Int("worker_count", cfg.WorkerCount),
			slog.Int("worker_queue_size", cfg.WorkerQueueSize),
		)
		consumeErrs <- kafka.RunConsumerGroup(consumeCtx, cfg.KafkaBrokers, cfg.KafkaConsumerGroup, topics, handler, logger)
	}()

	var runErr error
	select {
	case runErr = <-consumeErrs:
		stopConsuming()
	case <-consumeCtx.Done():
		logger.Info("shutdown signal received")
		<-consumeErrs // wait for the consumer loop to actually stop consuming
	}

	// Step two: no more messages are being claimed. Stop accepting new
	// jobs and let in-flight ones finish, bounded by shutdownTimeout —
	// if it's exceeded we proceed to close resources anyway (any
	// abandoned in-flight message stays uncommitted and is safely
	// redelivered later).
	shutdownDone := make(chan struct{})
	go func() {
		pool.Shutdown()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(shutdownTimeout):
		logger.Warn("worker pool shutdown timed out; proceeding to close resources")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("metrics server shutdown error", slog.String("error", err.Error()))
	}

	if err := producer.Close(); err != nil {
		logger.Warn("producer close error", slog.String("error", err.Error()))
	}

	logger.Info("shutdown complete")
	return runErr
}
