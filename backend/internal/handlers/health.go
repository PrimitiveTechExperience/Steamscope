package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
)

// Health is the liveness probe: it only says the process is up, so an
// orchestrator doesn't restart the API just because the database blipped.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"version":        observability.Version,
		"uptime_seconds": int(observability.Uptime().Seconds()),
	})
}

// Ready is the readiness probe: it checks the dependencies the API cannot
// work without and returns 503 naming whichever failed.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{"database": "ok", "redis": "ok"}
	status := http.StatusOK
	if err := h.DB.Ping(ctx); err != nil {
		checks["database"] = "unavailable"
		status = http.StatusServiceUnavailable
	}
	if err := h.Redis.Ping(ctx).Err(); err != nil {
		checks["redis"] = "unavailable"
		status = http.StatusServiceUnavailable
	}
	overall := "ok"
	if status != http.StatusOK {
		overall = "unavailable"
	}
	writeJSON(w, status, map[string]any{"status": overall, "checks": checks, "version": observability.Version})
}
