package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

// fakeRepository is a minimal in-memory implementation of
// repository.TransactionRepository used to test the application layer
// in isolation, without a real database or a mocking framework.
type fakeRepository struct {
	transactions map[string]*domain.Transaction
	outboxEvents map[string]*domain.OutboxEvent
	createErr    error
	findErr      error

	// lastEvent captures the OutboxEvent passed to the most recent
	// Create call, so tests can assert on it directly.
	lastEvent *domain.OutboxEvent
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		transactions: map[string]*domain.Transaction{},
		outboxEvents: map[string]*domain.OutboxEvent{},
	}
}

func (f *fakeRepository) Create(ctx context.Context, tx *domain.Transaction, event *domain.OutboxEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.lastEvent = event
	if f.createErr != nil {
		return f.createErr
	}
	f.transactions[string(tx.ID)] = tx
	if event != nil {
		f.outboxEvents[event.ID] = event
	}
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

func (f *fakeRepository) UpdateStatus(_ context.Context, id string, status domain.TransactionStatus, updatedAt time.Time) error {
	tx, ok := f.transactions[id]
	if !ok {
		return domain.ErrTransactionNotFound
	}
	tx.Status = status
	tx.UpdatedAt = updatedAt
	return nil
}

func (f *fakeRepository) FetchPendingOutboxEvents(_ context.Context, limit int) ([]*domain.OutboxEvent, error) {
	var pending []*domain.OutboxEvent
	for _, ev := range f.outboxEvents {
		if ev.Status == domain.OutboxStatusPending {
			pending = append(pending, ev)
		}
		if len(pending) >= limit {
			break
		}
	}
	return pending, nil
}

func (f *fakeRepository) MarkOutboxEventPublished(_ context.Context, id string, publishedAt time.Time) error {
	ev, ok := f.outboxEvents[id]
	if !ok {
		return domain.ErrRepository
	}
	return ev.MarkPublished(publishedAt)
}

var errRepositoryFailure = errors.New("simulated repository failure")
