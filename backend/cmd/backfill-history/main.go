package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/itad"
	"github.com/joho/godotenv"
)

// backfill-history pulls up to 5 years of historical Steam price-change
// events per tracked game from IsThereAnyDeal and writes a daily price_history
// row for each day in that window. Run it once (or whenever you add a new
// tracked game) - it's safe to re-run since price_history upserts on
// (app_id, recorded_date).
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

	cfg := config.LoadConfig()
	if cfg.ITADAPIKey == "" {
		log.Fatal("ITAD_API_KEY is not set - get a free key at https://isthereanydeal.com/apps/my/")
	}

	client := itad.New(cfg.ITADAPIKey)

	endDate := time.Now()
	startDate := endDate.AddDate(-5, 0, 0)

	for _, appID := range cfg.Steam.TrackedAppIDs {
		itadID, err := client.LookupGameID(appID)
		if err != nil {
			log.Printf("Skipping appID %d: %v", appID, err)
			continue
		}

		events, err := client.GetHistory(itadID)
		if err != nil {
			log.Printf("Skipping appID %d: %v", appID, err)
			continue
		}

		series := itad.BuildDailySeries(events, startDate, endDate)
		log.Printf("Backfilling %d days of price history for appID %d", len(series), appID)

		for _, point := range series {
			if err := db.UpsertPriceHistory(ctx, appID, point.Price, point.OriginalPrice, point.DiscountPercentage, point.Date); err != nil {
				log.Printf("Failed to upsert price history for appID %d on %s: %v", appID, point.Date.Format("2006-01-02"), err)
			}
		}

		time.Sleep(1 * time.Second) // stay well under ITAD's rate limit
	}
}
