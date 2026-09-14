package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"sync"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/scraper"

	"github.com/joho/godotenv"
)
	
func main() {
	// Start database connection.
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}
	ctx := context.Background()

	db, err := database.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Load configuration, cookies
	cfg := config.LoadConfig()

	s := scraper.New()

	cookies, err := config.LoadCookies(cfg.Steam.CookieFilePath)
	if err != nil {
		log.Fatalf("Failed to load cookies: %v", err)
	}

	if err := s.SetSteamCookies(cookies); err != nil {
		log.Fatalf("Failed to set Steam cookies: %v", err)
	}

	u, _ := url.Parse("https://store.steampowered.com")
	for _, c := range s.JarCookies(u) {
		log.Printf("Loaded cookie: %s", c.Name)
	}

	// Scrape games.
	appIDs := []int{
		730,    // Counter-Strike: Global Offensive
		570,    // Dota 2
		440,    // Team Fortress 2
		578080, // PLAYERUNKNOWN'S BATTLEGROUNDS
		4000,   // Garry's Mod
		550,    // Left 4 Dead 2
		252490, // Rust
		// 304930, // Unturned
		// 271590, // Grand Theft Auto V
		// 1174180, // Cyberpunk 2077
	}

	games, errors := s.ScrapeGames(appIDs, scraper.ReviewOption{
		Filter: cfg.Steam.ReviewFilter,
		MaxReviews: cfg.Steam.ReviewMaxReviews,
		Language: cfg.Steam.ReviewLanguage,
	})
	if errors != nil {
		log.Printf("Errors occurred while scraping game pages: %v", errors)
	}
	// Make Game insertion concurrent
	const workers = 5
	
	jobs := make(chan models.Game, len(games))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for game := range jobs {
				// log.Printf("Scraped game: %s (AppID: %d)", game.Name, game.AppID)
				log.Printf("START inserting game: %s (AppID: %d)", game.Name, game.AppID)
				if err := db.InsertGame(ctx, game); err != nil {
					log.Printf("Failed to insert game into database: %v", err)
				}
				log.Printf("DONE inserting game: %s (AppID: %d)", game.Name, game.AppID)
				if err := db.StoreReviews(ctx, game.AppID, game.Reviews, cfg.Steam.ReviewMaxReviews); err != nil {
					log.Printf("Failed to insert reviews into database for game %s (AppID: %d): %v", game.Name, game.AppID, err)
				}
				log.Printf("Inserted game into database: %s (AppID: %d)", game.Name, game.AppID)
			}
		}()
	}

	for _, game := range games {
		jobs <- game
	}
	close(jobs)

	wg.Wait()
	// reviews, err := s.FetchReviewsForGames(games, scraper.ReviewOption{
	// 	Filter: "recent",
	// 	MaxReviews: 1,
	// 	Language: "english",
	// })

	// if err != nil {
	// 	log.Printf("Errors occurred while fetching reviews: %v", err)
	// }
	// for appID, reviewList := range reviews {
	// 	for _, review := range reviewList {
	// 		log.Printf("Scraped review for game %d: %s", appID, review.Review)
	// 	}
	// }


}