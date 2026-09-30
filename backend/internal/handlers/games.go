package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/redis/go-redis/v9"
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
	filters.Developers = splitFilterValues(r.URL.Query().Get("developers"))
	filters.Publishers = splitFilterValues(r.URL.Query().Get("publishers"))

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
	filters.Limit = min(limit, maxGamesLimit)
	filters.Offset = offset

	cacheKey := h.searchCacheKey(r.Context(), filters)
	if cached, err := h.Redis.Get(r.Context(), cacheKey).Bytes(); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write(cached)
		return
	}

	games, err := h.DB.GetGames(r.Context(), filters)
	if err != nil {
		http.Error(w, "Failed to retrieve games", http.StatusInternalServerError)
		return
	}
	response := GameResponse{
		Games:  games,
		Total:  len(games),
		Limit:  filters.Limit,
		Offset: offset,
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode games", http.StatusInternalServerError)
		return
	}
	if len(encoded) <= maxCachedSearchBytes {
		h.Redis.Set(r.Context(), cacheKey, encoded, searchCacheTTL)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(encoded)
}

const (
	maxGamesLimit        = 100
	searchCacheTTL       = 10 * time.Minute
	maxCachedSearchBytes = 64 << 10
	searchVersionKey     = "search:version"
)

// searchCacheKey namespaces cached results under a version counter that's
// bumped whenever game data changes (see BumpSearchVersion), so a scrape
// invalidates every cached search at once without scanning keys - the old
// entries are just never read again and expire on their TTL.
func (h *Handler) searchCacheKey(ctx context.Context, filters models.GameFilters) string {
	version, err := h.Redis.Get(ctx, searchVersionKey).Result()
	if err != nil {
		version = "0"
	}
	// Filter order doesn't change results, so normalise it out of the key.
	for _, values := range [][]string{filters.Genres, filters.Tags, filters.Languages, filters.Developers, filters.Publishers} {
		slices.Sort(values)
	}
	encoded, _ := json.Marshal(filters)
	sum := sha256.Sum256(encoded)
	return "search:v" + version + ":" + hex.EncodeToString(sum[:])
}

// BumpSearchVersion invalidates all cached search results.
func BumpSearchVersion(ctx context.Context, rdb *redis.Client) {
	rdb.Incr(ctx, searchVersionKey)
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
	appIDStr := r.PathValue("appID")

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

func (h *Handler) GetPriceHistory(w http.ResponseWriter, r *http.Request) {
	appIDStr := r.PathValue("appID")

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		http.Error(w, "Invalid appID parameter", http.StatusBadRequest)
		return
	}

	history, err := h.DB.GetPriceHistory(r.Context(), appID)
	if err != nil {
		http.Error(w, "Failed to retrieve price history", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(history); err != nil {
		http.Error(w, "Failed to encode price history", http.StatusInternalServerError)
		return
	}
}
