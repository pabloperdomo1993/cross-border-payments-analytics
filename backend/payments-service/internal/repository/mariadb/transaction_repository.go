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

// Create persists a new transaction. Monetary fields and FXRate are
// written as their canonical decimal strings so the DECIMAL columns
// always receive an exact value — no float64 conversion ever happens.
func (r *TransactionRepository) Create(ctx context.Context, tx *domain.Transaction) error {
	_, err := r.db.ExecContext(ctx, insertTransactionQuery,
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
