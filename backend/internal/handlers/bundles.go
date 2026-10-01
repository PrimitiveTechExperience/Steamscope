package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
)

// GetBundles lists tracked bundles, biggest discount first. ?app_id=N limits
// it to bundles containing that game.
func (h *Handler) GetBundles(w http.ResponseWriter, r *http.Request) {
	appID := 0
	if v := r.URL.Query().Get("app_id"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "invalid app_id")
			return
		}
		appID = parsed
	}
	limit := 60
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = min(parsed, maxGamesLimit)
	}

	bundles, err := h.DB.GetBundles(r.Context(), appID, limit)
	if err != nil {
		log.Printf("get bundles: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load bundles")
		return
	}
	writeJSON(w, http.StatusOK, bundles)
}

func (h *Handler) GetBundle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("bundleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid bundle id")
		return
	}
	bundle, err := h.DB.GetBundle(r.Context(), id)
	if errors.Is(err, database.ErrNotFound) {
		writeError(w, http.StatusNotFound, "bundle not found")
		return
	}
	if err != nil {
		log.Printf("get bundle: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load bundle")
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}
