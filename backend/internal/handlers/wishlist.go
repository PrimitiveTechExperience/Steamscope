package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/cache"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/moderation"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
)

const (
	// Importing calls Steam, so it is limited per user.
	wishlistImportLimit = 6
	// Requesting is limited more tightly: each request can put many games in front of an admin.
	wishlistRequestLimit = 3
	wishlistWindow       = time.Hour
	// At most this many games can be requested in one go.
	maxWishlistRequest = 100
	// Names are looked up for this many of the requestable games, to make the list readable.
	maxWishlistNames = 30
	wishlistNameWait = 6 * time.Second
)

type wishlistGame struct {
	AppID int    `json:"app_id"`
	Name  string `json:"name"` // "" when it could not be looked up
}

type wishlistImportResponse struct {
	// WishlistSize is how many games Steam reported. 0 means the wishlist is empty or private.
	WishlistSize int `json:"wishlist_size"`
	// Watched is how many games the user started watching just now.
	Watched int `json:"watched"`
	// AlreadyWatched is how many on the wishlist they were already watching.
	AlreadyWatched int `json:"already_watched"`
	// Requestable are games we do not track yet that the user can ask to have added.
	Requestable []wishlistGame `json:"requestable"`
	// AwaitingReview is how many have already been requested and are on their way.
	AwaitingReview int `json:"awaiting_review"`
	// Unavailable is how many cannot be added (turned down earlier, or blocked).
	Unavailable int `json:"unavailable"`
}

// ImportWishlist starts watching every wishlisted game we already have, and
// reports the rest so the user can choose to request them.
func (h *Handler) ImportWishlist(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	ids, ok := h.fetchWishlist(w, r, wishlistImportLimit, "wishlistimport")
	if !ok {
		return
	}

	resp := wishlistImportResponse{WishlistSize: len(ids), Requestable: []wishlistGame{}}
	existing, err := h.DB.ExistingGameIDs(r.Context(), ids)
	if err != nil {
		serverError(w, r, "wishlist: look up games", err, "failed to import your wishlist")
		return
	}
	var known, unknown []int
	for _, id := range ids {
		if existing[id] {
			known = append(known, id)
		} else {
			unknown = append(unknown, id)
		}
	}
	added, err := h.DB.WatchGames(r.Context(), user.UserID, known)
	if err != nil {
		serverError(w, r, "wishlist: watch games", err, "failed to import your wishlist")
		return
	}
	resp.Watched, resp.AlreadyWatched = added, len(known)-added

	requestable, awaiting, unavailable, err := h.classifyMissing(r.Context(), unknown)
	if err != nil {
		serverError(w, r, "wishlist: classify games", err, "failed to import your wishlist")
		return
	}
	resp.AwaitingReview, resp.Unavailable = awaiting, unavailable
	for _, id := range requestable {
		resp.Requestable = append(resp.Requestable, wishlistGame{AppID: id})
	}
	h.nameWishlistGames(r.Context(), resp.Requestable)

	writeJSON(w, http.StatusOK, resp)
}

type wishlistRequest struct {
	AppIDs []int `json:"app_ids"`
}

// RequestWishlistGames asks for wishlisted games we do not track to be added,
// the same way a submission does (an admin approves them first). Only games
// that really are on the user's wishlist are accepted, so this cannot be used
// to put arbitrary IDs in front of the admins.
func (h *Handler) RequestWishlistGames(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	var req wishlistRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.AppIDs) == 0 {
		writeError(w, http.StatusBadRequest, "no games to request")
		return
	}
	if len(req.AppIDs) > maxWishlistRequest {
		writeError(w, http.StatusBadRequest, "you can request up to "+strconv.Itoa(maxWishlistRequest)+" games at a time")
		return
	}
	if user.SubmissionsBlocked {
		writeError(w, http.StatusForbidden, "you're not able to submit games")
		return
	}

	wishlist, ok := h.fetchWishlist(w, r, wishlistRequestLimit, "wishlistrequest")
	if !ok {
		return
	}
	onList := make(map[int]bool, len(wishlist))
	for _, id := range wishlist {
		onList[id] = true
	}
	var wanted []int
	seen := map[int]bool{}
	for _, id := range req.AppIDs {
		if id > 0 && onList[id] && !seen[id] {
			seen[id] = true
			wanted = append(wanted, id)
		}
	}
	existing, err := h.DB.ExistingGameIDs(r.Context(), wanted)
	if err != nil {
		serverError(w, r, "wishlist: look up games", err, "failed to request those games")
		return
	}
	var missing []int
	for _, id := range wanted {
		if !existing[id] {
			missing = append(missing, id)
		}
	}
	requestable, _, _, err := h.classifyMissing(r.Context(), missing)
	if err != nil {
		serverError(w, r, "wishlist: classify games", err, "failed to request those games")
		return
	}

	// As with a single submission, admins' requests are scraped straight away
	// and everyone else's wait for approval.
	initial := "awaiting_approval"
	if user.IsAdmin {
		initial = "pending"
	}
	requested := 0
	for _, id := range requestable {
		status, created, err := h.DB.SubmitTrackedGame(r.Context(), id, user.UserID, initial)
		if err != nil {
			serverError(w, r, "wishlist: submit game", err, "failed to request those games", "app_id", id)
			return
		}
		if !created {
			continue
		}
		if status == "pending" && !h.enqueueSubmission(r.Context(), steam.StoreKindApp, id, user.UserID) {
			continue
		}
		observability.Submissions.WithLabelValues("app", status).Inc()
		requested++
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"requested": requested,
		"skipped":   len(req.AppIDs) - requested,
		"status":    initial,
	})
}

// fetchWishlist reads the signed-in user's Steam wishlist, writing the error
// response itself when that is not possible.
func (h *Handler) fetchWishlist(w http.ResponseWriter, r *http.Request, limit int64, limitName string) ([]int, bool) {
	user := auth.CurrentUser(r.Context())
	if user.SteamID == nil {
		writeError(w, http.StatusBadRequest, "link your Steam account first")
		return nil, false
	}
	if !user.IsAdmin && !cache.Allow(r.Context(), h.Redis, "ratelimit:"+limitName+":"+strconv.FormatInt(user.UserID, 10), limit, wishlistWindow) {
		writeError(w, http.StatusTooManyRequests, "you have done that a few times already, try again in a while")
		return nil, false
	}
	ids, err := h.SteamAPI.GetWishlist(r.Context(), *user.SteamID)
	switch {
	case errors.Is(err, steam.ErrNoAPIKey):
		writeError(w, http.StatusServiceUnavailable, "steam is not set up on this server yet")
		return nil, false
	case err != nil:
		if r.Context().Err() != nil {
			return nil, false
		}
		log.Printf("wishlist: %v", err)
		writeError(w, http.StatusBadGateway, "could not reach Steam right now")
		return nil, false
	}
	return ids, true
}

// classifyMissing sorts wishlisted games we have no data for into those that
// can be requested, those already requested or being added, and those that
// cannot be added at all.
func (h *Handler) classifyMissing(ctx context.Context, appIDs []int) (requestable []int, awaiting, unavailable int, err error) {
	if len(appIDs) == 0 {
		return nil, 0, 0, nil
	}
	statuses, err := h.DB.GetTrackStatuses(ctx, appIDs)
	if err != nil {
		return nil, 0, 0, err
	}
	rules, err := h.DB.CompiledBlacklist(ctx) // once, not per game
	if err != nil {
		return nil, 0, 0, err
	}
	for _, id := range appIDs {
		switch statuses[id] {
		case "awaiting_approval", "pending", "tracked":
			awaiting++ // requested already, or tracked and about to appear
			continue
		case "rejected":
			unavailable++
			continue
		}
		if blockedByAppID(rules, id) {
			unavailable++
			continue
		}
		requestable = append(requestable, id)
	}
	return requestable, awaiting, unavailable, nil
}

// nameWishlistGames fills in names for the first few games, looked up from
// Steam in parallel within a short deadline. Games it cannot name stay blank.
func (h *Handler) nameWishlistGames(ctx context.Context, games []wishlistGame) {
	n := min(len(games), maxWishlistNames)
	if n == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, wishlistNameWait)
	defer cancel()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(g *wishlistGame) {
			defer wg.Done()
			defer func() { <-sem }()
			g.Name = h.SteamAPI.AppName(ctx, g.AppID)
		}(&games[i])
	}
	wg.Wait()
}

// blockedByAppID reports whether an app_id blacklist rule blocks the game.
func blockedByAppID(rules []moderation.Rule, appID int) bool {
	for _, rule := range rules {
		if rule.IsAppID() && rule.Matches(moderation.GameMeta{AppID: appID}) {
			return true
		}
	}
	return false
}
