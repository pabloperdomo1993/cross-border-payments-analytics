//go:build integration

// Integration test proving the relay correctly reads pending events
// from and marks them published in a REAL MariaDB — the Kafka side is
// a fake here deliberately, so this test isolates "does the relay talk
// to the real database correctly" from "does a message really reach
// Kafka", which the dedicated Kafka integration tests in
// payment-processor/analytics-service cover instead. Run with:
//
//	docker compose up -d mariadb
//	go test -tags=integration ./internal/outbox/...
package outbox_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/outbox"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/repository/mariadb"
)

// newUUID generates a UUIDv4-shaped string for test fixtures.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()

	host := envOr("TEST_DB_HOST", "localhost")
	port := envOr("TEST_DB_PORT", "3306")
	name := envOr("TEST_DB_NAME", "payments")
	user := envOr("TEST_DB_USER", "payments")
	pass := envOr("TEST_DB_PASSWORD", "payments")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, pass, host, port, name)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("skipping: cannot reach test MariaDB (run `docker compose up -d mariadb` first): %v", err)
	}
	return db
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type recordingPublisher struct {
	mu    sync.Mutex
	calls []string // aggregate ids published
}

func (p *recordingPublisher) Publish(_ context.Context, _ string, key, _ []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, string(key))
	return nil
}

func (p *recordingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func TestIntegration_Relay_PublishesRealPendingEventAndMarksItInDB(t *testing.T) {
	db := integrationDB(t)
	repo := mariadb.NewTransactionRepository(db)
	ctx := context.Background()

	// Seed one real pending outbox event via the real repository (the
	// same atomic Create path payments-service's API uses).
	amount, _ := domain.ParseMoney("50.00")
	rate, _ := domain.ParseFXRate("0.02")
	id, err := newUUID()
	if err != nil {
		t.Fatalf("generate id: %v", err)
	}
	eventID, err := newUUID()
	if err != nil {
		t.Fatalf("generate event id: %v", err)
	}
	tx, err := domain.NewTransaction(domain.NewTransactionParams{
		ID:                  domain.TransactionID(id),
		IdempotencyKey:      domain.IdempotencyKey("idem-" + id),
		SourceCountry:       "MX",
		DestinationCountry:  "US",
		SourceCurrency:      "MXN",
		DestinationCurrency: "USD",
		SourceAmount:        amount,
		DestinationAmount:   amount.Multiply(rate),
		FXRate:              rate,
		Provider:            "provider_b",
	})
	if err != nil {
		t.Fatalf("build transaction: %v", err)
	}
	event, err := domain.NewOutboxEvent(eventID, string(tx.ID), "payment.created", `{"id":"`+string(tx.ID)+`"}`, tx.CreatedAt)
	if err != nil {
		t.Fatalf("build outbox event: %v", err)
	}
	if err := repo.Create(ctx, tx, event); err != nil {
		t.Fatalf("seed transaction+event: %v", err)
	}

	pub := &recordingPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	relay := outbox.NewRelay(repo, pub, outbox.Config{
		Topic:        "payments.created",
		PollInterval: 50 * time.Millisecond,
		BatchSize:    10,
	}, logger)

	relayCtx, cancel := context.WithCancel(ctx)
	go relay.Run(relayCtx)
	defer cancel()

	deadline := time.After(5 * time.Second)
	for pub.count() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the relay to publish the seeded event")
		default:
		}
	}

	// Verify the DATABASE (not just the in-memory relay) reflects
	// "published" — re-fetch pending events and confirm this one is
	// gone.
	deadline2 := time.After(5 * time.Second)
	for {
		pending, err := repo.FetchPendingOutboxEvents(ctx, 1000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		stillPending := false
		for _, ev := range pending {
			if ev.ID == event.ID {
				stillPending = true
			}
		}
		if !stillPending {
			break
		}
		select {
		case <-deadline2:
			t.Fatal("timed out waiting for the event to be marked published in the database")
		default:
		}
	}
}
