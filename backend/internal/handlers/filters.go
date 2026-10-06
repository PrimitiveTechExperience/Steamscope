package handlers

import (
	"encoding/json"
	"net/http"
)

func (h *Handler) GetFilterOptions(w http.ResponseWriter, r *http.Request) {
	options, err := h.DB.GetFilterOptions(r.Context())
	if err != nil {
		serverError(w, r, "get filter options", err, "Failed to retrieve filter options")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(options); err != nil {
		serverError(w, r, "encode filter options", err, "Failed to encode filter options")
		return
	}
}
