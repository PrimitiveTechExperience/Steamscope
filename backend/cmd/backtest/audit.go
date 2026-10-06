package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/itad"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/prediction"
)

// audit compares the model's "chance of a lower price within 90 days" with
// what ITAD's own history says, independently of the model: over every 90-day
// window in the last 6 (and last 2) years, how often did the price reach at
// least 5% below today's price? A model that says 0% where ITAD shows a price
// that low turning up now and then is suspect.
func audit(ctx context.Context, db *database.DB, client *itad.Client, ids []int) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	fmt.Printf("%-26s %7s %8s %8s | %-9s %8s | %-14s %-14s | %s\n",
		"game", "now", "ITAD low", "deepest", "model", "next sale", "ITAD 90d freq", "(last 2y)", "verdict")
	for _, id := range ids {
		recorded, err := db.GetPriceHistory(ctx, id)
		if err != nil || len(recorded) == 0 {
			continue
		}
		name, _ := db.GetGameName(ctx, id)
		events, err := client.History(ctx, id)
		if err != nil || len(events) == 0 {
			fmt.Printf("%-26s (no ITAD history: %v)\n", trunc(name, 26), err)
			time.Sleep(time.Second)
			continue
		}

		// The model sees what the live API gives it: recorded history plus ITAD before it.
		var points []prediction.Point
		for _, p := range itad.BuildDailySeries(events, today.AddDate(-6, 0, 0), today) {
			if p.Date.Before(recorded[0].Date) {
				points = append(points, prediction.Point{Date: p.Date, Price: p.Price, Regular: p.OriginalPrice})
			}
		}
		for _, p := range recorded {
			points = append(points, prediction.Point{Date: p.Date, Price: p.Price, Regular: p.OriginalPrice})
		}
		low, deepest := paidLowAndDeepest(events)
		f := prediction.Predict(id, points, "audit", prediction.Options{Now: today, AllTimeLow: low})
		if f.Model == prediction.ModelInsufficient {
			continue
		}
		if f.Model == prediction.ModelFree {
			fmt.Printf("%-26s %7s %8s %8s | %-9s %8s | %-14s %-14s | free to play: nothing to wait for\n",
				trunc(name, 26), "free", "-", "-", "-", "-", "-", "-")
			time.Sleep(time.Second)
			continue
		}

		// The reference, from ITAD alone.
		series := itad.BuildDailySeries(events, today.AddDate(-6, 0, 0), today)
		price := f.CurrentPrice
		f6, n6 := windowFrequency(series, price, 90, 0)
		f2, n2 := windowFrequency(series, price, 90, 730)

		modelP := f.Horizons[1].PLower
		next := "-"
		if f.Next.MedianDays != nil {
			next = fmt.Sprintf("%dd", *f.Next.MedianDays)
		}
		fmt.Printf("%-26s %7.2f %8.2f %7d%% | %-9s %8s | %-14s %-14s | %s\n",
			trunc(name, 26), price, low, deepest, fmt.Sprintf("%.0f%%", 100*modelP), next,
			fmt.Sprintf("%.0f%% (n=%d)", 100*f6, n6), fmt.Sprintf("%.0f%% (n=%d)", 100*f2, n2), judge(modelP, f6, f2, price, low))
		time.Sleep(time.Second)
	}
}

// windowFrequency is the share of 90-day windows (starting each day, within
// the last lookbackDays days or the whole series when 0) in which the price
// reached 5% or more below `price`.
func windowFrequency(series []models.PricePoint, price float64, window, lookbackDays int) (float64, int) {
	threshold := price * (1 - prediction.DropThreshold)
	start := 0
	if lookbackDays > 0 && len(series) > lookbackDays {
		start = len(series) - lookbackDays
	}
	hits, n := 0, 0
	for i := start; i+window < len(series); i++ {
		n++
		for j := i + 1; j <= i+window; j++ {
			if series[j].Price <= threshold+1e-9 {
				hits++
				break
			}
		}
	}
	if n == 0 {
		return 0, 0
	}
	return float64(hits) / float64(n), n
}

// judge flags predictions that disagree with ITAD's own record.
func judge(model, itad6, itad2, price, low float64) string {
	switch {
	case price <= low*1.0001:
		return "at the all-time low: nothing lower is possible (0% is right)"
	case model == 0 && itad6 == 0:
		return "agree: ITAD shows no price that low in 6 years"
	case model == 0 && itad6 > 0:
		return fmt.Sprintf("CHECK: model says 0%% but ITAD saw it in %.0f%% of windows", 100*itad6)
	case math.Abs(model-itad6) <= 0.15 || math.Abs(model-itad2) <= 0.15:
		return "consistent with ITAD"
	case model > math.Max(itad6, itad2):
		return "model more optimistic than ITAD's record"
	default:
		return "model more pessimistic than ITAD's record"
	}
}

func paidLowAndDeepest(events []itad.HistoryEvent) (float64, int) {
	var prices []float64
	deepest := 0
	for _, e := range events {
		if e.Price <= 0 {
			continue
		}
		prices = append(prices, e.Price)
		if e.Cut > deepest && e.Cut < 100 {
			deepest = e.Cut
		}
	}
	sort.Float64s(prices)
	if len(prices) == 0 {
		return 0, deepest
	}
	return prices[0], deepest
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "~"
}
