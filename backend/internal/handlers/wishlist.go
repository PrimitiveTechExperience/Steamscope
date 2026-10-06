package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/cache"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/moderation"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
)

const (
	// Importing calls Steam, so it is limited per user.
	wishlistImportLimit = 6
	// Requesting is limited more tightly: each request can put many games in front of an admin.
	wishlistRequestLimit = 3
	// Checking the state is done whenever the feed opens (and repeatedly while games are
	// being added), so it gets roomier limits. Only the Steam call is expensive, and
	// Steam's answer is cached; everything else is worked out fresh from our own data.
	wishlistStatusLimit = 60
	wishlistStatusCalls = 600
	wishlistWindow      = time.Hour
	wishlistCacheTTL    = 5 * time.Minute
	// At most this many games can be requested in one go.
	maxWishlistRequest = 100
	// Names are looked up for this many of the requestable games, to make the list readable.
	maxWishlistNames = 30
	wishlistNameWait = 6 * time.Second
)

// Where a user's wishlist stands.
const (
	wishlistEmpty      = "empty"      // nothing on it, or it is private
	wishlistIncomplete = "incomplete" // some games still need importing
	wishlistWaiting    = "waiting"    // everything else is in; the user's requested games are being added
	wishlistComplete   = "complete"   // every wishlist game is on their watchlist
)

type wishlistGame struct {
	AppID int    `json:"app_id"`
	Name  string `json:"name"` // "" when it could not be looked up
}

// wishlistAnalysis is what a wishlist looks like next to our data and the
// user's watchlist.
type wishlistAnalysis struct {
	size          int
	known         int   // games we have data for
	unwatched     []int // of those, the ones the user is not watching yet
	requestable   []int // games we do not have that can be requested
	awaitingMine  []int // not tracked yet, and the user has asked for them
	awaitingOther []int // not tracked yet, being added for someone else; the user has not asked
	unavailable   int   // turned down, or blocked
}

func (a wishlistAnalysis) alreadyWatched() int { return a.known - len(a.unwatched) }

// remaining is how many games still need something from the user.
func (a wishlistAnalysis) remaining() int {
	return len(a.unwatched) + len(a.requestable) + len(a.awaitingOther)
}

func (a wishlistAnalysis) state() string {
	switch {
	case a.size == 0:
		return wishlistEmpty
	case a.remaining() > 0:
		return wishlistIncomplete
	case len(a.awaitingMine) > 0:
		return wishlistWaiting
	default:
		return wishlistComplete
	}
}

func (h *Handler) analyzeWishlist(ctx context.Context, userID int64, ids []int) (wishlistAnalysis, error) {
	a := wishlistAnalysis{size: len(ids)}
	existing, err := h.DB.ExistingGameIDs(ctx, ids)
	if err != nil {
		return a, err
	}
	var known, unknown []int
	for _, id := range ids {
		if existing[id] {
			known = append(known, id)
		} else {
			unknown = append(unknown, id)
		}
	}
	a.known = len(known)
	watched, err := h.DB.WatchedAmong(ctx, userID, known)
	if err != nil {
		return a, err
	}
	for _, id := range known {
		if !watched[id] {
			a.unwatched = append(a.unwatched, id)
		}
	}
	if len(unknown) == 0 {
		return a, nil
	}

	statuses, err := h.DB.GetTrackStatuses(ctx, unknown)
	if err != nil {
		return a, err
	}
	mine, err := h.DB.WishlistRequested(ctx, userID, unknown)
	if err != nil {
		return a, err
	}
	rules, err := h.DB.CompiledBlacklist(ctx) // once, not per game
	if err != nil {
		return a, err
	}
	for _, id := range unknown {
		switch statuses[id] {
		case "awaiting_approval", "pending": // requested already, and on its way
			if mine[id] {
				a.awaitingMine = append(a.awaitingMine, id)
			} else {
				a.awaitingOther = append(a.awaitingOther, id)
			}
		case "rejected":
			a.unavailable++
		default:
			// Includes a leftover "tracked" row for a game whose data is gone (it was
			// deleted from the database): there is nothing to watch, so it can be requested again.
			if blockedByAppID(rules, id) {
				a.unavailable++
			} else {
				a.requestable = append(a.requestable, id)
			}
		}
	}
	return a, nil
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
	// AwaitingReview is how many the user has asked for that are on their way.
	AwaitingReview int `json:"awaiting_review"`
	// Unavailable is how many cannot be added (turned down earlier, or blocked).
	Unavailable int `json:"unavailable"`
	// State is "empty", "incomplete", "waiting" or "complete".
	State string `json:"state"`
}

// ImportWishlist starts watching every wishlisted game we already have, and
// reports the rest so the user can choose to request them.
func (h *Handler) ImportWishlist(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	ids, ok := h.fetchWishlist(w, r, wishlistImportLimit, "wishlistimport", false)
	if !ok {
		return
	}
	before, err := h.analyzeWishlist(r.Context(), user.UserID, ids)
	if err != nil {
		serverError(w, r, "wishlist: analyze", err, "failed to import your wishlist")
		return
	}
	added, err := h.DB.WatchGames(r.Context(), user.UserID, before.unwatched)
	if err != nil {
		serverError(w, r, "wishlist: watch games", err, "failed to import your wishlist")
		return
	}
	// Games someone else already asked for will be watched for this user as
	// well, the moment they are added.
	var follow []database.WishlistRequest
	for _, id := range before.awaitingOther {
		follow = append(follow, database.WishlistRequest{AppID: id})
	}
	if err := h.DB.RecordWishlistRequests(r.Context(), user.UserID, follow); err != nil {
		serverError(w, r, "wishlist: record requests", err, "failed to import your wishlist")
		return
	}
	after, err := h.analyzeWishlist(r.Context(), user.UserID, ids)
	if err != nil {
		serverError(w, r, "wishlist: analyze", err, "failed to import your wishlist")
		return
	}

	resp := wishlistImportResponse{
		WishlistSize:   after.size,
		Watched:        added,
		AlreadyWatched: before.alreadyWatched(),
		Requestable:    []wishlistGame{},
		AwaitingReview: len(after.awaitingMine),
		Unavailable:    after.unavailable,
		State:          after.state(),
	}
	for _, id := range after.requestable {
		resp.Requestable = append(resp.Requestable, wishlistGame{AppID: id})
	}
	h.nameWishlistGames(r.Context(), resp.Requestable)
	writeJSON(w, http.StatusOK, resp)
}

type wishlistRequest struct {
	AppIDs []int `json:"app_ids"`
	// PinnedAppIDs are the ones (a subset of AppIDs) to pin to the feed once they are added.
	PinnedAppIDs []int `json:"pinned_app_ids"`
}

// RequestWishlistGames asks for wishlisted games we do not track to be added,
// the same way a submission does (an admin approves them first). Once a game is
// added the user starts watching it, pinned if they chose that. Only games that
// really are on the user's wishlist are accepted, so this cannot be used to put
// arbitrary IDs in front of the admins.
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

	wishlist, ok := h.fetchWishlist(w, r, wishlistRequestLimit, "wishlistrequest", false)
	if !ok {
		return
	}
	asked := make(map[int]bool, len(req.AppIDs))
	for _, id := range req.AppIDs {
		asked[id] = true
	}
	pin := make(map[int]bool, len(req.PinnedAppIDs))
	for _, id := range req.PinnedAppIDs {
		pin[id] = true
	}
	// Of what was asked for, only games really on the wishlist that we can still request.
	current, err := h.analyzeWishlist(r.Context(), user.UserID, wishlist)
	if err != nil {
		serverError(w, r, "wishlist: analyze", err, "failed to request those games")
		return
	}
	var wanted []database.WishlistRequest
	for _, id := range current.requestable {
		if asked[id] {
			wanted = append(wanted, database.WishlistRequest{AppID: id, Pinned: pin[id]})
		}
	}
	// Remember the choice first: an admin's request can finish scraping before
	// this handler returns.
	if err := h.DB.RecordWishlistRequests(r.Context(), user.UserID, wanted); err != nil {
		serverError(w, r, "wishlist: record requests", err, "failed to request those games")
		return
	}

	// As with a single submission, admins' requests are scraped straight away
	// and everyone else's wait for approval.
	initial := "awaiting_approval"
	if user.IsAdmin {
		initial = "pending"
	}
	requested := 0
	for _, want := range wanted {
		status, created, err := h.DB.SubmitTrackedGame(r.Context(), want.AppID, user.UserID, initial)
		if err != nil {
			serverError(w, r, "wishlist: submit game", err, "failed to request those games", "app_id", want.AppID)
			return
		}
		if !created {
			continue
		}
		if status == "pending" && !h.enqueueSubmission(r.Context(), steam.StoreKindApp, want.AppID, user.UserID) {
			continue
		}
		observability.Submissions.WithLabelValues("app", status).Inc()
		requested++
	}

	after, err := h.analyzeWishlist(r.Context(), user.UserID, wishlist)
	if err != nil {
		serverError(w, r, "wishlist: analyze", err, "failed to request those games")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"requested": requested,
		"skipped":   len(req.AppIDs) - requested,
		"status":    initial,
		"state":     after.state(),
	})
}

type wishlistStatusResponse struct {
	State string `json:"state"`
	// WishlistSize is how many games Steam reported.
	WishlistSize int `json:"wishlist_size"`
	// Remaining is how many games still need importing or requesting.
	Remaining int `json:"remaining"`
	// Waiting is how many requested games are still being added.
	Waiting     int `json:"waiting"`
	Unavailable int `json:"unavailable"`
}

func wishlistIDsKey(userID int64) string { return "wishlist:ids:" + strconv.FormatInt(userID, 10) }

// GetWishlistStatus says where the user's wishlist stands, without changing
// anything, so the feed can show "all imported" instead of an import button.
//
// Only Steam's answer (which games are on the wishlist) is cached, briefly. How
// those games stand against our data and the user's watchlist is worked out on
// every call, so a game that was deleted, added or watched shows up at once.
func (h *Handler) GetWishlistStatus(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	if user.SteamID == nil {
		writeError(w, http.StatusBadRequest, "link your Steam account first")
		return
	}
	if !user.IsAdmin && !cache.Allow(r.Context(), h.Redis, "ratelimit:wishliststatuscalls:"+strconv.FormatInt(user.UserID, 10), wishlistStatusCalls, wishlistWindow) {
		writeError(w, http.StatusTooManyRequests, "you have done that a few times already, try again in a while")
		return
	}
	ids, ok := h.fetchWishlist(w, r, wishlistStatusLimit, "wishliststatus", true)
	if !ok {
		return
	}
	a, err := h.analyzeWishlist(r.Context(), user.UserID, ids)
	if err != nil {
		serverError(w, r, "wishlist: analyze", err, "failed to check your wishlist")
		return
	}
	writeJSON(w, http.StatusOK, wishlistStatusResponse{
		State: a.state(), WishlistSize: a.size, Remaining: a.remaining(), Waiting: len(a.awaitingMine), Unavailable: a.unavailable,
	})
}

// fetchWishlist reads the signed-in user's Steam wishlist, writing the error
// response itself when that is not possible. With useCache it may answer from
// a copy fetched in the last few minutes; every fetch from Steam refreshes it.
func (h *Handler) fetchWishlist(w http.ResponseWriter, r *http.Request, limit int64, limitName string, useCache bool) ([]int, bool) {
	user := auth.CurrentUser(r.Context())
	if user.SteamID == nil {
		writeError(w, http.StatusBadRequest, "link your Steam account first")
		return nil, false
	}
	key := wishlistIDsKey(user.UserID)
	if useCache {
		if cached, err := h.Redis.Get(r.Context(), key).Bytes(); err == nil {
			var ids []int
			if json.Unmarshal(cached, &ids) == nil {
				return ids, true
			}
		}
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
	if encoded, err := json.Marshal(ids); err == nil {
		h.Redis.Set(r.Context(), key, encoded, wishlistCacheTTL)
	}
	return ids, true
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
