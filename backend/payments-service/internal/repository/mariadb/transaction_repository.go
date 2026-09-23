// Package mariadb implements repository.TransactionRepository against a
// MariaDB/MySQL database using database/sql. No ORM is used: queries are
// plain, parameterized SQL executed through context-aware calls.
package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

// TransactionRepository is a MariaDB-backed implementation of
// repository.TransactionRepository.
type TransactionRepository struct {
	db *sql.DB
}

// NewTransactionRepository builds a TransactionRepository backed by db.
func NewTransactionRepository(db *sql.DB) *TransactionRepository {
	return &TransactionRepository{db: db}
}

const insertTransactionQuery = `
INSERT INTO transactions (
	id, idempotency_key, source_country, destination_country,
	source_currency, destination_currency,
	source_amount, destination_amount, fx_rate, provider, status,
	created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

const insertOutboxEventQuery = `
INSERT INTO outbox_events (
	id, aggregate_id, event_type, payload, status, created_at
) VALUES (?, ?, ?, ?, ?, ?)
`

// Create persists a new transaction and, if event is non-nil, its
// outbox event, atomically: both inserts happen inside a single SQL
// transaction, so if the outbox insert fails, the payment insert is
// rolled back with it — a payment can never exist without a
// corresponding queued event. Monetary fields and FXRate are written
// as their canonical decimal strings so the DECIMAL columns always
// receive an exact value — no float64 conversion ever happens.
func (r *TransactionRepository) Create(ctx context.Context, tx *domain.Transaction, event *domain.OutboxEvent) error {
	sqlTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: begin transaction: %v", domain.ErrRepository, err)
	}
	// If Commit succeeds below, this Rollback becomes a harmless no-op
	// (sql.ErrTxDone, discarded) — it only actually rolls back when we
	// return early due to an error.
	defer sqlTx.Rollback() //nolint:errcheck

	_, err = sqlTx.ExecContext(ctx, insertTransactionQuery,
		string(tx.ID),
		string(tx.IdempotencyKey),
		string(tx.SourceCountry),
		string(tx.DestinationCountry),
		string(tx.SourceCurrency),
		string(tx.DestinationCurrency),
		tx.SourceAmount.String(),
		tx.DestinationAmount.String(),
		tx.FXRate.String(),
		string(tx.Provider),
		string(tx.Status),
		tx.CreatedAt,
		tx.UpdatedAt,
	)
	if err != nil {
		if isDuplicateKeyErr(err) {
			return fmt.Errorf("%w: transaction %q already exists", domain.ErrConflict, tx.ID)
		}
		return fmt.Errorf("%w: create transaction: %v", domain.ErrRepository, err)
	}

	if event != nil {
		_, err = sqlTx.ExecContext(ctx, insertOutboxEventQuery,
			event.ID, event.AggregateID, event.EventType, event.Payload,
			string(event.Status), event.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("%w: create outbox event: %v", domain.ErrRepository, err)
		}
	}

	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("%w: commit transaction: %v", domain.ErrRepository, err)
	}
	return nil
}

const findTransactionByIDQuery = `
SELECT id, idempotency_key, source_country, destination_country,
       source_currency, destination_currency,
       source_amount, destination_amount, fx_rate, provider, status,
       created_at, updated_at
FROM transactions
WHERE id = ?
`

// FindByID retrieves a transaction by ID. It returns
// domain.ErrTransactionNotFound if no row matches.
func (r *TransactionRepository) FindByID(ctx context.Context, id string) (*domain.Transaction, error) {
	row := r.db.QueryRowContext(ctx, findTransactionByIDQuery, id)

	var (
		txID, idempotencyKey, sourceCountry, destinationCountry string
		sourceCurrency, destinationCurrency                     string
		sourceAmount, destinationAmount, fxRate                 string
		provider, status                                        string
		createdAt, updatedAt                                    time.Time
	)

	err := row.Scan(
		&txID, &idempotencyKey, &sourceCountry, &destinationCountry,
		&sourceCurrency, &destinationCurrency,
		&sourceAmount, &destinationAmount, &fxRate, &provider, &status,
		&createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrTransactionNotFound
		}
		return nil, fmt.Errorf("%w: find transaction: %v", domain.ErrRepository, err)
	}

	parsedSourceAmount, err := domain.ParseMoney(sourceAmount)
	if err != nil {
		return nil, fmt.Errorf("%w: corrupt source_amount for transaction %q: %v", domain.ErrRepository, txID, err)
	}
	parsedDestinationAmount, err := domain.ParseMoney(destinationAmount)
	if err != nil {
		return nil, fmt.Errorf("%w: corrupt destination_amount for transaction %q: %v", domain.ErrRepository, txID, err)
	}
	parsedFXRate, err := domain.ParseFXRate(fxRate)
	if err != nil {
		return nil, fmt.Errorf("%w: corrupt fx_rate for transaction %q: %v", domain.ErrRepository, txID, err)
	}

	return &domain.Transaction{
		ID:                  domain.TransactionID(txID),
		IdempotencyKey:      domain.IdempotencyKey(idempotencyKey),
		SourceCountry:       domain.CountryCode(sourceCountry),
		DestinationCountry:  domain.CountryCode(destinationCountry),
		SourceCurrency:      domain.CurrencyCode(sourceCurrency),
		DestinationCurrency: domain.CurrencyCode(destinationCurrency),
		SourceAmount:        parsedSourceAmount,
		DestinationAmount:   parsedDestinationAmount,
		FXRate:              parsedFXRate,
		Provider:            domain.Provider(provider),
		Status:              domain.TransactionStatus(status),
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}, nil
}

const updateStatusQuery = `UPDATE transactions SET status = ?, updated_at = ? WHERE id = ?`

// UpdateStatus persists a transaction's status/updated_at. It returns
// domain.ErrTransactionNotFound if id doesn't exist.
func (r *TransactionRepository) UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, updatedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, updateStatusQuery, string(status), updatedAt, id)
	if err != nil {
		return fmt.Errorf("%w: update status: %v", domain.ErrRepository, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%w: update status rows affected: %v", domain.ErrRepository, err)
	}
	if rows == 0 {
		return domain.ErrTransactionNotFound
	}
	return nil
}

const fetchPendingOutboxEventsQuery = `
SELECT id, aggregate_id, event_type, payload, status, created_at, published_at
FROM outbox_events
WHERE status = ?
ORDER BY created_at
LIMIT ?
`

// FetchPendingOutboxEvents returns up to limit pending events, oldest
// first. This implementation targets a single relay instance (this
// project's actual deployment shape, see internal/outbox) — it does not
// attempt to "claim" rows for safe concurrent multi-instance polling
// (that would need row-claiming via an atomic status transition in the
// same query, not just a plain SELECT), which would be unused
// complexity here.
func (r *TransactionRepository) FetchPendingOutboxEvents(ctx context.Context, limit int) ([]*domain.OutboxEvent, error) {
	rows, err := r.db.QueryContext(ctx, fetchPendingOutboxEventsQuery, string(domain.OutboxStatusPending), limit)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch pending outbox events: %v", domain.ErrRepository, err)
	}
	defer rows.Close()

	events := []*domain.OutboxEvent{}
	for rows.Next() {
		var (
			id, aggregateID, eventType, payload, status string
			createdAt                                   time.Time
			publishedAt                                 sql.NullTime
		)
		if err := rows.Scan(&id, &aggregateID, &eventType, &payload, &status, &createdAt, &publishedAt); err != nil {
			return nil, fmt.Errorf("%w: scan outbox event: %v", domain.ErrRepository, err)
		}
		ev := &domain.OutboxEvent{
			ID:          id,
			AggregateID: aggregateID,
			EventType:   eventType,
			Payload:     payload,
			Status:      domain.OutboxEventStatus(status),
			CreatedAt:   createdAt,
		}
		if publishedAt.Valid {
			t := publishedAt.Time
			ev.PublishedAt = &t
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: outbox events rows: %v", domain.ErrRepository, err)
	}
	return events, nil
}

const markOutboxEventPublishedQuery = `
UPDATE outbox_events SET status = ?, published_at = ? WHERE id = ? AND status = ?
`

// MarkOutboxEventPublished records id as published. The WHERE clause
// only matches a currently-pending row, so calling this twice for the
// same id (e.g. a retried relay tick racing itself) is safe: the second
// call simply matches zero rows rather than corrupting published_at.
func (r *TransactionRepository) MarkOutboxEventPublished(ctx context.Context, id string, publishedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, markOutboxEventPublishedQuery,
		string(domain.OutboxStatusPublished), publishedAt, id, string(domain.OutboxStatusPending),
	)
	if err != nil {
		return fmt.Errorf("%w: mark outbox event published: %v", domain.ErrRepository, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%w: mark outbox event published rows affected: %v", domain.ErrRepository, err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: outbox event %q not found or already published", domain.ErrRepository, id)
	}
	return nil
}

// duplicateKeyErrNumber is the MySQL/MariaDB error number for a
// duplicate primary/unique key violation (ER_DUP_ENTRY).
const duplicateKeyErrNumber = 1062

// isDuplicateKeyErr reports whether err is a MariaDB/MySQL duplicate-key
// violation, keeping the driver-specific error type contained to this
// file rather than leaking it to callers.
func isDuplicateKeyErr(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateKeyErrNumber
}
