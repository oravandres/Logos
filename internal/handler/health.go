package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler exposes a health-check endpoint backed by a database ping.
type HealthHandler struct {
	Pool *pgxpool.Pool
}

// Check pings the database and reports healthy/unhealthy status.
func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	if err := h.Pool.Ping(r.Context()); err != nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}
