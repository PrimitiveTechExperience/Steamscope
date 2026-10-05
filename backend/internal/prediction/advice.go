package prediction

import (
	"fmt"
	"math"
	"time"
)

// Verdicts reported in Advice.Verdict.
const (
	VerdictBuyNow  = "buy_now"
	VerdictWait    = "wait"
	VerdictTossUp  = "toss_up"
	VerdictNoData  = "not_enough_data"
	buyThreshold   = 60
	waitThreshold  = 40
	baseScore      = 50
	defaultPatient = 90
)

// AdviceInput is what is known about the person asking, beyond the game itself.
type AdviceInput struct {
	// TargetPrice is the price the user said they would buy at (nil if unset).
	TargetPrice *float64
	// Watching is true when the game is on the user's watchlist.
	Watching bool
	// DaysWatched is how long it has been watched (meaningful when Watching).
	DaysWatched int
	// WatchStartPrice is the price on the day watching began (nil if unknown).
	WatchStartPrice *float64
}

// Reason is one factor behind the verdict. Impact is the number of points it
// added to (or, when negative, took from) the buy-now score.
type Reason struct {
	Code   string `json:"code"`
	Text   string `json:"text"`
	Impact int    `json:"impact"`
}

// Advice is the buy-now-or-wait call for one game and one person.
type Advice struct {
	Verdict string `json:"verdict"`
	// Score is 0-100: high means buy now, low means wait.
	Score int `json:"score"`
	// Confidence is 0-1: how far the score is from a toss-up, scaled by how
	// much evidence the forecast rests on.
	Confidence float64  `json:"confidence"`
	Reasons    []Reason `json:"reasons"`
	// PatienceDays is how far ahead the call looked; it shrinks the longer the
	// user has already been watching.
	PatienceDays int `json:"patience_days"`
	// ChanceOfLower is the chance of a lower price within PatienceDays.
	ChanceOfLower float64 `json:"chance_of_lower"`
	// ExpectedSavingPercent is the expected extra saving from waiting that long.
	ExpectedSavingPercent float64 `json:"expected_saving_percent"`
	// WaitUntil is the likely date of the next sale (only for a "wait" verdict).
	WaitUntil    *string `json:"wait_until"`
	Personalized bool    `json:"personalized"`
}

// patience is how many days ahead to look. Someone who has already waited a
// long time without a drop has shown less patience.
func patience(in AdviceInput) int {
	if !in.Watching {
		return defaultPatient
	}
	switch {
	case in.DaysWatched > 240:
		return 30
	case in.DaysWatched > 120:
		return 45
	case in.DaysWatched > 60:
		return 60
	}
	return defaultPatient
}

// Advise turns a forecast into a buy-now-or-wait call.
//
// The score starts at 50 and each rule below moves it, with the push from
// forecast-based rules scaled down when the forecast is weak. The user's own
// target price is never scaled: it is a fact, not an estimate.
func Advise(f Forecast, in AdviceInput) Advice {
	a := Advice{Reasons: []Reason{}, PatienceDays: patience(in), Personalized: in.Watching}
	score := float64(baseScore)
	add := func(code, text string, impact float64) {
		score += impact
		a.Reasons = append(a.Reasons, Reason{Code: code, Text: text, Impact: int(math.Round(impact))})
	}
	price := f.CurrentPrice

	// The user's own target.
	targetMet := false
	if in.TargetPrice != nil && price > 0 {
		if price <= *in.TargetPrice+1e-9 {
			targetMet = true
			add("target_met", fmt.Sprintf("The price ($%.2f) is at or below your target of $%.2f", price, *in.TargetPrice), 40)
		} else {
			gap := (price - *in.TargetPrice) / price
			add("above_target", fmt.Sprintf("The price is %.0f%% above your target of $%.2f", 100*gap, *in.TargetPrice), -math.Min(15, gap*60))
		}
	}

	if f.Model == ModelInsufficient {
		a.Verdict = VerdictNoData
		if targetMet {
			a.Verdict = VerdictBuyNow
		}
		a.Score = clampScore(score)
		return a
	}

	shrink := 0.5 + 0.5*f.Confidence // weak forecasts push less
	addForecast := func(code, text string, impact float64) { add(code, text, impact*shrink) }

	// How close the price is to the lowest on record.
	if f.HistoricLow > 0 && price > 0 {
		ratio := price / f.HistoricLow
		switch {
		case ratio <= 1.02:
			addForecast("at_historic_low", fmt.Sprintf("Within 2%% of the lowest price on record ($%.2f)", f.HistoricLow), 30)
		case ratio <= 1.10:
			addForecast("near_historic_low", fmt.Sprintf("Within 10%% of the lowest price on record ($%.2f)", f.HistoricLow), 15)
		case ratio >= 1.5:
			addForecast("far_above_low", fmt.Sprintf("%.0f%% above the lowest price on record ($%.2f)", 100*(ratio-1), f.HistoricLow), -10)
		}
	}

	// How good today's discount is for this game.
	if f.Typical.MedianDepthPercent > 0 {
		switch {
		case f.CurrentDisc == 0:
			addForecast("full_price", fmt.Sprintf("Not on sale; its usual sale is %.0f%% off", f.Typical.MedianDepthPercent), -15)
		case float64(f.CurrentDisc) >= f.Typical.MedianDepthPercent:
			addForecast("deep_discount", fmt.Sprintf("%d%% off matches or beats its usual sale of %.0f%%", f.CurrentDisc, f.Typical.MedianDepthPercent), 15)
		case float64(f.CurrentDisc) < f.Typical.MedianDepthPercent/2:
			addForecast("shallow_discount", fmt.Sprintf("Only %d%% off; its usual sale is %.0f%%", f.CurrentDisc, f.Typical.MedianDepthPercent), -10)
		}
	}

	// The chance, and size, of a better price within the patience window.
	pBetter := interpolate(f, a.PatienceDays, func(c CurvePoint) float64 { return c.PLowerBy })
	expectedLow := interpolateHorizon(f, a.PatienceDays)
	saving := 0.0
	if price > 0 && expectedLow > 0 {
		saving = math.Max(0, (price-expectedLow)/price)
	}
	a.ChanceOfLower = round4(pBetter)
	a.ExpectedSavingPercent = round2(100 * saving)
	switch {
	case pBetter >= 0.5:
		pressure := pBetter * math.Min(1, saving/0.20)
		addForecast("better_price_likely",
			fmt.Sprintf("%.0f%% chance of a lower price within %d days (about %.0f%% cheaper on average)", 100*pBetter, a.PatienceDays, 100*saving),
			-40*pressure)
	case pBetter < 0.25:
		addForecast("unlikely_to_drop",
			fmt.Sprintf("Only a %.0f%% chance of a lower price within %d days", 100*pBetter, a.PatienceDays),
			20*(0.25-pBetter)/0.25)
	}

	// How long, and at what price, the user has been watching.
	if in.Watching {
		if in.DaysWatched >= 30 {
			add("waited_long", fmt.Sprintf("You have watched this game for %d days", in.DaysWatched),
				math.Min(12, math.Round(float64(in.DaysWatched)/30)*2))
		}
		if in.WatchStartPrice != nil && *in.WatchStartPrice > 0 && price > 0 {
			switch {
			case price <= *in.WatchStartPrice*0.9:
				add("cheaper_than_when_watched", fmt.Sprintf("Cheaper than when you started watching ($%.2f)", *in.WatchStartPrice), 10)
			case price >= *in.WatchStartPrice*1.05:
				add("pricier_than_when_watched", fmt.Sprintf("Pricier than when you started watching ($%.2f)", *in.WatchStartPrice), -5)
			}
		}
	}

	a.Score = clampScore(score)
	switch {
	case a.Score >= buyThreshold:
		a.Verdict = VerdictBuyNow
	case a.Score <= waitThreshold:
		a.Verdict = VerdictWait
	default:
		a.Verdict = VerdictTossUp
	}
	if targetMet && a.Verdict != VerdictBuyNow {
		// The user named this price themselves, so it overrides the estimate.
		// The reasons above still show that a lower price is possible.
		a.Reasons = append(a.Reasons, Reason{Code: "target_overrides",
			Text: "A lower price is possible, but this meets the target you set"})
		a.Verdict = VerdictBuyNow
		a.Score = max(a.Score, buyThreshold)
	}
	a.Confidence = round2(math.Min(1, math.Abs(float64(a.Score)-baseScore)/baseScore) * (0.5 + 0.5*f.Confidence))

	if a.Verdict == VerdictWait && f.Next.MedianDays != nil {
		d := f.GeneratedAt.AddDate(0, 0, *f.Next.MedianDays).Format(time.DateOnly)
		a.WaitUntil = &d
	}
	return a
}

func clampScore(s float64) int {
	return int(math.Max(0, math.Min(100, math.Round(s))))
}

// interpolate reads a curve value at an arbitrary number of days ahead.
func interpolate(f Forecast, days int, value func(CurvePoint) float64) float64 {
	if len(f.Curve) == 0 {
		return 0
	}
	prev := CurvePoint{}
	for _, c := range f.Curve {
		if c.DaysAhead >= days {
			if c.DaysAhead == prev.DaysAhead {
				return value(c)
			}
			frac := float64(days-prev.DaysAhead) / float64(c.DaysAhead-prev.DaysAhead)
			return value(prev) + frac*(value(c)-value(prev))
		}
		prev = c
	}
	return value(f.Curve[len(f.Curve)-1])
}

// interpolateHorizon reads the expected lowest price within a window.
func interpolateHorizon(f Forecast, days int) float64 {
	if len(f.Horizons) == 0 {
		return 0
	}
	prev := Horizon{Days: 0, ExpectedLow: f.CurrentPrice}
	for _, h := range f.Horizons {
		if h.Days >= days {
			frac := float64(days-prev.Days) / float64(h.Days-prev.Days)
			return prev.ExpectedLow + frac*(h.ExpectedLow-prev.ExpectedLow)
		}
		prev = h
	}
	return f.Horizons[len(f.Horizons)-1].ExpectedLow
}
