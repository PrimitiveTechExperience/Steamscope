package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/itad"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/scraper"
)

// minHistoryRows is how many price_history rows a game needs before it's
// considered to have a real history; below that it gets an ITAD backfill.
const minHistoryRows = 30

func backfillThinHistories(ctx context.Context, db *database.DB, cfg *config.Config) {
	if cfg.ITADAPIKey == "" {
		return
	}
	appIDs, err := db.GetAppIDsNeedingBackfill(ctx, minHistoryRows)
	if err != nil {
		log.Printf("Scheduler: %v", err)
		return
	}
	client := itad.New(cfg.ITADAPIKey)
	to := time.Now()
	for _, appID := range appIDs {
		days, err := itad.BackfillGame(ctx, db, client, appID, to.AddDate(-2, 0, 0), to)
		if errors.Is(err, itad.ErrNoHistory) {
			continue // nothing to fetch; expected for some games, not worth a log line per run
		}
		if err != nil {
			log.Printf("Scheduler: backfill for appID %d failed: %v", appID, err)
			continue
		}
		log.Printf("Scheduler: backfilled %d days of price history for appID %d", days, appID)
		time.Sleep(time.Second) // stay under ITAD's rate limit
	}
}

// scrapeAndBackfill re-scrapes every tracked game and bundle, then backfills
// any game whose price history is still thin, recording run metrics.
func scrapeAndBackfill(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper) error {
	appIDs, err := db.GetTrackedAppIDs(ctx)
	if err != nil {
		observability.ScrapeRuns.WithLabelValues("error").Inc()
		return fmt.Errorf("failed to load tracked games: %w", err)
	}
	log.Printf("Scheduler: starting scrape run for %d games", len(appIDs))
	start := time.Now()
	runErr := scraper.RunScrapeAll(ctx, db, cfg, s, appIDs)
	observability.ScrapeDuration.Set(time.Since(start).Seconds())
	observability.ScrapeGames.Set(float64(len(appIDs)))
	if runErr != nil {
		observability.ScrapeRuns.WithLabelValues("error").Inc()
		log.Printf("Scheduler: scrape run finished with errors: %v", runErr)
	} else {
		observability.ScrapeRuns.WithLabelValues("success").Inc()
		observability.ScrapeLastSuccess.SetToCurrentTime()
		log.Println("Scheduler: scrape run complete")
	}
	backfillThinHistories(ctx, db, cfg)
	return runErr
}

// RunOnce does one complete maintenance pass - scrape, price-history
// backfill and old-history cleanup - and returns. It is what the standalone
// scraper command (and so the daily cron job) runs.
func RunOnce(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper) error {
	err := scrapeAndBackfill(ctx, db, cfg, s)
	pruneOldHistory(ctx, db)
	return err
}

func pruneOldHistory(ctx context.Context, db *database.DB) {
	cutoff := time.Now().AddDate(-2, 0, 0)
	if n, err := db.DeleteOldPriceHistory(ctx, cutoff); err != nil {
		log.Printf("Scheduler: price history cleanup failed: %v", err)
	} else if n > 0 {
		log.Printf("Scheduler: deleted %d price_history rows older than 2 years", n)
	}
}

// Start launches a background loop that re-scrapes all tracked games (read
// from the tracked_games table each run, so user submissions are included)
// once per interval, immediately on startup and then on every tick,
// recording a fresh price_history entry for each game. onScrape runs after
// each scrape (e.g. to invalidate caches). It also prunes price_history rows
// older than the 2-year retention window once a day. It never returns; call
// it with `go scheduler.Start(...)`.
func Start(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper, interval time.Duration, onScrape func()) {
	runScrape := func() {
		if err := scrapeAndBackfill(ctx, db, cfg, s); err != nil {
			log.Printf("Scheduler: %v", err)
		}
		onScrape()
	}

	runCleanup := func() { pruneOldHistory(ctx, db) }

	runScrape()
	runCleanup()

	scrapeTicker := time.NewTicker(interval)
	defer scrapeTicker.Stop()
	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-scrapeTicker.C:
			runScrape()
		case <-cleanupTicker.C:
			runCleanup()
		}
	}
}
