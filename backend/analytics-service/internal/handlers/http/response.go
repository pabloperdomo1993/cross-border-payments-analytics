package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/domain"
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

// writeError maps an error to a consistent JSON error response and the
// appropriate HTTP status code, matching the envelope shape used by
// payments-service. Unexpected errors are logged with full detail
// server-side but never exposed to the client.
func writeError(w http.ResponseWriter, ctx context.Context, logger *slog.Logger, err error) {
	if verr, ok := domain.AsValidationError(err); ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: errorBody{
			Code:    "VALIDATION_ERROR",
			Message: "invalid query parameters",
			Fields:  verr.Fields,
		}})
		return
	}

	logger.ErrorContext(ctx, "internal_error", slog.String("error", err.Error()))
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: errorBody{
		Code:    "INTERNAL_ERROR",
		Message: "internal server error",
	}})
}
