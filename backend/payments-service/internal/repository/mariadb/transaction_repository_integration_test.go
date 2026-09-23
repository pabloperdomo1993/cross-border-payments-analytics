//go:build integration

// Integration tests against a REAL MariaDB instance — no mocked
// database/sql driver. Run with:
//
//	docker compose up -d mariadb
//	go test -tags=integration ./internal/repository/mariadb/...
//
// Connection defaults to the ports docker-compose.yml already publishes
// (localhost:3306); override via TEST_DB_* env vars if needed.
package mariadb_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/repository/mariadb"
)

// newUUID generates a UUIDv4-shaped string for test fixtures, deliberately
// not importing the application package's ID generator to keep this
// repository-layer test independent of the application layer.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func testDB(t *testing.T) *sql.DB {
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
		t.Skipf("skipping: cannot reach test MariaDB at %s:%s (run `docker compose up -d mariadb` first): %v", host, port, err)
	}
	return db
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// validTransaction returns a Transaction+OutboxEvent pair with unique
// IDs per call, so parallel/sequential tests never collide on the
// unique idempotency_key or primary key constraints.
func validTransaction(t *testing.T) (*domain.Transaction, *domain.OutboxEvent) {
	t.Helper()

	id, err := newUUID()
	if err != nil {
		t.Fatalf("generate id: %v", err)
	}
	eventID, err := newUUID()
	if err != nil {
		t.Fatalf("generate event id: %v", err)
	}

	amount, _ := domain.ParseMoney("100.00")
	rate, _ := domain.ParseFXRate("0.01")

	tx, err := domain.NewTransaction(domain.NewTransactionParams{
		ID:                  domain.TransactionID(id),
		IdempotencyKey:      domain.IdempotencyKey("idem-" + id),
		SourceCountry:       "CO",
		DestinationCountry:  "US",
		SourceCurrency:      "COP",
		DestinationCurrency: "USD",
		SourceAmount:        amount,
		DestinationAmount:   amount.Multiply(rate),
		FXRate:              rate,
		Provider:            "provider_a",
	})
	if err != nil {
		t.Fatalf("build transaction: %v", err)
	}

	event, err := domain.NewOutboxEvent(eventID, id, "payment.created", `{"id":"`+id+`"}`, tx.CreatedAt)
	if err != nil {
		t.Fatalf("build outbox event: %v", err)
	}

	return tx, event
}

func TestIntegration_Create_InsertsPaymentAndOutboxEventAtomically(t *testing.T) {
	db := testDB(t)
	repo := mariadb.NewTransactionRepository(db)
	ctx := context.Background()

	tx, event := validTransaction(t)

	if err := repo.Create(ctx, tx, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := repo.FindByID(ctx, string(tx.ID))
	if err != nil {
		t.Fatalf("expected transaction to be retrievable, got error: %v", err)
	}
	if got.ID != tx.ID {
		t.Errorf("expected id %q, got %q", tx.ID, got.ID)
	}

	pending, err := repo.FetchPendingOutboxEvents(ctx, 1000)
	if err != nil {
		t.Fatalf("unexpected error fetching pending events: %v", err)
	}
	found := false
	for _, ev := range pending {
		if ev.ID == event.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected the outbox event to be present among pending events")
	}
}

func TestIntegration_Create_RollsBackPaymentWhenOutboxInsertFails(t *testing.T) {
	db := testDB(t)
	repo := mariadb.NewTransactionRepository(db)
	ctx := context.Background()

	tx, event := validTransaction(t)
	// A payload exceeding TEXT's ~64KB capacity forces the outbox
	// INSERT to fail (data too long), while the transactions INSERT
	// alone would have succeeded — this is exactly the scenario the
	// transactional outbox must guard against.
	oversized := make([]byte, 100_000)
	for i := range oversized {
		oversized[i] = 'x'
	}
	event.Payload = string(oversized)

	err := repo.Create(ctx, tx, event)
	if err == nil {
		t.Fatal("expected an error when the outbox event insert fails")
	}
	if !errors.Is(err, domain.ErrRepository) {
		t.Errorf("expected error to wrap domain.ErrRepository, got %v", err)
	}

	if _, err := repo.FindByID(ctx, string(tx.ID)); !errors.Is(err, domain.ErrTransactionNotFound) {
		t.Errorf("expected the payment to NOT be persisted after the outbox insert failed (atomicity), got err=%v", err)
	}
}

func TestIntegration_Create_DuplicateIdempotencyKeyConflicts(t *testing.T) {
	db := testDB(t)
	repo := mariadb.NewTransactionRepository(db)
	ctx := context.Background()

	tx1, event1 := validTransaction(t)
	if err := repo.Create(ctx, tx1, event1); err != nil {
		t.Fatalf("unexpected error creating first transaction: %v", err)
	}

	// Second transaction: different id, but the SAME idempotency_key —
	// must be rejected by the unique constraint.
	tx2, event2 := validTransaction(t)
	tx2.IdempotencyKey = tx1.IdempotencyKey

	err := repo.Create(ctx, tx2, event2)
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("expected domain.ErrConflict for a duplicate idempotency_key, got %v", err)
	}

	if _, err := repo.FindByID(ctx, string(tx2.ID)); !errors.Is(err, domain.ErrTransactionNotFound) {
		t.Errorf("expected the second (conflicting) transaction to not be persisted, got err=%v", err)
	}
}

func TestIntegration_Create_DuplicateID_DatabaseConstraint(t *testing.T) {
	db := testDB(t)
	repo := mariadb.NewTransactionRepository(db)
	ctx := context.Background()

	tx, event := validTransaction(t)
	if err := repo.Create(ctx, tx, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Same primary key (id), different idempotency_key and event id —
	// the PRIMARY KEY constraint on transactions.id must reject it.
	tx2, event2 := validTransaction(t)
	tx2.ID = tx.ID

	err := repo.Create(ctx, tx2, event2)
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("expected domain.ErrConflict for a duplicate primary key, got %v", err)
	}
}

func TestIntegration_FindByID_NotFound(t *testing.T) {
	db := testDB(t)
	repo := mariadb.NewTransactionRepository(db)

	_, err := repo.FindByID(context.Background(), "00000000-0000-4000-8000-000000000000")
	if !errors.Is(err, domain.ErrTransactionNotFound) {
		t.Errorf("expected domain.ErrTransactionNotFound, got %v", err)
	}
}

func TestIntegration_UpdateStatus(t *testing.T) {
	db := testDB(t)
	repo := mariadb.NewTransactionRepository(db)
	ctx := context.Background()

	tx, event := validTransaction(t)
	if err := repo.Create(ctx, tx, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := tx.MarkCompleted(); err != nil {
		t.Fatalf("unexpected domain error: %v", err)
	}
	if err := repo.UpdateStatus(ctx, string(tx.ID), tx.Status, tx.UpdatedAt); err != nil {
		t.Fatalf("unexpected error updating status: %v", err)
	}

	got, err := repo.FindByID(ctx, string(tx.ID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != domain.StatusCompleted {
		t.Errorf("expected status %q, got %q", domain.StatusCompleted, got.Status)
	}
}

func TestIntegration_UpdateStatus_NotFound(t *testing.T) {
	db := testDB(t)
	repo := mariadb.NewTransactionRepository(db)

	err := repo.UpdateStatus(context.Background(), "00000000-0000-4000-8000-000000000000", domain.StatusCompleted, time.Now().UTC())
	if !errors.Is(err, domain.ErrTransactionNotFound) {
		t.Errorf("expected domain.ErrTransactionNotFound, got %v", err)
	}
}

func TestIntegration_MarkOutboxEventPublished(t *testing.T) {
	db := testDB(t)
	repo := mariadb.NewTransactionRepository(db)
	ctx := context.Background()

	tx, event := validTransaction(t)
	if err := repo.Create(ctx, tx, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	publishedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := repo.MarkOutboxEventPublished(ctx, event.ID, publishedAt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A published event must no longer appear among pending events.
	pending, err := repo.FetchPendingOutboxEvents(ctx, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ev := range pending {
		if ev.ID == event.ID {
			t.Error("expected the published event to no longer be pending")
		}
	}

	// Marking it published again must not succeed (already published).
	err = repo.MarkOutboxEventPublished(ctx, event.ID, time.Now().UTC())
	if err == nil {
		t.Error("expected an error marking an already-published event as published again")
	}
}
