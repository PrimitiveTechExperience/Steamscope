package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/sanitize"
	"github.com/redis/go-redis/v9"
)

type GameFilters = models.GameFilters

func (h *Handler) GetGames(w http.ResponseWriter, r *http.Request) {
	limit := 20
	offset := 0
	filters := models.GameFilters{}
	filters.Search = sanitize.Text(r.URL.Query().Get("search"), maxFilterLength)
	filters.Genre = sanitize.Text(r.URL.Query().Get("genre"), maxFilterLength)
	filters.Developer = sanitize.Text(r.URL.Query().Get("developer"), maxFilterLength)
	filters.Publisher = sanitize.Text(r.URL.Query().Get("publisher"), maxFilterLength)
	filters.Genres = splitFilterValues(r.URL.Query().Get("genres"))
	filters.Tags = splitFilterValues(r.URL.Query().Get("tags"))
	filters.Languages = splitFilterValues(r.URL.Query().Get("languages"))
	filters.Developers = splitFilterValues(r.URL.Query().Get("developers"))
	filters.Publishers = splitFilterValues(r.URL.Query().Get("publishers"))

	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "Invalid limit parameter")
			return
		}
		limit = parsed
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "Invalid offset parameter")
			return
		}
		offset = parsed
	}
	if value := r.URL.Query().Get("minPrice"); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "Invalid minPrice parameter")
			return
		}
		filters.MinPrice = parsed
	}
	if value := r.URL.Query().Get("maxPrice"); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "Invalid maxPrice parameter")
			return
		}
		filters.MaxPrice = parsed
	}
	if filters.MaxPrice > 0 && filters.MinPrice > filters.MaxPrice {
		writeError(w, http.StatusBadRequest, "minPrice cannot exceed maxPrice")
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
		serverError(w, r, "get games", err, "Failed to retrieve games")
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
		serverError(w, r, "encode games", err, "Failed to encode games")
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
	maxFilterLength      = 100
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
		// Control characters (a NUL byte makes Postgres reject the query)
		// have no place in a filter value.
		if item = sanitize.Text(item, maxFilterLength); item != "" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (h *Handler) GetGame(w http.ResponseWriter, r *http.Request) {
	appIDStr := r.PathValue("appID")

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid appID parameter")
		return
	}

	game, err := h.DB.GetGame(r.Context(), appID)
	if errors.Is(err, database.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Game not found")
		return
	}
	if err != nil {
		serverError(w, r, "get game", err, "Failed to retrieve game")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(game); err != nil {
		serverError(w, r, "encode game", err, "Failed to encode game")
		return
	}
}

func (h *Handler) GetPriceHistory(w http.ResponseWriter, r *http.Request) {
	appIDStr := r.PathValue("appID")

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid appID parameter")
		return
	}

	history, err := h.DB.GetPriceHistory(r.Context(), appID)
	if err != nil {
		serverError(w, r, "get price history", err, "Failed to retrieve price history")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(history); err != nil {
		serverError(w, r, "encode price history", err, "Failed to encode price history")
		return
	}
}
