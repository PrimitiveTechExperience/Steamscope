package scraper

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/alerts"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

// RunScrape scrapes the given appIDs and persists the results (game details,
// reviews, and a price_history row for today) to the database. It is shared
// by the one-shot scraper CLI and the in-server daily scheduler so the two
// never drift apart.
func RunScrape(ctx context.Context, db *database.DB, cfg *config.Config, s *Scraper, appIDs []int) error {
	return runScrape(ctx, db, cfg, s, appIDs, false)
}

// RunScrapeAll is RunScrape plus a re-scrape of every tracked bundle, not
// just the ones these games advertise - so bundles users submitted by link
// keep their price history current too. Used for the daily run.
func RunScrapeAll(ctx context.Context, db *database.DB, cfg *config.Config, s *Scraper, appIDs []int) error {
	return runScrape(ctx, db, cfg, s, appIDs, true)
}

func runScrape(ctx context.Context, db *database.DB, cfg *config.Config, s *Scraper, appIDs []int, includeTrackedBundles bool) error {
	games, err := s.ScrapeGames(appIDs, ReviewOption{
		Filter:     cfg.Steam.ReviewFilter,
		MaxReviews: cfg.Steam.ReviewMaxReviews,
		Language:   cfg.Steam.ReviewLanguage,
	})
	if err != nil {
		log.Printf("Errors occurred while scraping game pages: %v", err)
	}

	const workers = 5
	today := time.Now()

	notifier := alerts.New(db, cfg)
	jobs := make(chan models.Game, len(games))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for game := range jobs {
				// Steam redirects unknown app IDs to the store front page,
				// which parses as a "game" with no name.
				if game.Name == "" {
					log.Printf("Skipping AppID %d: page had no game name (not a valid store page?)", game.AppID)
					continue
				}
				log.Printf("START inserting game: %s (AppID: %d)", game.Name, game.AppID)
				if err := db.InsertGame(ctx, game); err != nil {
					log.Printf("Failed to insert game into database: %v", err)
					continue
				}
				if game.PriceUnknown {
					// No price on the page (delisted or unreleased): keep the last known
					// price rather than recording today as free.
					log.Printf("No price found on the page for %s (AppID: %d); keeping its last known price", game.Name, game.AppID)
				} else if err := db.UpsertPriceHistory(ctx, game.AppID, game.Price, game.OriginalPrice, game.DiscountPercentage, today); err != nil {
					log.Printf("Failed to upsert price history for game %s (AppID: %d): %v", game.Name, game.AppID, err)
				} else if targets, err := db.CreatePriceDropNotifications(ctx, game.AppID, game.Price, today); err != nil {
					log.Printf("Failed to create price drop notifications for game %s (AppID: %d): %v", game.Name, game.AppID, err)
				} else if len(targets) > 0 {
					// Also send them through each user's chosen channels (email, Discord, web push).
					notifier.Dispatch(ctx, targets)
				}
				if err := db.StoreReviews(ctx, game.AppID, game.Reviews, cfg.Steam.ReviewMaxReviews); err != nil {
					log.Printf("Failed to insert reviews into database for game %s (AppID: %d): %v", game.Name, game.AppID, err)
				}
				log.Printf("DONE inserting game: %s (AppID: %d)", game.Name, game.AppID)
			}
		}()
	}

	for _, game := range games {
		jobs <- game
	}
	close(jobs)

	wg.Wait()

	// Bundles: the ones these games' pages advertise (auto-discovery), plus
	// optionally every bundle already tracked.
	bundleIDs := map[int]bool{}
	for _, game := range games {
		for _, id := range game.BundleIDs {
			bundleIDs[id] = true
		}
	}
	if includeTrackedBundles {
		tracked, err := db.GetTrackedBundleIDs(ctx)
		if err != nil {
			log.Printf("Failed to load tracked bundles: %v", err)
		}
		for _, id := range tracked {
			bundleIDs[id] = true
		}
	}
	ScrapeAndStoreBundles(ctx, db, s, bundleIDs, today)
	return nil
}

// ScrapeAndStoreBundles scrapes the given bundle pages and saves each
// (contents, price, and today's price-history row). Failures are logged and
// skipped so one removed bundle doesn't block the rest. It returns the IDs
// that were stored.
func ScrapeAndStoreBundles(ctx context.Context, db *database.DB, s *Scraper, bundleIDs map[int]bool, date time.Time) map[int]bool {
	stored := map[int]bool{}
	if len(bundleIDs) == 0 {
		return stored
	}
	ids := make([]int, 0, len(bundleIDs))
	for id := range bundleIDs {
		ids = append(ids, id)
	}
	bundles, _ := s.ScrapeBundles(ids)
	for _, b := range bundles {
		if err := db.UpsertBundle(ctx, b, date); err != nil {
			log.Printf("Failed to store bundle %d (%s): %v", b.BundleID, b.Name, err)
			continue
		}
		stored[b.BundleID] = true
	}
	log.Printf("Bundles: stored %d of %d", len(stored), len(ids))
	return stored
}
