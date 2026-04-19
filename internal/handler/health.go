package handler

import (
	"context"
	"log/slog"
	"net/http"
)

// Pinger checks whether a dependency is reachable.
type Pinger interface {
	Ping(context.Context) error
}

// HealthHandler exposes liveness and readiness endpoints.
type HealthHandler struct {
	Pinger Pinger
}

// Live reports whether the process is running.
func (h *HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready reports whether the service can safely receive traffic. Wired at /readyz.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	h.writeReadiness(w, r, "ready", "unready")
}

// Compat preserves the original /api/v1/health body shape ("healthy" / "unhealthy")
// for downstream consumers (LogosUI dashboard, monitoring) that depend on the
// pre-existing label. New code should use /readyz, which uses ready/unready.
func (h *HealthHandler) Compat(w http.ResponseWriter, r *http.Request) {
	h.writeReadiness(w, r, "healthy", "unhealthy")
}

// writeReadiness runs the dependency check and writes the response with the
// supplied okLabel / notOkLabel so the same logic can serve /readyz and the
// legacy /api/v1/health path with their respective body shapes.
func (h *HealthHandler) writeReadiness(w http.ResponseWriter, r *http.Request, okLabel, notOkLabel string) {
	if h.Pinger == nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"status": notOkLabel})
		return
	}

	if err := h.Pinger.Ping(r.Context()); err != nil {
		slog.Warn("readiness check failed", "error", err)
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"status": notOkLabel})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": okLabel})
}
