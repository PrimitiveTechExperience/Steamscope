package handlers

import (
	"encoding/json"
	"net/http"
)

func (h *Handler) GetFilterOptions(w http.ResponseWriter, r *http.Request) {
	options, err := h.DB.GetFilterOptions(r.Context())
	if err != nil {
		http.Error(w, "Failed to retrieve filter options", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(options); err != nil {
		http.Error(w, "Failed to encode filter options", http.StatusInternalServerError)
		return
	}
}
