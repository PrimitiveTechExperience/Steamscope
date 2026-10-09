package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/sanitize"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
	"golang.org/x/sync/errgroup"
)

func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	prefs, err := h.DB.GetPreferences(r.Context(), user.UserID)
	if err != nil {
		serverError(w, r, "get preferences", err, "failed to load preferences")
		return
	}
	writeJSON(w, http.StatusOK, prefs)
}

func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	var prefs models.Preferences
	if !decodeJSON(w, r, &prefs) {
		return
	}
	if prefs.Theme != "light" && prefs.Theme != "dark" {
		writeError(w, http.StatusBadRequest, "theme must be light or dark")
		return
	}
	if prefs.PriceDropThresholdPercent < 1 || prefs.PriceDropThresholdPercent > 100 {
		writeError(w, http.StatusBadRequest, "price drop threshold must be between 1 and 100")
		return
	}
	prefs.DiscordWebhookURL = strings.TrimSpace(prefs.DiscordWebhookURL)
	current, err := h.DB.GetPreferences(r.Context(), user.UserID)
	if err != nil {
		serverError(w, r, "get preferences", err, "failed to save preferences")
		return
	}
	if msg, ok := h.validateAlertPreferences(r, user.UserID, prefs, current); !ok {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if len(prefs.PreferredGenres) > 20 {
		writeError(w, http.StatusBadRequest, "too many preferred genres")
		return
	}
	for i, genre := range prefs.PreferredGenres {
		prefs.PreferredGenres[i] = sanitize.Text(genre, 50)
		if prefs.PreferredGenres[i] == "" {
			writeError(w, http.StatusBadRequest, "invalid genre")
			return
		}
	}
	if err := h.DB.UpdatePreferences(r.Context(), user.UserID, prefs); err != nil {
		serverError(w, r, "update preferences", err, "failed to save preferences")
		return
	}
	writeJSON(w, http.StatusOK, prefs)
}

func (h *Handler) GetWatchlist(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	watchlist, err := h.DB.GetWatchlist(r.Context(), user.UserID)
	if err != nil {
		serverError(w, r, "get watchlist", err, "failed to load watchlist")
		return
	}
	writeJSON(w, http.StatusOK, watchlist)
}

type watchRequest struct {
	Pinned      bool     `json:"pinned"`
	TargetPrice *float64 `json:"target_price"`
}

func (h *Handler) WatchGame(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	appID, err := strconv.Atoi(r.PathValue("appID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid appID")
		return
	}
	var req watchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TargetPrice != nil && (*req.TargetPrice < 0 || *req.TargetPrice > 10000) {
		writeError(w, http.StatusBadRequest, "target price must be between 0 and 10000")
		return
	}
	err = h.DB.UpsertWatchedGame(r.Context(), user.UserID, appID, req.Pinned, req.TargetPrice)
	if errors.Is(err, database.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		serverError(w, r, "watch game", err, "failed to watch game")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UnwatchGame(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	appID, err := strconv.Atoi(r.PathValue("appID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid appID")
		return
	}
	if err := h.DB.DeleteWatchedGame(r.Context(), user.UserID, appID); err != nil {
		serverError(w, r, "unwatch game", err, "failed to unwatch game")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	notifications, err := h.DB.GetNotifications(r.Context(), user.UserID)
	if err != nil {
		serverError(w, r, "get notifications", err, "failed to load notifications")
		return
	}
	writeJSON(w, http.StatusOK, notifications)
}

type markReadRequest struct {
	IDs []int64 `json:"ids"`
}

func (h *Handler) MarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	var req markReadRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.DB.MarkNotificationsRead(r.Context(), user.UserID, req.IDs); err != nil {
		serverError(w, r, "mark notifications read", err, "failed to update notifications")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const steamProfileTTL = 10 * time.Minute

func (h *Handler) GetSteamProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	if user.SteamID == nil {
		writeJSON(w, http.StatusOK, map[string]any{"profile": nil})
		return
	}
	profile, err := h.steamProfile(r, *user.SteamID)
	if err == nil {
		h.fillTrackStatuses(r, profile)
	}
	if errors.Is(err, steam.ErrNoAPIKey) {
		writeError(w, http.StatusServiceUnavailable, "steam profiles aren't configured on this server")
		return
	}
	if err != nil {
		log.Printf("steam profile: %v", err)
		writeError(w, http.StatusBadGateway, "couldn't reach steam right now")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": profile})
}

// steamProfile fetches a profile via the Web API, cached in Redis so feed
// page loads don't each cost two Steam API calls.
func (h *Handler) steamProfile(r *http.Request, steamID string) (*models.SteamProfile, error) {
	key := "steamprofile:" + steamID
	if cached, err := h.Redis.Get(r.Context(), key).Bytes(); err == nil {
		var profile models.SteamProfile
		if json.Unmarshal(cached, &profile) == nil {
			return &profile, nil
		}
	}
	profile, err := h.SteamAPI.GetProfile(r.Context(), steamID)
	if err != nil {
		return nil, err
	}
	if encoded, err := json.Marshal(profile); err == nil {
		h.Redis.Set(r.Context(), key, encoded, steamProfileTTL)
	}
	return profile, nil
}

func (h *Handler) UnlinkSteam(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	if err := h.DB.SetSteamID(r.Context(), user.UserID, nil); err != nil {
		serverError(w, r, "unlink steam", err, "failed to unlink steam")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetFeed assembles the feed page's data in one round trip.
func (h *Handler) GetFeed(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	var feed models.Feed

	g, ctx := errgroup.WithContext(r.Context())
	g.Go(func() (err error) {
		feed.Watchlist, err = h.DB.GetWatchlist(ctx, user.UserID)
		return err
	})
	g.Go(func() (err error) {
		feed.Deals, err = h.DB.GetDeals(ctx, user.UserID)
		return err
	})
	g.Go(func() (err error) {
		feed.Suggestions, err = h.DB.GetSuggestions(ctx, user.UserID)
		return err
	})
	if err := g.Wait(); err != nil {
		serverError(w, r, "get feed", err, "failed to load feed")
		return
	}
	writeJSON(w, http.StatusOK, feed)
}

// Recent search inputs are stored per user as a capped Redis list; the
// results themselves live in the shared search cache (see games.go).

const (
	recentSearchesMax = 10
	recentSearchesTTL = 30 * 24 * time.Hour
)

type recentSearch struct {
	Search     string   `json:"search,omitempty"`
	Genres     []string `json:"genres,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Languages  []string `json:"languages,omitempty"`
	Developers []string `json:"developers,omitempty"`
	Publishers []string `json:"publishers,omitempty"`
	MinPrice   *float64 `json:"min_price,omitempty"`
	MaxPrice   *float64 `json:"max_price,omitempty"`
	// MinDiscount is the "at least this % off" filter, 1-100.
	MinDiscount *int `json:"min_discount,omitempty"`
}

// clean strips control characters and bounds every user-supplied string.
func (s *recentSearch) clean() {
	s.Search = sanitize.Text(s.Search, 100)
	if s.MinDiscount != nil && (*s.MinDiscount < 1 || *s.MinDiscount > 100) {
		s.MinDiscount = nil
	}
	for _, list := range []*[]string{&s.Genres, &s.Tags, &s.Languages, &s.Developers, &s.Publishers} {
		if len(*list) > 30 {
			*list = (*list)[:30]
		}
		cleaned := make([]string, 0, len(*list))
		for _, v := range *list {
			if v = sanitize.Text(v, 100); v != "" {
				cleaned = append(cleaned, v)
			}
		}
		*list = cleaned
	}
}

func (s recentSearch) isEmpty() bool {
	return strings.TrimSpace(s.Search) == "" && len(s.Genres) == 0 && len(s.Tags) == 0 &&
		len(s.Languages) == 0 && len(s.Developers) == 0 && len(s.Publishers) == 0 &&
		s.MinPrice == nil && s.MaxPrice == nil && s.MinDiscount == nil
}

func recentSearchesKey(userID int64) string {
	return "recent:" + strconv.FormatInt(userID, 10)
}

func (h *Handler) GetRecentSearches(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	values, err := h.Redis.LRange(r.Context(), recentSearchesKey(user.UserID), 0, recentSearchesMax-1).Result()
	if err != nil {
		serverError(w, r, "get recent searches", err, "failed to load recent searches")
		return
	}
	searches := make([]recentSearch, 0, len(values))
	for _, value := range values {
		var s recentSearch
		if json.Unmarshal([]byte(value), &s) == nil {
			searches = append(searches, s)
		}
	}
	writeJSON(w, http.StatusOK, searches)
}

func (h *Handler) AddRecentSearch(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	var s recentSearch
	if !decodeJSON(w, r, &s) {
		return
	}
	s.clean()
	if s.isEmpty() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	encoded, err := json.Marshal(s)
	if err != nil || len(encoded) > 4096 {
		writeError(w, http.StatusBadRequest, "search is too large to save")
		return
	}

	key := recentSearchesKey(user.UserID)
	pipe := h.Redis.TxPipeline()
	pipe.LRem(r.Context(), key, 0, encoded) // de-duplicate: move a repeat to the front
	pipe.LPush(r.Context(), key, encoded)
	pipe.LTrim(r.Context(), key, 0, recentSearchesMax-1)
	pipe.Expire(r.Context(), key, recentSearchesTTL)
	if _, err := pipe.Exec(r.Context()); err != nil {
		serverError(w, r, "add recent search", err, "failed to save search")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fillTrackStatuses marks which of the profile's recently played games we
// already track (or have queued), so the feed can offer a "track" button for
// the rest. Done per request because the profile itself is cached.
func (h *Handler) fillTrackStatuses(r *http.Request, profile *models.SteamProfile) {
	ids := make([]int, 0, len(profile.RecentlyPlayed))
	for _, g := range profile.RecentlyPlayed {
		ids = append(ids, g.AppID)
	}
	statuses, err := h.DB.GetTrackStatuses(r.Context(), ids)
	if err != nil {
		log.Printf("steam profile: track statuses: %v", err)
		return
	}
	for i := range profile.RecentlyPlayed {
		profile.RecentlyPlayed[i].TrackStatus = statuses[profile.RecentlyPlayed[i].AppID]
	}
}
