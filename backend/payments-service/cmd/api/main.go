// Command api runs the payments-service HTTP server.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/application"
	httphandler "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/handlers/http"
	paymentskafka "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/kafka"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/outbox"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/repository/mariadb"
)

const (
	shutdownTimeout   = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second

	dbMaxOpenConns    = 25
	dbMaxIdleConns    = 25
	dbConnMaxLifetime = 5 * time.Minute
)

type config struct {
	httpPort   string
	dbHost     string
	dbPort     string
	dbName     string
	dbUser     string
	dbPassword string

	kafkaBrokers       []string
	outboxTopic        string
	outboxPollInterval time.Duration
	outboxBatchSize    int
}

func loadConfig() config {
	return config{
		httpPort:           getEnv("HTTP_PORT", "8080"),
		dbHost:             getEnv("DB_HOST", "localhost"),
		dbPort:             getEnv("DB_PORT", "3306"),
		dbName:             getEnv("DB_NAME", "payments"),
		dbUser:             getEnv("DB_USER", "payments"),
		dbPassword:         os.Getenv("DB_PASSWORD"),
		kafkaBrokers:       strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
		outboxTopic:        getEnv("KAFKA_TOPIC_PAYMENTS_CREATED", "payments.created"),
		outboxPollInterval: getDuration("OUTBOX_POLL_INTERVAL", 500*time.Millisecond),
		outboxBatchSize:    getInt("OUTBOX_BATCH_SIZE", 50),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("service exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := loadConfig()

	db, err := openDB(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	producer, err := paymentskafka.NewProducer(cfg.kafkaBrokers)
	if err != nil {
		return fmt.Errorf("open kafka producer: %w", err)
	}

	repo := mariadb.NewTransactionRepository(db)
	createTx := application.NewCreateTransaction(repo)
	getTx := application.NewGetTransaction(repo)

	txHandler := httphandler.NewTransactionHandler(createTx, getTx, logger)
	healthHandler := httphandler.NewHealthHandler(db)
	router := httphandler.NewRouter(txHandler, healthHandler, logger)

	server := &http.Server{
		Addr:              ":" + cfg.httpPort,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	relay := outbox.NewRelay(repo, producer, outbox.Config{
		Topic:        cfg.outboxTopic,
		PollInterval: cfg.outboxPollInterval,
		BatchSize:    cfg.outboxBatchSize,
	}, logger)

	relayDone := make(chan struct{})
	go func() {
		logger.Info("outbox relay started",
			slog.String("topic", cfg.outboxTopic),
			slog.Duration("poll_interval", cfg.outboxPollInterval),
		)
		relay.Run(ctx)
		close(relayDone)
	}()

	serverErrs := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrs <- err
			return
		}
		serverErrs <- nil
	}()

	var runErr error
	select {
	case runErr = <-serverErrs:
		stop()
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http server shutdown error", slog.String("error", err.Error()))
	}

	select {
	case <-relayDone:
	case <-time.After(shutdownTimeout):
		logger.Warn("outbox relay shutdown timed out")
	}

	if err := producer.Close(); err != nil {
		logger.Warn("producer close error", slog.String("error", err.Error()))
	}

	logger.Info("shutdown complete")
	return runErr
}

func openDB(cfg config) (*sql.DB, error) {
	mysqlCfg := mysql.Config{
		User:                 cfg.dbUser,
		Passwd:               cfg.dbPassword,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%s", cfg.dbHost, cfg.dbPort),
		DBName:               cfg.dbName,
		ParseTime:            true,
		AllowNativePasswords: true,
	}
	dsn := mysqlCfg.FormatDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(dbMaxOpenConns)
	db.SetMaxIdleConns(dbMaxIdleConns)
	db.SetConnMaxLifetime(dbConnMaxLifetime)

	return db, nil
}
