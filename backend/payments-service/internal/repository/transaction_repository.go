// Package repository defines persistence abstractions for the payments
// service. The domain and application layers depend only on the
// interface declared here — never on a concrete storage technology.
package repository

import (
	"context"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

// TransactionRepository persists and retrieves Transactions, and owns
// the transactional outbox that accompanies them.
//
// Implementations must return domain.ErrTransactionNotFound (or an error
// wrapping it) from FindByID when no transaction matches the given id,
// and should wrap unexpected storage failures with domain.ErrRepository
// rather than leaking driver-specific error types.
type TransactionRepository interface {
	// Create persists transaction and event atomically: either both
	// are written, or neither is (see the MariaDB implementation for
	// the BEGIN/INSERT/INSERT/COMMIT this performs). event may be nil
	// for callers that don't need an outbox entry (none currently do,
	// but the interface doesn't force one).
	Create(ctx context.Context, transaction *domain.Transaction, event *domain.OutboxEvent) error

	FindByID(ctx context.Context, id string) (*domain.Transaction, error)

	// UpdateStatus persists a transaction's Status/UpdatedAt after a
	// domain-level transition (Transaction.MarkCompleted/MarkFailed).
	UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, updatedAt time.Time) error

	// FetchPendingOutboxEvents returns up to limit pending events,
	// oldest first, for the relay to publish. This targets a single
	// relay instance (this project's actual deployment shape) — it
	// does not implement row-claiming for safe concurrent multi
	// -instance polling, which would need an atomic claim (not just a
	// SELECT) and isn't needed here.
	FetchPendingOutboxEvents(ctx context.Context, limit int) ([]*domain.OutboxEvent, error)

	// MarkOutboxEventPublished records that id was successfully
	// published, so it's excluded from future FetchPendingOutboxEvents
	// calls.
	MarkOutboxEventPublished(ctx context.Context, id string, publishedAt time.Time) error
}
