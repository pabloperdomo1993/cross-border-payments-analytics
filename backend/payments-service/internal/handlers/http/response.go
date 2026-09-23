package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payments-service/internal/domain"
)

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError maps a domain/application error to a consistent JSON error
// response and the appropriate HTTP status code. Unexpected errors are
// logged with full detail server-side but never exposed to the client,
// so database/internal failures never leak to API consumers.
func writeError(w http.ResponseWriter, ctx context.Context, logger *slog.Logger, err error) {
	if verr, ok := domain.AsValidationError(err); ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: errorBody{
			Code:    "VALIDATION_ERROR",
			Message: "invalid transaction",
			Fields:  verr.Fields,
		}})
		return
	}

	if errors.Is(err, domain.ErrTransactionNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: errorBody{
			Code:    "NOT_FOUND",
			Message: "transaction not found",
		}})
		return
	}

	if errors.Is(err, domain.ErrConflict) {
		writeJSON(w, http.StatusConflict, errorResponse{Error: errorBody{
			Code:    "CONFLICT",
			Message: "transaction already exists",
		}})
		return
	}

	logger.ErrorContext(ctx, "internal_error",
		slog.String("error", err.Error()),
		slog.String("correlation_id", CorrelationID(ctx)),
	)
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: errorBody{
		Code:    "INTERNAL_ERROR",
		Message: "internal server error",
	}})
}
