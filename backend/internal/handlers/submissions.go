package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/cache"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/scraper"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
)

type submissionJob struct {
	appID  int
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
	if err := scraper.RunScrape(ctx, db, cfg, s, []int{job.appID}); err != nil {
		log.Printf("submission %d: scrape: %v", job.appID, err)
	}

	appID := job.appID
	name, err := db.GetGameName(ctx, appID)
	if err == nil && name != "" {
		if err := db.SetTrackedGameStatus(ctx, appID, "tracked"); err != nil {
			log.Printf("submission %d: %v", appID, err)
		}
		db.CreateNotification(ctx, job.userID, &appID, "submission_tracked",
			fmt.Sprintf("%s is now being tracked. Thanks for the submission!", name))
		onTracked()
		return
	}

	if err := db.SetTrackedGameStatus(ctx, appID, "failed"); err != nil {
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
	appID, err := steam.ParseStoreAppURL(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limitKey := "ratelimit:submit:" + strconv.FormatInt(user.UserID, 10)
	if !cache.Allow(r.Context(), h.Redis, limitKey, 5, time.Hour) {
		writeError(w, http.StatusTooManyRequests, "you can submit up to 5 games an hour")
		return
	}

	status, shouldScrape, err := h.DB.SubmitTrackedGame(r.Context(), appID, user.UserID)
	if err != nil {
		log.Printf("submit game: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to submit game")
		return
	}
	if shouldScrape && !h.Submissions.enqueue(submissionJob{appID: appID, userID: user.UserID}) {
		h.DB.SetTrackedGameStatus(r.Context(), appID, "failed")
		writeError(w, http.StatusServiceUnavailable, "too many games are being added right now, try again shortly")
		return
	}

	httpStatus := http.StatusOK
	if shouldScrape {
		httpStatus = http.StatusAccepted
	}
	writeJSON(w, httpStatus, map[string]any{"app_id": appID, "status": status})
}

func (h *Handler) GetSubmissions(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	submissions, err := h.DB.GetSubmissions(r.Context(), user.UserID)
	if err != nil {
		log.Printf("get submissions: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load submissions")
		return
	}
	writeJSON(w, http.StatusOK, submissions)
}
