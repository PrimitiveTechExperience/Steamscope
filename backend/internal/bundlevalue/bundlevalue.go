// Package bundlevalue judges whether a Steam bundle is worth buying.
//
// A bundle's own "discount" is measured against the games' prices on the day
// the page was read, which may already be sale prices. This package compares
// the bundle's price with three other things: buying every game separately at
// today's prices, buying them at their regular prices, and the bundle's own
// price history. It also flags games that cost less on their own than their
// fair share of the bundle, for anyone who only wants some of them.
//
// The verdict is a transparent score: each rule that applies is returned with
// its point impact so the interface can say why.
package bundlevalue

import (
	"fmt"
	"math"
)

// Verdicts reported in Value.Verdict.
const (
	GreatDeal  = "great_deal"
	GoodDeal   = "good_deal"
	Fair       = "fair"
	PoorValue  = "poor_value"
	NoData     = "not_enough_data"
	greatScore = 75
	goodScore  = 60
	fairScore  = 40
	baseScore  = 50
	// minPriced is how many games must have a known price before judging.
	minPriced = 2
)

// Item is one game in the bundle. A Regular of 0 means the price is unknown.
type Item struct {
	AppID   int
	Name    string
	Price   float64 // today's price
	Regular float64 // undiscounted price
}

// Input is everything needed to judge one bundle.
type Input struct {
	BundlePrice     float64
	DiscountPercent int
	Items           []Item
	// Prices is the bundle's own recorded daily price history, oldest first.
	Prices []float64
}

// ItemValue is how one game fares inside the bundle.
type ItemValue struct {
	AppID           int     `json:"app_id"`
	Name            string  `json:"name"`
	Price           float64 `json:"price"`
	Regular         float64 `json:"regular_price"`
	DiscountPercent int     `json:"discount_percent"`
	// BundleShare is the part of the bundle price this game accounts for,
	// splitting the price in proportion to the games' regular prices.
	BundleShare float64 `json:"bundle_share"`
	// CheaperAlone is true when buying just this game now costs less than its share.
	CheaperAlone bool `json:"cheaper_alone"`
	Priced       bool `json:"priced"`
}

// Totals are the sums the verdict rests on, over the games with a known price.
type Totals struct {
	Regular     float64 `json:"regular"`
	Separate    float64 `json:"separate"`
	Bundle      float64 `json:"bundle"`
	PricedItems int     `json:"priced_items"`
	Items       int     `json:"items"`
}

// Reason is one factor behind the verdict.
type Reason struct {
	Code   string `json:"code"`
	Text   string `json:"text"`
	Impact int    `json:"impact"`
}

// Value is the assessment of a bundle.
type Value struct {
	Verdict string `json:"verdict"`
	// Score is 0-100: high means a good deal.
	Score   int      `json:"score"`
	Reasons []Reason `json:"reasons"`
	Totals  Totals   `json:"totals"`

	// Savings are negative when the bundle costs more.
	SavingsVsSeparate        float64 `json:"savings_vs_separate"`
	SavingsVsSeparatePercent float64 `json:"savings_vs_separate_percent"`
	SavingsVsRegular         float64 `json:"savings_vs_regular"`
	SavingsVsRegularPercent  float64 `json:"savings_vs_regular_percent"`

	// Completeness is the share of games whose price was known (0-1).
	Completeness float64 `json:"completeness"`
	// AtRecordLow is true when the bundle is discounted and at the lowest
	// price recorded for it. RecordLow is that price (0 if there is no history).
	AtRecordLow bool    `json:"at_record_low"`
	RecordLow   float64 `json:"record_low"`

	CheaperAloneCount int         `json:"cheaper_alone_count"`
	Items             []ItemValue `json:"items"`
}

// Evaluate assesses a bundle.
func Evaluate(in Input) Value {
	v := Value{Reasons: []Reason{}, Items: []ItemValue{}}
	v.Totals.Bundle = round2(in.BundlePrice)
	v.Totals.Items = len(in.Items)

	// Record low, from the bundle's own history.
	v.RecordLow, v.AtRecordLow = recordLow(in.Prices, in.BundlePrice, in.DiscountPercent)

	// Totals and per-game values over the games with a known price.
	var regularSum, separateSum float64
	for _, it := range in.Items {
		iv := ItemValue{AppID: it.AppID, Name: it.Name, Price: round2(it.Price), Regular: round2(it.Regular)}
		if it.Regular > 0 {
			iv.Priced = true
			regularSum += it.Regular
			separateSum += it.Price
			v.Totals.PricedItems++
			if it.Regular > it.Price {
				iv.DiscountPercent = int(math.Round(100 * (it.Regular - it.Price) / it.Regular))
			}
		}
		v.Items = append(v.Items, iv)
	}
	v.Totals.Regular, v.Totals.Separate = round2(regularSum), round2(separateSum)
	if v.Totals.Items > 0 {
		v.Completeness = round4(float64(v.Totals.PricedItems) / float64(v.Totals.Items))
	}

	if in.BundlePrice <= 0 || v.Totals.PricedItems < minPriced || regularSum <= 0 {
		v.Verdict = NoData
		v.Score = baseScore
		return v
	}

	// Each game's fair share of the bundle price, by regular price.
	for i := range v.Items {
		if !v.Items[i].Priced {
			continue
		}
		v.Items[i].BundleShare = round2(in.BundlePrice * v.Items[i].Regular / regularSum)
		if v.Items[i].Price+0.005 < v.Items[i].BundleShare {
			v.Items[i].CheaperAlone = true
			v.CheaperAloneCount++
		}
	}

	v.SavingsVsSeparate = round2(separateSum - in.BundlePrice)
	v.SavingsVsRegular = round2(regularSum - in.BundlePrice)
	if separateSum > 0 {
		v.SavingsVsSeparatePercent = round2(100 * (separateSum - in.BundlePrice) / separateSum)
	}
	v.SavingsVsRegularPercent = round2(100 * (regularSum - in.BundlePrice) / regularSum)

	score := float64(baseScore)
	// When some prices are unknown the comparison covers only part of the
	// bundle, so every rule counts for proportionally less.
	weight := v.Completeness
	add := func(code, text string, impact float64) {
		impact *= weight
		score += impact
		v.Reasons = append(v.Reasons, Reason{Code: code, Text: text, Impact: int(math.Round(impact))})
	}

	// 1. Versus buying every game separately at today's prices.
	sep := (separateSum - in.BundlePrice) / math.Max(separateSum, 0.01)
	switch {
	case separateSum == 0:
		// The games are all free separately; the bundle can't beat that.
	case sep >= 0.05:
		add("cheaper_than_separate",
			fmt.Sprintf("Buying these games separately today costs $%.2f; the bundle is $%.2f, saving $%.2f (%.0f%%)", separateSum, in.BundlePrice, separateSum-in.BundlePrice, 100*sep),
			math.Min(30, 100*sep))
	case sep < -0.02:
		add("dearer_than_separate",
			fmt.Sprintf("The bundle ($%.2f) costs more than buying the games separately today ($%.2f)", in.BundlePrice, separateSum),
			-math.Min(30, -100*sep))
	default:
		add("about_same_as_separate",
			fmt.Sprintf("About the same as buying the games separately today ($%.2f)", separateSum), 0)
	}

	// 2. Versus their regular prices.
	reg := (regularSum - in.BundlePrice) / regularSum
	switch {
	case reg >= 0.6:
		add("deep_vs_regular", fmt.Sprintf("%.0f%% below the games' regular prices ($%.2f)", 100*reg, regularSum), 15)
	case reg >= 0.4:
		add("good_vs_regular", fmt.Sprintf("%.0f%% below the games' regular prices ($%.2f)", 100*reg, regularSum), 10)
	case reg >= 0.2:
		add("some_vs_regular", fmt.Sprintf("%.0f%% below the games' regular prices ($%.2f)", 100*reg, regularSum), 5)
	case reg < 0.1:
		add("small_vs_regular", fmt.Sprintf("Only %.0f%% below the games' regular prices ($%.2f)", math.Max(0, 100*reg), regularSum), -10)
	}

	// 3. The bundle's own history.
	if v.RecordLow > 0 {
		switch {
		case v.AtRecordLow:
			add("bundle_record_low", fmt.Sprintf("This is the lowest price recorded for this bundle ($%.2f)", v.RecordLow), 15)
		case in.DiscountPercent > 0 && in.BundlePrice <= v.RecordLow*1.05 && distinct(in.Prices) >= 2:
			add("near_bundle_record_low", fmt.Sprintf("Within 5%% of the lowest price recorded for this bundle ($%.2f)", v.RecordLow), 7)
		case in.BundlePrice >= v.RecordLow*1.25 && distinct(in.Prices) >= 2:
			add("above_bundle_record_low", fmt.Sprintf("%.0f%% above the lowest price recorded for this bundle ($%.2f)", 100*(in.BundlePrice/v.RecordLow-1), v.RecordLow), -10)
		}
	}

	// 4. Games that are cheaper on their own.
	if v.Totals.PricedItems > 0 && 2*v.CheaperAloneCount >= v.Totals.PricedItems && v.CheaperAloneCount > 0 {
		add("most_cheaper_alone",
			fmt.Sprintf("%d of %d games cost less on their own today than their share of the bundle", v.CheaperAloneCount, v.Totals.PricedItems), -10)
	}

	if v.Totals.PricedItems < v.Totals.Items {
		v.Reasons = append(v.Reasons, Reason{Code: "some_prices_unknown",
			Text: fmt.Sprintf("Prices for %d of %d games were not available, so this covers only part of the bundle", v.Totals.Items-v.Totals.PricedItems, v.Totals.Items)})
	}

	v.Score = int(math.Max(0, math.Min(100, math.Round(score))))
	switch {
	case v.Score >= greatScore:
		v.Verdict = GreatDeal
	case v.Score >= goodScore:
		v.Verdict = GoodDeal
	case v.Score >= fairScore:
		v.Verdict = Fair
	default:
		v.Verdict = PoorValue
	}
	return v
}

// recordLow returns the lowest recorded price and whether today's bundle price
// is a record low. It needs a real discount and at least two distinct recorded
// prices: a bundle that has never changed price has no record to speak of.
func recordLow(prices []float64, current float64, discountPercent int) (low float64, atLow bool) {
	for _, p := range prices {
		if p > 0 && (low == 0 || p < low) {
			low = p
		}
	}
	if low == 0 {
		return 0, false
	}
	return round2(low), current > 0 && discountPercent > 0 && distinct(prices) >= 2 && current <= low+0.005
}

func distinct(prices []float64) int {
	seen := map[float64]bool{}
	for _, p := range prices {
		seen[round2(p)] = true
	}
	return len(seen)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
