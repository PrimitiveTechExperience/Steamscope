package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/cache"
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
	cfg := config.LoadConfig()

	db, err := database.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	rdb, err := cache.New(ctx, cfg.RedisURL)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer rdb.Close()

	sessions, err := auth.NewSessionManager(rdb, cfg.Auth.SessionHashKey, cfg.Auth.SessionBlockKey, cfg.Auth.CookieSecure)
	if err != nil {
		log.Fatalf("Invalid session configuration: %v", err)
	}

	if err := db.SeedTrackedGames(ctx, cfg.Steam.TrackedAppIDs); err != nil {
		log.Fatalf("Failed to seed tracked games: %v", err)
	}

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

	invalidateSearchCache := func() { handlers.BumpSearchVersion(ctx, rdb) }

	submissions := handlers.NewSubmissionQueue(100)
	go submissions.Run(ctx, db, cfg, s, invalidateSearchCache)
	if cfg.DisableScheduler {
		log.Println("In-process scheduler disabled (DISABLE_SCHEDULER=true); run cmd/scraper from cron instead")
	} else {
		go scheduler.Start(ctx, db, cfg, s, 24*time.Hour, invalidateSearchCache)
	}

	h := handlers.New(handlers.Deps{
		DB:          db,
		Redis:       rdb,
		Sessions:    sessions,
		Config:      cfg,
		Submissions: submissions,
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           router.New(h, cfg.FrontendURL),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("API listening on :%s", port)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}
}
