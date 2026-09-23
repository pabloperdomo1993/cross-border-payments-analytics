package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/application"
	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

// CreateTransactionUseCase is the subset of application.CreateTransaction
// the handler depends on. Declaring it here (rather than importing the
// concrete type everywhere) keeps the handler easy to test with a
// lightweight fake instead of a mocking framework.
type CreateTransactionUseCase interface {
	Execute(ctx context.Context, input application.CreateTransactionInput) (*domain.Transaction, error)
}

// GetTransactionUseCase is the subset of application.GetTransaction the
// handler depends on.
type GetTransactionUseCase interface {
	Execute(ctx context.Context, id string) (*domain.Transaction, error)
}

// TransactionHandler serves the /api/v1/transactions endpoints.
type TransactionHandler struct {
	create CreateTransactionUseCase
	get    GetTransactionUseCase
	logger *slog.Logger
}

// NewTransactionHandler builds a TransactionHandler.
func NewTransactionHandler(create CreateTransactionUseCase, get GetTransactionUseCase, logger *slog.Logger) *TransactionHandler {
	return &TransactionHandler{create: create, get: get, logger: logger}
}

type createTransactionRequest struct {
	SourceCountry       string `json:"source_country"`
	DestinationCountry  string `json:"destination_country"`
	SourceCurrency      string `json:"source_currency"`
	DestinationCurrency string `json:"destination_currency"`
	Amount              string `json:"amount"`
	FXRate              string `json:"fx_rate"`
	Provider            string `json:"provider"`
}

// transactionResponse mirrors domain.Transaction for JSON output.
// Amount and FXRate are serialized as decimal strings, matching the
// request format, so clients never have to deal with binary
// floating-point precision issues either.
type transactionResponse struct {
	ID                  string `json:"id"`
	SourceCountry       string `json:"source_country"`
	DestinationCountry  string `json:"destination_country"`
	SourceCurrency      string `json:"source_currency"`
	DestinationCurrency string `json:"destination_currency"`
	Amount              string `json:"amount"`
	FXRate              string `json:"fx_rate"`
	Provider            string `json:"provider"`
	Status              string `json:"status"`
	CreatedAt           string `json:"created_at"`
}

func toTransactionResponse(tx *domain.Transaction) transactionResponse {
	return transactionResponse{
		ID:                  string(tx.ID),
		SourceCountry:       string(tx.SourceCountry),
		DestinationCountry:  string(tx.DestinationCountry),
		SourceCurrency:      string(tx.SourceCurrency),
		DestinationCurrency: string(tx.DestinationCurrency),
		Amount:              tx.Amount.String(),
		FXRate:              tx.FXRate.String(),
		Provider:            string(tx.Provider),
		Status:              string(tx.Status),
		CreatedAt:           tx.CreatedAt.Format(time.RFC3339),
	}
}

// Create handles POST /api/v1/transactions.
func (h *TransactionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r.Context(), h.logger, &domain.ValidationError{
			Fields: map[string]string{"body": "must be valid JSON"},
		})
		return
	}

	tx, err := h.create.Execute(r.Context(), application.CreateTransactionInput{
		SourceCountry:       req.SourceCountry,
		DestinationCountry:  req.DestinationCountry,
		SourceCurrency:      req.SourceCurrency,
		DestinationCurrency: req.DestinationCurrency,
		Amount:              req.Amount,
		FXRate:              req.FXRate,
		Provider:            req.Provider,
	})
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}

	h.logger.InfoContext(r.Context(), "transaction_created",
		slog.String("transaction_id", string(tx.ID)),
		slog.String("correlation_id", CorrelationID(r.Context())),
	)
	writeJSON(w, http.StatusCreated, toTransactionResponse(tx))
}

// Get handles GET /api/v1/transactions/{id}.
func (h *TransactionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	tx, err := h.get.Execute(r.Context(), id)
	if err != nil {
		writeError(w, r.Context(), h.logger, err)
		return
	}

	writeJSON(w, http.StatusOK, toTransactionResponse(tx))
}
