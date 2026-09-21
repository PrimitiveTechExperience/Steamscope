package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

func (h *Handler) GetReviews(w http.ResponseWriter, r *http.Request) {
	appIDStr := r.PathValue("appID")
	if appIDStr == "" {
		http.Error(w, "Missing appID parameter", http.StatusBadRequest)
		return
	}
	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		http.Error(w, "Invalid appID parameter", http.StatusBadRequest)
		return
	}
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit := 20
	offset := 0
	if limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			http.Error(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
	}
	if offsetStr != "" {
		offset, err = strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			http.Error(w, "Invalid offset parameter", http.StatusBadRequest)
			return
		}
	}
	reviews, err := h.DB.GetReviews(context.Background(), appID, limit, offset)
	if err != nil {
		http.Error(w, "Failed to retrieve reviews", http.StatusInternalServerError)
		return
	}
	total, err := h.DB.GetCountOfReviews(context.Background(), appID)
	if err != nil {
		http.Error(w, "Failed to count reviews", http.StatusInternalServerError)
		return
	}
	response := ReviewResponse{
		Reviews: reviews,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode reviews", http.StatusInternalServerError)
		return
	}
}