// Package repository defines persistence abstractions for the payments
// service. The domain and application layers depend only on the
// interface declared here — never on a concrete storage technology.
package repository

import (
	"context"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

// TransactionRepository persists and retrieves Transactions.
//
// Implementations must return domain.ErrTransactionNotFound (or an error
// wrapping it) from FindByID when no transaction matches the given id,
// and should wrap unexpected storage failures with domain.ErrRepository
// rather than leaking driver-specific error types.
type TransactionRepository interface {
	Create(ctx context.Context, transaction *domain.Transaction) error
	FindByID(ctx context.Context, id string) (*domain.Transaction, error)
}
