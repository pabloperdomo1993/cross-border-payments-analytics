// Command api runs the analytics-service HTTP API and its Kafka
// ingestion consumer.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	clickhousedriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/config"
	httphandler "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/handlers/http"
	analyticskafka "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/kafka"
	chrepo "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/repository/clickhouse"
)

const (
	shutdownTimeout   = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

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

	conn, err := openClickHouse(cfg)
	if err != nil {
		return fmt.Errorf("open clickhouse: %w", err)
	}
	defer conn.Close()

	repo := chrepo.NewAnalyticsRepository(conn)

	analyticsHandler := httphandler.NewAnalyticsHandler(repo, logger)
	healthHandler := httphandler.NewHealthHandler(repo)
	router := httphandler.NewRouter(analyticsHandler, healthHandler, logger, cfg.CORSAllowedOrigin)

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErrs := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrs <- err
			return
		}
		serverErrs <- nil
	}()

	consumerHandler := analyticskafka.NewConsumerHandler(repo, analyticskafka.BatchConfig{
		MaxSize:     cfg.BatchMaxSize,
		MaxInterval: cfg.BatchMaxInterval,
	}, logger)

	consumeErrs := make(chan error, 1)
	go func() {
		topics := []string{cfg.TopicPaymentsProcessed, cfg.TopicPaymentsDLQ}
		logger.Info("starting kafka consumer",
			slog.Any("brokers", cfg.KafkaBrokers),
			slog.String("group", cfg.KafkaConsumerGroup),
			slog.Any("topics", topics),
		)
		consumeErrs <- analyticskafka.RunConsumerGroup(ctx, cfg.KafkaBrokers, cfg.KafkaConsumerGroup, topics, consumerHandler, logger)
	}()

	var runErr error
	consumerAlreadyStopped := false
	select {
	case runErr = <-serverErrs:
		stop()
	case err := <-consumeErrs:
		if err != nil {
			runErr = err
		}
		consumerAlreadyStopped = true
		stop()
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http server shutdown error", slog.String("error", err.Error()))
	}

	if !consumerAlreadyStopped {
		select {
		case <-consumeErrs:
		case <-time.After(shutdownTimeout):
			logger.Warn("kafka consumer shutdown timed out")
		}
	}

	logger.Info("shutdown complete")
	return runErr
}

func openClickHouse(cfg config.Config) (driver.Conn, error) {
	conn, err := clickhousedriver.Open(&clickhousedriver.Options{
		Addr: []string{cfg.ClickHouseAddr},
		Auth: clickhousedriver.Auth{
			Database: cfg.ClickHouseDatabase,
			Username: cfg.ClickHouseUser,
			Password: cfg.ClickHousePassword,
		},
	})
	if err != nil {
		return nil, err
	}
	return conn, nil
}
