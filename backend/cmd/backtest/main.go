// Command backtest measures how good the price forecast is on real data.
//
// For every tracked game it replays history: standing at past dates it
// forecasts the following days using only earlier data, then checks what
// happened. It reports the Brier score against a constant "base rate" guess
// and a calibration table (when the model says 30%, does it happen 30% of the
// time?).
//
//	go run ./backend/cmd/backtest [-days 90] [-min 180] [-step 30] [-itad]
//	go run ./backend/cmd/backtest -audit
//
// Run it from the repository root so .env is found.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/itad"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/prediction"
	"github.com/joho/godotenv"
)

func main() {
	lookahead := flag.Int("days", 90, "forecast window to score: 30, 90, 180, 365 or 730")
	minHistory := flag.Int("min", 180, "days of history required before a cut-off is scored")
	step := flag.Int("step", 30, "days between cut-offs")
	runAudit := flag.Bool("audit", false, "compare the model's chance of a lower price with ITAD's own record (needs ITAD_API_KEY)")
	useITAD := flag.Bool("itad", false, "extend each history with ITAD's longer log (needs ITAD_API_KEY)")
	flag.Parse()

	godotenv.Load()
	ctx := context.Background()
	db, err := database.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()
	cfg := config.LoadConfig()

	var client *itad.Client
	if *useITAD || *runAudit {
		if cfg.ITADAPIKey == "" {
			log.Fatal("-itad needs ITAD_API_KEY")
		}
		client = itad.New(cfg.ITADAPIKey)
	}

	ids, err := db.GetTrackedAppIDs(ctx)
	if err != nil {
		log.Fatalf("tracked games: %v", err)
	}
	if *runAudit {
		audit(ctx, db, client, ids)
		return
	}
	series := map[int][]prediction.Point{}
	for _, id := range ids {
		recorded, err := db.GetPriceHistory(ctx, id)
		if err != nil || len(recorded) == 0 {
			continue
		}
		var points []prediction.Point
		if client != nil {
			points = extend(ctx, client, id, recorded[0].Date)
			time.Sleep(time.Second) // stay under ITAD's rate limit
		}
		for _, p := range recorded {
			points = append(points, prediction.Point{Date: p.Date, Price: p.Price, Regular: p.OriginalPrice})
		}
		series[id] = points
	}

	fmt.Printf("Backtesting %d games, scoring \"is a price at least 5%% lower within %d days?\"\n\n", len(series), *lookahead)
	report := prediction.Backtest(series, prediction.BacktestConfig{LookaheadDays: *lookahead, MinHistoryDays: *minHistory, StepDays: *step})
	if len(report.Samples) == 0 {
		fmt.Println("Not enough history to score anything.")
		return
	}

	fmt.Printf("Samples:            %d\n", len(report.Samples))
	fmt.Printf("Base rate:          %.3f  (how often a lower price really appeared)\n", report.BaseRate)
	fmt.Printf("Brier (model):      %.4f\n", report.Brier)
	fmt.Printf("Brier (base rate):  %.4f\n", report.BaselineBrier)
	fmt.Printf("Skill:              %+.3f  (0 = no better than guessing the base rate, 1 = perfect)\n\n", report.Skill)

	fmt.Println("Calibration (predicted vs observed):")
	for _, b := range report.Buckets {
		if b.N == 0 {
			continue
		}
		fmt.Printf("  %.0f%%-%.0f%%   n=%-4d predicted %.3f   observed %.3f\n", b.From*100, b.To*100, b.N, b.MeanPredicted, b.ObservedRate)
	}

	var onSale, offSale []prediction.Sample
	for _, s := range report.Samples {
		if s.OnSale {
			onSale = append(onSale, s)
		} else {
			offSale = append(offSale, s)
		}
	}
	fmt.Printf("\nOn sale at the cut-off: %d samples, not on sale: %d samples\n", len(onSale), len(offSale))
}

// extend returns ITAD's daily history for dates before `before`.
func extend(ctx context.Context, c *itad.Client, appID int, before time.Time) []prediction.Point {
	events, err := c.History(ctx, appID)
	if err != nil {
		log.Printf("app %d: ITAD unavailable: %v", appID, err)
		return nil
	}
	var points []prediction.Point
	for _, p := range itad.BuildDailySeries(events, time.Now().AddDate(-6, 0, 0), time.Now()) {
		if p.Date.Before(before) {
			points = append(points, prediction.Point{Date: p.Date, Price: p.Price, Regular: p.OriginalPrice})
		}
	}
	return points
}
