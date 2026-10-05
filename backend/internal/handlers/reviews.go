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
		writeError(w, http.StatusBadRequest, "Missing appID parameter")
		return
	}
	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid appID parameter")
		return
	}
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit := 20
	offset := 0
	if limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			writeError(w, http.StatusBadRequest, "Invalid limit parameter")
			return
		}
	}
	if offsetStr != "" {
		offset, err = strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			writeError(w, http.StatusBadRequest, "Invalid offset parameter")
			return
		}
	}
	reviews, err := h.DB.GetReviews(context.Background(), appID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to retrieve reviews")
		return
	}
	total, err := h.DB.GetCountOfReviews(context.Background(), appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to count reviews")
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
		writeError(w, http.StatusInternalServerError, "Failed to encode reviews")
		return
	}
}
