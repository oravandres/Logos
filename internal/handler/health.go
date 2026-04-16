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

// Ready reports whether the service can safely receive traffic.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	if h.Pinger == nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unready"})
		return
	}

	if err := h.Pinger.Ping(r.Context()); err != nil {
		slog.Warn("readiness check failed", "error", err)
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unready",
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
