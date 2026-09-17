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

	games, err = db.GetGamesByGenre(ctx, "Action", 1)
	if err != nil {
		log.Fatalf("Failed to get games by genre: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}

	games, err = db.GetGamesByTag(ctx, "Multiplayer", 1)
	if err != nil {
		log.Fatalf("Failed to get games by tag: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}

	games, err = db.GetGamesByDeveloper(ctx, "Valve", 1)
	if err != nil {
		log.Fatalf("Failed to get games by developer: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}

	games, err = db.GetGamesByPublisher(ctx, "Valve", 1)
	if err != nil {
		log.Fatalf("Failed to get games by publisher: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}

	games, err = db.GetGamesByLanguage(ctx, "english", 1)
	if err != nil {
		log.Fatalf("Failed to get games by language: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}

	games, err = db.SearchGames(ctx, "Dota", 10, 1)
	if err != nil {
		log.Fatalf("Failed to search games: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}

	count, err := db.GetCountOfGames(ctx)
	if err != nil {
		log.Fatalf("Failed to get count of games: %v", err)
	}
	fmt.Printf("Total number of games in the database: %d\n", count)
	count, err = db.GetCountOfGamesByGenre(ctx, "Action")
	if err != nil {
		log.Fatalf("Failed to get count of games by genre: %v", err)
	}
	fmt.Printf("Total number of Action games in the database: %d\n", count)
	count, err = db.GetCountOfGamesByTag(ctx, "FPS")
	if err != nil {
		log.Fatalf("Failed to get count of games by tag: %v", err)
	}
	fmt.Printf("Total number of FPS games in the database: %d\n", count)
	count, err = db.GetCountOfGamesByDeveloper(ctx, "Facepunch Studios")
	if err != nil {
		log.Fatalf("Failed to get count of games by developer: %v", err)
	}
	fmt.Printf("Total number of games by Facepunch Studios in the database: %d\n", count)
	count, err = db.GetCountOfGamesByPublisher(ctx, "Valve")
	if err != nil {
		log.Fatalf("Failed to get count of games by publisher: %v", err)
	}
	fmt.Printf("Total number of games by Valve in the database: %d\n", count)
	count, err = db.GetCountOfGamesByLanguage(ctx, "english")
	if err != nil {
		log.Fatalf("Failed to get count of games by language: %v", err)
	}
	fmt.Printf("Total number of games with English language support in the database: %d\n", count)

}

