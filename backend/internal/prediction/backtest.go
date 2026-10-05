package prediction

import (
	"math"
	"sort"
	"time"
)

// BacktestConfig controls a backtest. The zero value uses sensible defaults.
type BacktestConfig struct {
	MinHistoryDays int // history required before a cut-off is scored (default 180)
	StepDays       int // days between cut-offs (default 30)
	LookaheadDays  int // the window forecast and checked (default 90; must be 30, 90, 180, 365 or 730)
	Paths          int // simulated futures per forecast (default 400)
}

// Sample is one scored forecast: standing at Cutoff, the model gave P as the
// chance of a lower price within the look-ahead window, and Outcome says
// whether one really appeared.
type Sample struct {
	AppID   int
	Cutoff  time.Time
	P       float64
	Outcome bool
	OnSale  bool
}

// Bucket groups samples whose predicted probability falls in a range.
type Bucket struct {
	From, To      float64
	N             int
	MeanPredicted float64
	ObservedRate  float64
}

// BacktestReport scores the model against the simple alternative of always
// predicting the overall base rate.
type BacktestReport struct {
	Samples       []Sample
	BaseRate      float64
	Brier         float64 // mean squared error of the model's probabilities; lower is better
	BaselineBrier float64 // the same for a constant forecast of BaseRate
	// Skill is 1 - Brier/BaselineBrier: 0 means no better than the base rate,
	// 1 is perfect, negative is worse.
	Skill   float64
	Buckets []Bucket
}

// Backtest replays history. For each game it moves a cut-off through the
// series; at each one it forecasts using only data up to that day and checks
// the forecast against what happened in the following LookaheadDays.
func Backtest(series map[int][]Point, cfg BacktestConfig) BacktestReport {
	if cfg.MinHistoryDays <= 0 {
		cfg.MinHistoryDays = 180
	}
	if cfg.StepDays <= 0 {
		cfg.StepDays = 30
	}
	if cfg.LookaheadDays <= 0 {
		cfg.LookaheadDays = 90
	}
	if cfg.Paths <= 0 {
		cfg.Paths = 400
	}

	ids := make([]int, 0, len(series))
	for id := range series {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	var report BacktestReport
	for _, id := range ids {
		points := series[id]
		if len(points) == 0 {
			continue
		}
		var lastDate time.Time
		for _, p := range points {
			if d := truncate(p.Date); d.After(lastDate) {
				lastDate = d
			}
		}
		all := buildDays(points, lastDate)

		for i := cfg.MinHistoryDays; i+cfg.LookaheadDays < len(all); i += cfg.StepDays {
			cutoff := all[i].date
			var known []Point
			for _, p := range points {
				if !truncate(p.Date).After(cutoff) {
					known = append(known, p)
				}
			}
			f := Predict(id, known, "backtest", Options{Now: cutoff, Paths: cfg.Paths, Seed: 1})
			if f.Model == ModelInsufficient {
				continue
			}

			threshold := all[i].price * (1 - DropThreshold)
			outcome := false
			for j := i + 1; j <= i+cfg.LookaheadDays; j++ {
				if all[j].price <= threshold+1e-9 {
					outcome = true
					break
				}
			}
			report.Samples = append(report.Samples, Sample{
				AppID: id, Cutoff: cutoff, P: pAt(f, cfg.LookaheadDays), Outcome: outcome, OnSale: f.OnSale,
			})
		}
	}
	report.score()
	return report
}

func pAt(f Forecast, days int) float64 {
	for _, h := range f.Horizons {
		if h.Days == days {
			return h.PLower
		}
	}
	return interpolate(f, days, func(c CurvePoint) float64 { return c.PLowerBy })
}

func (r *BacktestReport) score() {
	n := float64(len(r.Samples))
	if n == 0 {
		return
	}
	hits := 0.0
	for _, s := range r.Samples {
		if s.Outcome {
			hits++
		}
	}
	r.BaseRate = hits / n
	for _, s := range r.Samples {
		y := 0.0
		if s.Outcome {
			y = 1
		}
		r.Brier += (s.P - y) * (s.P - y)
		r.BaselineBrier += (r.BaseRate - y) * (r.BaseRate - y)
	}
	r.Brier /= n
	r.BaselineBrier /= n
	if r.BaselineBrier > 0 {
		r.Skill = 1 - r.Brier/r.BaselineBrier
	}

	const width = 0.2
	for lo := 0.0; lo < 1-1e-9; lo += width {
		b := Bucket{From: lo, To: lo + width}
		var sumP, sumY float64
		for _, s := range r.Samples {
			last := lo+width >= 1-1e-9
			if s.P >= lo && (s.P < lo+width || (last && s.P <= 1)) {
				b.N++
				sumP += s.P
				if s.Outcome {
					sumY++
				}
			}
		}
		if b.N > 0 {
			b.MeanPredicted = math.Round(sumP/float64(b.N)*1000) / 1000
			b.ObservedRate = math.Round(sumY/float64(b.N)*1000) / 1000
		}
		r.Buckets = append(r.Buckets, b)
	}
}
