package scraper

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

// RunScrape scrapes the given appIDs and persists the results (game details,
// reviews, and a price_history row for today) to the database. It is shared
// by the one-shot scraper CLI and the in-server daily scheduler so the two
// never drift apart.
func RunScrape(ctx context.Context, db *database.DB, cfg *config.Config, s *Scraper, appIDs []int) error {
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

	jobs := make(chan models.Game, len(games))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for game := range jobs {
				log.Printf("START inserting game: %s (AppID: %d)", game.Name, game.AppID)
				if err := db.InsertGame(ctx, game); err != nil {
					log.Printf("Failed to insert game into database: %v", err)
					continue
				}
				if err := db.UpsertPriceHistory(ctx, game.AppID, game.Price, game.OriginalPrice, game.DiscountPercentage, today); err != nil {
					log.Printf("Failed to upsert price history for game %s (AppID: %d): %v", game.Name, game.AppID, err)
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
	return nil
}
