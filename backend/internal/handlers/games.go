package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

type GameFilters = models.GameFilters

func (h *Handler) GetGames(w http.ResponseWriter, r *http.Request) {
	limit := 20
	offset := 0
	filters := models.GameFilters{}
	filters.Search = r.URL.Query().Get("search")
	filters.Genre = r.URL.Query().Get("genre")
	filters.Developer = r.URL.Query().Get("developer")
	filters.Publisher = r.URL.Query().Get("publisher")
	filters.Genres = splitFilterValues(r.URL.Query().Get("genres"))
	filters.Tags = splitFilterValues(r.URL.Query().Get("tags"))
	filters.Languages = splitFilterValues(r.URL.Query().Get("languages"))

	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			http.Error(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
		limit = parsed
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			http.Error(w, "Invalid offset parameter", http.StatusBadRequest)
			return
		}
		offset = parsed
	}
	if value := r.URL.Query().Get("minPrice"); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed < 0 {
			http.Error(w, "Invalid minPrice parameter", http.StatusBadRequest)
			return
		}
		filters.MinPrice = parsed
	}
	if value := r.URL.Query().Get("maxPrice"); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed < 0 {
			http.Error(w, "Invalid maxPrice parameter", http.StatusBadRequest)
			return
		}
		filters.MaxPrice = parsed
	}
	if filters.MaxPrice > 0 && filters.MinPrice > filters.MaxPrice {
		http.Error(w, "minPrice cannot exceed maxPrice", http.StatusBadRequest)
		return
	}
	filters.Limit = limit
	filters.Offset = offset

	games, err := h.DB.GetGames(r.Context(), filters)
	if err != nil {
		http.Error(w, "Failed to retrieve games", http.StatusInternalServerError)
		return
	}
	response := GameResponse{
		Games:  games,
		Total:  len(games),
		Limit:  limit,
		Offset: offset,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode games", http.StatusInternalServerError)
		return
	}
}

func splitFilterValues(value string) []string {
	if value == "" {
		return nil
	}

	values := strings.Split(value, ",")
	filtered := make([]string, 0, len(values))
	for _, item := range values {
		if item = strings.TrimSpace(item); item != "" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (h *Handler) GetGame(w http.ResponseWriter, r *http.Request) {
	appIDStr := r.URL.Query().Get("appID")

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		http.Error(w, "Invalid appID parameter", http.StatusBadRequest)
		return
	}

	game, err := h.DB.GetGame(r.Context(), appID)
	if err != nil {
		http.Error(w, "Failed to retrieve game", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(game); err != nil {
		http.Error(w, "Failed to encode game", http.StatusInternalServerError)
		return
	}
}
