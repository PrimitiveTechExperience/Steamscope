package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
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

	s := scraper.New(cfg.Steam.BaseURL)

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
	// Query games from the database (More testing than anything else)
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
	fmt.Printf("Querying games: %v\n", appIDs)
	games, err := db.GetGames(ctx, appIDs, 1)
	if err != nil {
		log.Fatalf("Failed to get games: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
		for _, review := range game.Reviews {
			fmt.Printf("  Review: %s\n", review.Review)
		}
	}
	game, err := db.GetGame(ctx, 730)
	if err != nil {
		log.Fatalf("Failed to get game: %v", err)
	}
	fmt.Printf("Game: %s\n", game.Name)
	for _, review := range game.Reviews {
		fmt.Printf("  Review: %s\n", review.Review)
	}
}

