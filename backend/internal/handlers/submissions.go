package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/cache"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/itad"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/moderation"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/scraper"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
)

type submissionJob struct {
	kind   steam.StoreKind
	id     int
	userID int64
}

// SubmissionQueue scrapes user-submitted games one at a time on a single
// worker, so a burst of submissions can't fan out into unbounded concurrent
// requests to Steam.
type SubmissionQueue struct {
	jobs chan submissionJob
}

func NewSubmissionQueue(size int) *SubmissionQueue {
	return &SubmissionQueue{jobs: make(chan submissionJob, size)}
}

// Len is how many submissions are waiting to be scraped.
func (q *SubmissionQueue) Len() int { return len(q.jobs) }

func (q *SubmissionQueue) enqueue(job submissionJob) bool {
	select {
	case q.jobs <- job:
		return true
	default:
		return false
	}
}

// Run processes submissions until ctx is cancelled. onTracked is called
// after each game that's successfully added (e.g. to invalidate caches).
func (q *SubmissionQueue) Run(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper, onTracked func()) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-q.jobs:
			processSubmission(ctx, db, cfg, s, job, onTracked)
		}
	}
}

func processSubmission(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper, job submissionJob, onTracked func()) {
	if job.kind == steam.StoreKindBundle {
		processBundleSubmission(ctx, db, cfg, s, job, onTracked)
		return
	}
	if err := scraper.RunScrape(ctx, db, cfg, s, []int{job.id}); err != nil {
		log.Printf("submission %d: scrape: %v", job.id, err)
	}

	appID := job.id
	if blockedAfterScrape(ctx, db, appID) {
		if err := db.RejectGame(ctx, appID); err != nil {
			log.Printf("submission %d: %v", appID, err)
		}
		if err := db.DeleteWishlistRequests(ctx, appID); err != nil {
			log.Printf("submission %d: %v", appID, err)
		}
		db.CreateNotification(ctx, job.userID, nil, "submission_rejected",
			fmt.Sprintf("Your submission of Steam game %d wasn't approved.", appID))
		return
	}
	name, err := db.GetGameName(ctx, appID)
	if err == nil && name != "" {
		if err := db.SetTrackedGameStatus(ctx, appID, "tracked"); err != nil {
			log.Printf("submission %d: %v", appID, err)
		}
		// Today's price was just written by the scrape; pull the past two
		// years from ITAD so the chart has something to draw immediately.
		if cfg.ITADAPIKey != "" {
			now := time.Now()
			if _, err := itad.BackfillGame(ctx, db, itad.New(cfg.ITADAPIKey), appID, now.AddDate(-2, 0, 0), now); err != nil {
				log.Printf("submission %d: price history backfill: %v", appID, err)
			}
		}
		db.CreateNotification(ctx, job.userID, &appID, "submission_tracked",
			fmt.Sprintf("%s is now being tracked. Thanks for the submission!", name))
		// Users who asked for it from their wishlist now watch it, pinned if they chose that.
		if n, err := db.ApplyWishlistRequests(ctx, appID); err != nil {
			log.Printf("submission %d: wishlist requests: %v", appID, err)
		} else if n > 0 {
			log.Printf("submission %d: %d wishlist request(s) now watching", appID, n)
		}
		onTracked()
		return
	}

	if err := db.SetTrackedGameStatus(ctx, appID, "failed"); err != nil {
		log.Printf("submission %d: %v", appID, err)
	}
	if err := db.DeleteWishlistRequests(ctx, appID); err != nil {
		log.Printf("submission %d: %v", appID, err)
	}
	db.CreateNotification(ctx, job.userID, nil, "submission_failed",
		fmt.Sprintf("We couldn't find a Steam game with app ID %d, so it wasn't added.", appID))
}

type submitRequest struct {
	URL string `json:"url"`
}

func (h *Handler) SubmitGame(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	var req submitRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	kind, id, err := steam.ParseStoreURL(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if user.SubmissionsBlocked {
		writeError(w, http.StatusForbidden, "you're not able to submit games")
		return
	}
	if kind == steam.StoreKindApp && h.appIDBlacklisted(r.Context(), id) {
		writeError(w, http.StatusUnprocessableEntity, "that game can't be added to Steamscope")
		return
	}
	// Admins aren't rate limited: their submissions are scraped straight away.
	limitKey := "ratelimit:submit:" + strconv.FormatInt(user.UserID, 10)
	if !user.IsAdmin && !cache.Allow(r.Context(), h.Redis, limitKey, 5, time.Hour) {
		writeError(w, http.StatusTooManyRequests, "you can submit up to 5 games or bundles an hour")
		return
	}

	// Ordinary users' submissions wait for an admin to approve them before
	// anything is scraped; admins' own submissions go straight through.
	initial := "awaiting_approval"
	if user.IsAdmin {
		initial = "pending"
	}
	var status string
	var created bool
	if kind == steam.StoreKindBundle {
		status, created, err = h.DB.SubmitBundle(r.Context(), id, user.UserID, initial)
	} else {
		status, created, err = h.DB.SubmitTrackedGame(r.Context(), id, user.UserID, initial)
	}
	if err != nil {
		serverError(w, r, "submit game", err, "failed to submit game")
		return
	}
	if created && status == "pending" && !h.enqueueSubmission(r.Context(), kind, id, user.UserID) {
		writeError(w, http.StatusServiceUnavailable, "too many submissions are being processed right now, try again shortly")
		return
	}

	httpStatus := http.StatusOK
	outcome := "duplicate"
	if created {
		httpStatus = http.StatusAccepted
		outcome = status
	}
	observability.Submissions.WithLabelValues(string(kind), outcome).Inc()
	writeJSON(w, httpStatus, map[string]any{"kind": kind, "id": id, "status": status})
}

func (h *Handler) GetSubmissions(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	submissions, err := h.DB.GetSubmissions(r.Context(), user.UserID)
	if err != nil {
		serverError(w, r, "get submissions", err, "failed to load submissions")
		return
	}
	writeJSON(w, http.StatusOK, submissions)
}

// processBundleSubmission scrapes a submitted bundle (and stores it), then
// tells the submitter how it went.
func processBundleSubmission(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper, job submissionJob, onTracked func()) {
	stored := scraper.ScrapeAndStoreBundles(ctx, db, s, map[int]bool{job.id: true}, time.Now())
	if !stored[job.id] {
		if err := db.SetBundleStatus(ctx, job.id, "failed"); err != nil {
			log.Printf("bundle submission %d: %v", job.id, err)
		}
		db.CreateNotification(ctx, job.userID, nil, "bundle_failed",
			fmt.Sprintf("We couldn't read Steam bundle %d, so it wasn't added.", job.id))
		return
	}
	// Only today's price was just recorded; import the rest of its history so
	// its chart and forecast have something to work from straight away.
	if cfg.ITADAPIKey != "" {
		now := time.Now()
		if _, err := itad.BackfillBundle(ctx, db, itad.New(cfg.ITADAPIKey), job.id, now.AddDate(-itad.BundleHistoryYears, 0, 0), now); err != nil &&
			!errors.Is(err, itad.ErrNoHistory) && !errors.Is(err, itad.ErrBundleUnknown) {
			log.Printf("bundle submission %d: history import: %v", job.id, err)
		}
	}
	name, _ := db.GetBundleName(ctx, job.id)
	db.CreateNotification(ctx, job.userID, nil, "bundle_tracked",
		fmt.Sprintf("%s is now being tracked. Thanks for the submission!", name))
	onTracked()
}

// enqueueSubmission hands an approved submission to the scrape worker. If the
// queue is full it marks the item failed so it can be resubmitted.
func (h *Handler) enqueueSubmission(ctx context.Context, kind steam.StoreKind, id int, userID int64) bool {
	if h.Submissions.enqueue(submissionJob{kind: kind, id: id, userID: userID}) {
		return true
	}
	if kind == steam.StoreKindBundle {
		h.DB.SetBundleStatus(ctx, id, "failed")
	} else {
		h.DB.SetTrackedGameStatus(ctx, id, "failed")
	}
	return false
}

// InvalidateCaches drops cached search results after game data changes.
func (h *Handler) InvalidateCaches(ctx context.Context) {
	BumpSearchVersion(ctx, h.Redis)
}

// appIDBlacklisted reports whether an app_id rule blocks this game outright.
// (Name/developer/publisher rules need the scraped data; see blockedAfterScrape.)
func (h *Handler) appIDBlacklisted(ctx context.Context, appID int) bool {
	rules, err := h.DB.CompiledBlacklist(ctx)
	if err != nil {
		log.Printf("blacklist: %v", err)
		return false
	}
	for _, r := range rules {
		if r.Matches(moderation.GameMeta{AppID: appID}) && r.IsAppID() {
			return true
		}
	}
	return false
}

// blockedAfterScrape checks a freshly scraped game against every blacklist
// rule (name, developer and publisher rules can only be checked once we know
// them).
func blockedAfterScrape(ctx context.Context, db *database.DB, appID int) bool {
	rules, err := db.CompiledBlacklist(ctx)
	if err != nil || len(rules) == 0 {
		return false
	}
	metas, err := db.GamesMeta(ctx, appID)
	if err != nil || len(metas) == 0 {
		return false
	}
	for _, r := range rules {
		if r.Matches(metas[0]) {
			return true
		}
	}
	return false
}
