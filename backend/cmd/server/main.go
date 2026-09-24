package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/handlers"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/router"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/scheduler"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/scraper"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Error loading .env file")
	}
	ctx := context.Background()

	db, err := database.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	h := handlers.New(db)
	mux := router.New(h)

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
	go scheduler.Start(ctx, db, cfg, s, cfg.Steam.TrackedAppIDs, 24*time.Hour)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("API listening on :8080")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}
}
