package prediction

import (
	"math"
	"testing"
)

func TestBacktestScoresARegularGameWell(t *testing.T) {
	// A sale every 60 days for three years is exactly what the model is built
	// to read, so it should clearly beat a constant guess.
	pts, _ := cyclic(1100, 30, 60, 10, 0.5, 20)
	report := Backtest(map[int][]Point{1: pts}, BacktestConfig{LookaheadDays: 30, StepDays: 15})

	if len(report.Samples) < 40 {
		t.Fatalf("only %d samples", len(report.Samples))
	}
	if report.Skill < 0.5 {
		t.Errorf("skill = %.2f (Brier %.3f vs baseline %.3f), want a clear win on perfectly regular sales",
			report.Skill, report.Brier, report.BaselineBrier)
	}
	if report.Brier >= report.BaselineBrier {
		t.Errorf("model Brier %.3f is not better than the baseline %.3f", report.Brier, report.BaselineBrier)
	}
}

func TestBacktestNeverPeeksAtTheFuture(t *testing.T) {
	// If the forecast could see the future, a game whose behaviour changes
	// would still be forecast perfectly. Sales occur every 60 days for the
	// first year and then stop for good: at the end the model, knowing only
	// the past, must still expect sales to continue, so it should be wrong
	// there (a high chance of a sale that never comes).
	var pts []Point
	regular, _ := cyclic(365, 30, 60, 10, 0.5, 20)
	pts = append(pts, regular...)
	for i := 365; i < 700; i++ {
		pts = append(pts, Point{Date: epoch.AddDate(0, 0, i), Price: 20, Regular: 20})
	}
	report := Backtest(map[int][]Point{1: pts}, BacktestConfig{LookaheadDays: 30, StepDays: 10, MinHistoryDays: 200})

	wrong := 0
	for _, s := range report.Samples {
		if s.Cutoff.After(epoch.AddDate(0, 0, 400)) && s.P > 0.5 && !s.Outcome {
			wrong++
		}
	}
	if wrong == 0 {
		t.Error("after sales stop the model should (wrongly) still predict some; it looks like it saw the future")
	}
}

func TestBacktestSkipsGamesWithTooLittleHistory(t *testing.T) {
	pts, _ := cyclic(150, 30, 60, 10, 0.5, 20)
	report := Backtest(map[int][]Point{1: pts, 2: nil}, BacktestConfig{})
	if len(report.Samples) != 0 || report.Skill != 0 || report.Brier != 0 {
		t.Errorf("report = %+v, want empty", report)
	}
}

func TestBacktestScoringArithmetic(t *testing.T) {
	r := BacktestReport{Samples: []Sample{
		{P: 0.9, Outcome: true}, {P: 0.8, Outcome: true}, {P: 0.1, Outcome: false}, {P: 0.2, Outcome: false},
	}}
	r.score()

	if r.BaseRate != 0.5 {
		t.Errorf("base rate = %v, want 0.5", r.BaseRate)
	}
	// (0.01 + 0.04 + 0.01 + 0.04) / 4 = 0.025; a constant 0.5 scores 0.25.
	if math.Abs(r.Brier-0.025) > 1e-9 || math.Abs(r.BaselineBrier-0.25) > 1e-9 {
		t.Errorf("Brier %v baseline %v, want 0.025 and 0.25", r.Brier, r.BaselineBrier)
	}
	if math.Abs(r.Skill-0.9) > 1e-9 {
		t.Errorf("skill = %v, want 1 - 0.025/0.25 = 0.9", r.Skill)
	}

	total := 0
	for _, b := range r.Buckets {
		total += b.N
	}
	if total != 4 || len(r.Buckets) != 5 {
		t.Errorf("buckets hold %d samples across %d buckets", total, len(r.Buckets))
	}
	if top := r.Buckets[4]; top.N != 2 || top.ObservedRate != 1 || top.MeanPredicted != 0.85 {
		t.Errorf("top bucket = %+v", top)
	}
	// 0.1 falls in [0, 0.2); 0.2 belongs to the next bucket, [0.2, 0.4).
	if low := r.Buckets[0]; low.N != 1 || low.ObservedRate != 0 || low.MeanPredicted != 0.1 {
		t.Errorf("bottom bucket = %+v", low)
	}
	if second := r.Buckets[1]; second.N != 1 || second.MeanPredicted != 0.2 {
		t.Errorf("second bucket = %+v", second)
	}
}

func TestBacktestIsDeterministic(t *testing.T) {
	pts, _ := cyclic(600, 30, 60, 10, 0.5, 20)
	a := Backtest(map[int][]Point{1: pts}, BacktestConfig{})
	b := Backtest(map[int][]Point{1: pts}, BacktestConfig{})
	if a.Brier != b.Brier || len(a.Samples) != len(b.Samples) {
		t.Error("the same data produced different backtests")
	}
}

func TestBacktestSkipsFreeGames(t *testing.T) {
	// For a $0 game "a price at least 5% below $0" is trivially true, which once
	// made free games look like perfect misses. They must be left out entirely.
	var free []Point
	for i := 0; i < 600; i++ {
		free = append(free, Point{Date: epoch.AddDate(0, 0, i), Price: 0, Regular: 0})
	}
	if r := Backtest(map[int][]Point{1: free}, BacktestConfig{}); len(r.Samples) != 0 {
		t.Errorf("a free game produced %d samples", len(r.Samples))
	}
}

func TestBacktestOutcomeNeedsAStrictlyLowerPrice(t *testing.T) {
	// A game that never changes price never gets a lower one: every outcome is false.
	var flat []Point
	for i := 0; i < 700; i++ {
		flat = append(flat, Point{Date: epoch.AddDate(0, 0, i), Price: 20, Regular: 20})
	}
	r := Backtest(map[int][]Point{1: flat}, BacktestConfig{LookaheadDays: 30})
	if len(r.Samples) == 0 {
		t.Fatal("no samples")
	}
	for _, s := range r.Samples {
		if s.Outcome {
			t.Fatalf("a flat-priced game cannot see a lower price, but %v was scored as one", s.Cutoff)
		}
	}
}
