package handlers

import (
	"context"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/scraper"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
)

// ScrapeFunc is the shape of the scrape a submission performs.
type ScrapeFunc = func(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper, appIDs []int) error

// ProcessAppSubmissionForTest runs the worker's handling of one approved game
// submission with the scrape replaced by scrape.
func ProcessAppSubmissionForTest(ctx context.Context, db *database.DB, cfg *config.Config, appID int, userID int64, scrape ScrapeFunc) {
	old := scrapeApps
	scrapeApps = scrape
	defer func() { scrapeApps = old }()
	processSubmission(ctx, db, cfg, nil, submissionJob{kind: steam.StoreKindApp, id: appID, userID: userID}, func() {})
}
