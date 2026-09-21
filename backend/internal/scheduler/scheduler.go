package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/scraper"
)

// Start launches a background loop that re-scrapes all tracked games once
// per interval (immediately on startup, then on every tick), recording a
// fresh price_history entry for each game every time it runs. It never
// returns; call it with `go scheduler.Start(...)`.
func Start(ctx context.Context, db *database.DB, cfg *config.Config, s *scraper.Scraper, appIDs []int, interval time.Duration) {
	runOnce := func() {
		log.Println("Scheduler: starting scrape run")
		if err := scraper.RunScrape(ctx, db, cfg, s, appIDs); err != nil {
			log.Printf("Scheduler: scrape run finished with errors: %v", err)
		} else {
			log.Println("Scheduler: scrape run complete")
		}
	}

	runOnce()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}
