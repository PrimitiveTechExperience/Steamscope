package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"

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
	games, err := db.GetGames(ctx, models.GameFilters{Limit: 20})
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

	games, err = db.GetGames(ctx, models.GameFilters{Genre: "Action", Limit: 100})
	if err != nil {
		log.Fatalf("Failed to get games by genre: %v", err)
	}
	fmt.Printf("Games in Action genre:\n")
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}
	fmt.Printf("Games made by Valve:\n")
	games, err = db.GetGames(ctx, models.GameFilters{Developer: "Valve", Limit: 100})
	if err != nil {
		log.Fatalf("Failed to get games by developer: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}
	fmt.Printf("Games published by Valve:\n")
	games, err = db.GetGames(ctx, models.GameFilters{Publisher: "Valve", Limit: 100})
	if err != nil {
		log.Fatalf("Failed to get games by publisher: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}
	fmt.Printf("Games with Action genre:\n")
	games, err = db.GetGames(ctx, models.GameFilters{Genres: []string{"action"}, Limit: 100})
	if err != nil {
		log.Fatalf("Failed to get games by genres: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}
	fmt.Printf("Games with Multiplayer tag:\n")
	games, err = db.GetGames(ctx, models.GameFilters{Tags: []string{"multiplayer"}, Limit: 100})
	if err != nil {
		log.Fatalf("Failed to get games by tags: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}
	fmt.Printf("Games with English language support:\n")
	games, err = db.GetGames(ctx, models.GameFilters{Languages: []string{"english"}, Limit: 100})
	if err != nil {
		log.Fatalf("Failed to get games by languages: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}
	fmt.Printf("Games matching search query 'Dota':\n")
	games, err = db.GetGames(ctx, models.GameFilters{Search: "Dota", Limit: 10})
	if err != nil {
		log.Fatalf("Failed to search games: %v", err)
	}
	for _, game := range games {
		fmt.Printf("Game: %s\n", game.Name)
	}

	count, err := db.GetGamesCount(ctx, models.GameFilters{})
	if err != nil {
		log.Fatalf("Failed to get count of games: %v", err)
	}
	fmt.Printf("Total number of games in the database: %d\n", count)
	count, err = db.GetGamesCount(ctx, models.GameFilters{Genre: "Action"})
	if err != nil {
		log.Fatalf("Failed to get count of games by genre: %v", err)
	}
	fmt.Printf("Total number of Action games in the database: %d\n", count)
	count, err = db.GetGamesCount(ctx, models.GameFilters{Genres: []string{"action"}})
	if err != nil {
		log.Fatalf("Failed to get count of games by genres: %v", err)
	}
	fmt.Printf("Total number of Action games using the genre list: %d\n", count)
	count, err = db.GetGamesCount(ctx, models.GameFilters{Tags: []string{"multiplayer"}})
	if err != nil {
		log.Fatalf("Failed to get count of games by tags: %v", err)
	}
	fmt.Printf("Total number of Multiplayer games: %d\n", count)
	count, err = db.GetGamesCount(ctx, models.GameFilters{Languages: []string{"english"}})
	if err != nil {
		log.Fatalf("Failed to get count of games by languages: %v", err)
	}
	fmt.Printf("Total number of games with English language support: %d\n", count)
	count, err = db.GetGamesCount(ctx, models.GameFilters{Developer: "Facepunch Studios"})
	if err != nil {
		log.Fatalf("Failed to get count of games by developer: %v", err)
	}
	fmt.Printf("Total number of games by Facepunch Studios in the database: %d\n", count)
	count, err = db.GetGamesCount(ctx, models.GameFilters{Publisher: "Valve"})
	if err != nil {
		log.Fatalf("Failed to get count of games by publisher: %v", err)
	}
	fmt.Printf("Total number of games by Valve in the database: %d\n", count)
}
