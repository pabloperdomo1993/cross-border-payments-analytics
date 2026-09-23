package http

import (
	"context"
	"net/http"
	"time"
)

// readinessPingTimeout bounds how long the readiness check waits on the
// database before reporting not-ready, so a slow/unreachable DB can't
// hang the /ready endpoint.
const readinessPingTimeout = 2 * time.Second

// Pinger is the subset of *sql.DB the readiness check depends on,
// declared here so tests can substitute a fake instead of a real
// database connection.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// HealthHandler serves the liveness (/health) and readiness (/ready)
// endpoints.
type HealthHandler struct {
	db Pinger
}

// NewHealthHandler builds a HealthHandler. db is used only by Ready.
func NewHealthHandler(db Pinger) *HealthHandler {
	return &HealthHandler{db: db}
}

// Health reports whether the process itself is alive. It never touches
// the database, so it stays accurate even when MariaDB is unreachable.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready reports whether the service's dependencies (currently just
// MariaDB) are reachable.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessPingTimeout)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
