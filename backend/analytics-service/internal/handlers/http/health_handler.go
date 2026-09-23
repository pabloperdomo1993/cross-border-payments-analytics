package http

import (
	"context"
	"net/http"
	"time"
)

const readinessPingTimeout = 2 * time.Second

// Pinger is the subset of the repository the readiness check depends on.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler serves the liveness (/health) and readiness (/ready)
// endpoints.
type HealthHandler struct {
	pinger Pinger
}

// NewHealthHandler builds a HealthHandler. pinger is used only by Ready.
func NewHealthHandler(pinger Pinger) *HealthHandler {
	return &HealthHandler{pinger: pinger}
}

// Health reports whether the process itself is alive. It never touches
// ClickHouse, so it stays accurate even when ClickHouse is unreachable.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready reports whether ClickHouse is reachable.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessPingTimeout)
	defer cancel()

	if err := h.pinger.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
