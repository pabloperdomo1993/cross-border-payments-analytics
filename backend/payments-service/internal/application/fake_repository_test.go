package application_test

import (
	"context"
	"errors"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

// fakeRepository is a minimal in-memory implementation of
// repository.TransactionRepository used to test the application layer
// in isolation, without a real database or a mocking framework.
type fakeRepository struct {
	transactions map[string]*domain.Transaction
	createErr    error
	findErr      error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{transactions: map[string]*domain.Transaction{}}
}

func (f *fakeRepository) Create(_ context.Context, tx *domain.Transaction) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.transactions[string(tx.ID)] = tx
	return nil
}

func (f *fakeRepository) FindByID(_ context.Context, id string) (*domain.Transaction, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	tx, ok := f.transactions[id]
	if !ok {
		return nil, domain.ErrTransactionNotFound
	}
	return tx, nil
}

var errRepositoryFailure = errors.New("simulated repository failure")
