package application

import (
	"context"
	"fmt"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/repository"
)

// CreateTransactionInput carries the raw, string-typed input for
// creating a transaction (as received from the HTTP layer, before any
// domain parsing/validation happens).
type CreateTransactionInput struct {
	SourceCountry       string
	DestinationCountry  string
	SourceCurrency      string
	DestinationCurrency string
	Amount              string
	FXRate              string
	Provider            string
}

// CreateTransaction is the use case for registering a new cross-border
// payment. It owns input validation, ID generation, and persistence —
// no SQL or HTTP concerns live here.
type CreateTransaction struct {
	repo repository.TransactionRepository
}

// NewCreateTransaction builds a CreateTransaction use case backed by repo.
func NewCreateTransaction(repo repository.TransactionRepository) *CreateTransaction {
	return &CreateTransaction{repo: repo}
}

// Execute validates input, builds a new pending Transaction, and
// persists it. On success it returns the created Transaction.
func (uc *CreateTransaction) Execute(ctx context.Context, input CreateTransactionInput) (*domain.Transaction, error) {
	fields := map[string]string{}

	amount, err := domain.ParseMoney(input.Amount)
	if err != nil {
		fields["amount"] = err.Error()
	}

	fxRate, err := domain.ParseFXRate(input.FXRate)
	if err != nil {
		fields["fx_rate"] = err.Error()
	}

	if len(fields) > 0 {
		return nil, &domain.ValidationError{Fields: fields}
	}

	id, err := NewTransactionID()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrRepository, err)
	}

	tx, err := domain.NewTransaction(domain.NewTransactionParams{
		ID:                  domain.TransactionID(id),
		SourceCountry:       domain.CountryCode(input.SourceCountry),
		DestinationCountry:  domain.CountryCode(input.DestinationCountry),
		SourceCurrency:      domain.CurrencyCode(input.SourceCurrency),
		DestinationCurrency: domain.CurrencyCode(input.DestinationCurrency),
		Amount:              amount,
		FXRate:              fxRate,
		Provider:            domain.Provider(input.Provider),
	})
	if err != nil {
		return nil, err
	}

	if err := uc.repo.Create(ctx, tx); err != nil {
		return nil, err
	}

	return tx, nil
}
