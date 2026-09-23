package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/application"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

func TestGetTransaction_Found(t *testing.T) {
	repo := newFakeRepository()
	created, err := application.NewCreateTransaction(repo).Execute(context.Background(), validInput())
	if err != nil {
		t.Fatalf("unexpected error seeding transaction: %v", err)
	}

	uc := application.NewGetTransaction(repo)
	got, err := uc.Execute(context.Background(), string(created.ID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected transaction %q, got %q", created.ID, got.ID)
	}
}

func TestGetTransaction_NotFound(t *testing.T) {
	repo := newFakeRepository()
	uc := application.NewGetTransaction(repo)

	_, err := uc.Execute(context.Background(), "550e8400-e29b-41d4-a716-446655440000")
	if !errors.Is(err, domain.ErrTransactionNotFound) {
		t.Errorf("expected ErrTransactionNotFound, got %v", err)
	}
}

func TestGetTransaction_InvalidID(t *testing.T) {
	repo := newFakeRepository()
	uc := application.NewGetTransaction(repo)

	_, err := uc.Execute(context.Background(), "not-a-uuid")
	if _, ok := domain.AsValidationError(err); !ok {
		t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
	}
}

func TestGetTransaction_RepositoryFailure(t *testing.T) {
	repo := newFakeRepository()
	repo.findErr = errRepositoryFailure
	uc := application.NewGetTransaction(repo)

	_, err := uc.Execute(context.Background(), "550e8400-e29b-41d4-a716-446655440000")
	if !errors.Is(err, errRepositoryFailure) {
		t.Errorf("expected error to wrap repository failure, got %v", err)
	}
}
