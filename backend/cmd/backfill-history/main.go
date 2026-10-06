package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/itad"
	"github.com/joho/godotenv"
)

// backfill-history pulls up to 2 years of historical Steam price-change
// events per tracked game from IsThereAnyDeal and writes a daily price_history
// row for each day in that window. Run it once (or whenever you add a new
// tracked game) - it's safe to re-run since price_history upserts on
// (app_id, recorded_date).
func main() {
	bundlesOnly := flag.Bool("bundles-only", false, "only import bundle histories, leave games alone")
	flag.Parse()
	if err := godotenv.Load(); err != nil {
		log.Println("Error loading .env file")
	}
	ctx := context.Background()

	db, err := database.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	cfg := config.LoadConfig()
	if cfg.ITADAPIKey == "" {
		log.Fatal("ITAD_API_KEY is not set - get a free key at https://isthereanydeal.com/apps/my/")
	}

	client := itad.New(cfg.ITADAPIKey)

	endDate := time.Now()
	startDate := endDate.AddDate(-2, 0, 0)

	appIDs := []int{}
	if !*bundlesOnly {
		if err := db.SeedTrackedGames(ctx, cfg.Steam.TrackedAppIDs); err != nil {
			log.Fatalf("Failed to seed tracked games: %v", err)
		}
		appIDs, err = db.GetTrackedAppIDs(ctx)
		if err != nil {
			log.Fatalf("Failed to load tracked games: %v", err)
		}
	}

	for _, appID := range appIDs {
		days, err := itad.BackfillGame(ctx, db, client, appID, startDate, endDate)
		if err != nil {
			log.Printf("Skipping appID %d: %v", appID, err)
			continue
		}
		log.Printf("Backfilled %d days for appID %d", days, appID)

		time.Sleep(1 * time.Second) // stay well under ITAD's rate limit
	}

	// Bundles keep their full ITAD history too (up to itad.BundleHistoryYears).
	bundleIDs, err := db.GetTrackedBundleIDs(ctx)
	if err != nil {
		log.Fatalf("Failed to load tracked bundles: %v", err)
	}
	for _, id := range bundleIDs {
		added, err := itad.BackfillBundle(ctx, db, client, id, endDate.AddDate(-itad.BundleHistoryYears, 0, 0), endDate)
		if err != nil {
			log.Printf("Skipping bundle %d: %v", id, err)
			continue
		}
		log.Printf("Imported %d new days for bundle %d", added, id)

		time.Sleep(1 * time.Second)
	}
}
