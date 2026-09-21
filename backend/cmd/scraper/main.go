package main

import (
	"context"
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

	// Scrape and persist all tracked games (details, reviews, price history).
	if err := scraper.RunScrape(ctx, db, cfg, s, cfg.Steam.TrackedAppIDs); err != nil {
		log.Printf("Scrape run finished with errors: %v", err)
	}
}