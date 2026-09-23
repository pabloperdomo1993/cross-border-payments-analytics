package application

import (
	"context"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/repository"
)

// GetTransaction is the use case for retrieving a single transaction by
// ID.
type GetTransaction struct {
	repo repository.TransactionRepository
}

// NewGetTransaction builds a GetTransaction use case backed by repo.
func NewGetTransaction(repo repository.TransactionRepository) *GetTransaction {
	return &GetTransaction{repo: repo}
}

// Execute validates id and retrieves the matching Transaction. It
// returns an error wrapping domain.ErrTransactionNotFound when no
// transaction exists for id.
func (uc *GetTransaction) Execute(ctx context.Context, id string) (*domain.Transaction, error) {
	if err := domain.TransactionID(id).Validate(); err != nil {
		return nil, err
	}

	return uc.repo.FindByID(ctx, id)
}
