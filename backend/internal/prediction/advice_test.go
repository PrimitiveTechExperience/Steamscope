package prediction

import (
	"strings"
	"testing"
	"time"
)

// fc builds a forecast by hand so each advice rule can be tested on its own.
// pLower is the chance of a lower price within every horizon; expLow is the
// expected lowest price in the 90-day window.
type fcOpts struct {
	price, low, typicalDepth float64
	disc                     int
	pLower, expLow           float64
	confidence               float64
	nextMedian               *int
}

func fc(o fcOpts) Forecast {
	if o.confidence == 0 {
		o.confidence = 1
	}
	var curve []CurvePoint
	for _, d := range []int{1, 30, 45, 60, 90, 180, 365, 730} {
		curve = append(curve, CurvePoint{DaysAhead: d, PLowerBy: o.pLower * float64(min(d, 90)) / 90})
	}
	f := Forecast{
		Model: ModelWeibull, CurrentPrice: o.price, HistoricLow: o.low, CurrentDisc: o.disc, Confidence: o.confidence,
		GeneratedAt: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		Typical:     TypicalSale{MedianDepthPercent: o.typicalDepth},
		Curve:       curve,
		Next:        NextSale{MedianDays: o.nextMedian},
	}
	for _, d := range []int{30, 90, 180, 365, 730} {
		f.Horizons = append(f.Horizons, Horizon{Days: d, ExpectedLow: o.expLow})
	}
	return f
}

func f64(v float64) *float64 { return &v }
func i(v int) *int           { return &v }

func reasonCodes(a Advice) []string {
	var out []string
	for _, r := range a.Reasons {
		out = append(out, r.Code)
	}
	return out
}

func has(a Advice, code string) bool {
	for _, c := range reasonCodes(a) {
		if c == code {
			return true
		}
	}
	return false
}

func impact(a Advice, code string) int {
	for _, r := range a.Reasons {
		if r.Code == code {
			return r.Impact
		}
	}
	return 0
}

func TestAdviceBuyNowAtHistoricLow(t *testing.T) {
	// $10 is the lowest price on record and at its usual 50% discount, with
	// little chance of going lower.
	a := Advise(fc(fcOpts{price: 10, low: 10, disc: 50, typicalDepth: 50, pLower: 0.05, expLow: 9.9}), AdviceInput{})
	if a.Verdict != VerdictBuyNow {
		t.Fatalf("verdict = %s (score %d, %v), want buy_now", a.Verdict, a.Score, reasonCodes(a))
	}
	for _, code := range []string{"at_historic_low", "deep_discount", "unlikely_to_drop"} {
		if !has(a, code) {
			t.Errorf("missing reason %s in %v", code, reasonCodes(a))
		}
	}
	if impact(a, "at_historic_low") != 30 || impact(a, "deep_discount") != 15 {
		t.Errorf("impacts: %+v", a.Reasons)
	}
	if a.Score != 100 { // 50 + 30 + 15 + 16 would be 111
		t.Errorf("score = %d, want it clamped to 100", a.Score)
	}
}

func TestAdviceWaitWhenASaleIsLikelyAndTheGameIsFullPrice(t *testing.T) {
	// Full price $20, usual sale 50% off ($10), a sale very likely within 90 days.
	f := fc(fcOpts{price: 20, low: 10, disc: 0, typicalDepth: 50, pLower: 0.9, expLow: 11, nextMedian: i(21)})
	a := Advise(f, AdviceInput{})

	if a.Verdict != VerdictWait {
		t.Fatalf("verdict = %s (score %d, %v), want wait", a.Verdict, a.Score, reasonCodes(a))
	}
	for _, code := range []string{"full_price", "better_price_likely", "far_above_low"} {
		if !has(a, code) {
			t.Errorf("missing reason %s in %v", code, reasonCodes(a))
		}
	}
	if a.PatienceDays != 90 || a.ChanceOfLower != 0.9 {
		t.Errorf("patience %d, chance %.2f", a.PatienceDays, a.ChanceOfLower)
	}
	if a.ExpectedSavingPercent != 45 {
		t.Errorf("expected saving = %.2f%%, want 45%% ($20 -> $11)", a.ExpectedSavingPercent)
	}
	if a.WaitUntil == nil || *a.WaitUntil != "2026-10-26" {
		t.Errorf("wait until = %v, want 2026-10-26 (21 days out)", a.WaitUntil)
	}
	if a.Personalized {
		t.Error("an anonymous visitor's advice is not personalized")
	}
}

func TestAdviceOnlyNamesAWaitDateWhenTheVerdictIsWait(t *testing.T) {
	a := Advise(fc(fcOpts{price: 10, low: 10, disc: 50, typicalDepth: 50, pLower: 0.05, expLow: 9.9, nextMedian: i(21)}), AdviceInput{})
	if a.Verdict == VerdictWait || a.WaitUntil != nil {
		t.Errorf("verdict %s with wait_until %v", a.Verdict, a.WaitUntil)
	}
}

func TestAdviceTargetPrice(t *testing.T) {
	cheap := fcOpts{price: 14, low: 10, disc: 30, typicalDepth: 50, pLower: 0.8, expLow: 10}

	t.Run("a met target buys even when a lower price is likely", func(t *testing.T) {
		a := Advise(fc(cheap), AdviceInput{TargetPrice: f64(15), Watching: true})
		if a.Verdict != VerdictBuyNow || !has(a, "target_met") || impact(a, "target_met") != 40 {
			t.Fatalf("verdict %s %v", a.Verdict, a.Reasons)
		}
		if !has(a, "better_price_likely") {
			t.Error("the chance of a lower price should still be shown")
		}
		if a.Score < buyThreshold {
			t.Errorf("score %d is below the buy threshold although the verdict is buy_now", a.Score)
		}
		// Without the target the same game is not a clear buy.
		if without := Advise(fc(cheap), AdviceInput{}); without.Verdict == VerdictBuyNow {
			t.Errorf("without a target the verdict should not be buy_now, got %s", without.Verdict)
		}
	})
	t.Run("an unmet target pushes toward waiting, in proportion to the gap", func(t *testing.T) {
		near := Advise(fc(cheap), AdviceInput{TargetPrice: f64(13)})
		far := Advise(fc(cheap), AdviceInput{TargetPrice: f64(7)})
		if impact(near, "above_target") >= 0 || impact(far, "above_target") >= impact(near, "above_target") {
			t.Errorf("above_target impacts: near %d far %d", impact(near, "above_target"), impact(far, "above_target"))
		}
		if impact(far, "above_target") != -15 {
			t.Errorf("a far-off target is capped at -15, got %d", impact(far, "above_target"))
		}
	})
	t.Run("the target applies even with too little history", func(t *testing.T) {
		f := Forecast{Model: ModelInsufficient, CurrentPrice: 9}
		if a := Advise(f, AdviceInput{TargetPrice: f64(10)}); a.Verdict != VerdictBuyNow {
			t.Errorf("verdict = %s, want buy_now: the user's own target is a fact", a.Verdict)
		}
		if a := Advise(f, AdviceInput{TargetPrice: f64(5)}); a.Verdict != VerdictNoData {
			t.Errorf("verdict = %s, want not_enough_data", a.Verdict)
		}
	})
}

func TestAdviceNotEnoughData(t *testing.T) {
	a := Advise(Forecast{Model: ModelInsufficient, CurrentPrice: 20}, AdviceInput{})
	if a.Verdict != VerdictNoData || a.Score != 50 || len(a.Reasons) != 0 {
		t.Errorf("advice = %+v", a)
	}
}

func TestAdvicePatienceShrinksTheLongerSomeoneHasWatched(t *testing.T) {
	for _, tc := range []struct {
		watching bool
		days     int
		want     int
	}{
		{false, 500, 90}, {true, 0, 90}, {true, 60, 90}, {true, 61, 60}, {true, 121, 45}, {true, 241, 30},
	} {
		if got := patience(AdviceInput{Watching: tc.watching, DaysWatched: tc.days}); got != tc.want {
			t.Errorf("watching=%v days=%d: patience %d, want %d", tc.watching, tc.days, got, tc.want)
		}
	}
}

func TestAdviceLongWatchersAreNudgedTowardBuying(t *testing.T) {
	f := fc(fcOpts{price: 20, low: 10, disc: 0, typicalDepth: 50, pLower: 0.6, expLow: 14})
	fresh := Advise(f, AdviceInput{Watching: true, DaysWatched: 3})
	long := Advise(f, AdviceInput{Watching: true, DaysWatched: 150})

	if has(fresh, "waited_long") {
		t.Error("a recent watcher has not waited long")
	}
	if !has(long, "waited_long") || impact(long, "waited_long") != 10 {
		t.Errorf("150 days watched should add 10 points, got %+v", long.Reasons)
	}
	if long.Score <= fresh.Score {
		t.Errorf("scores: fresh %d, long %d; waiting a long time should raise the buy score", fresh.Score, long.Score)
	}
	if !long.Personalized {
		t.Error("a watcher gets personalized advice")
	}
	if long.PatienceDays != 45 {
		t.Errorf("patience = %d, want 45 after 150 days", long.PatienceDays)
	}
	if impact(Advise(f, AdviceInput{Watching: true, DaysWatched: 900}), "waited_long") != 12 {
		t.Error("the long-wait bonus is capped at 12")
	}
}

func TestAdviceComparesWithThePriceWhenWatchingBegan(t *testing.T) {
	f := fc(fcOpts{price: 18, low: 10, disc: 10, typicalDepth: 50, pLower: 0.4, expLow: 15})
	cheaper := Advise(f, AdviceInput{Watching: true, DaysWatched: 10, WatchStartPrice: f64(25)})
	pricier := Advise(f, AdviceInput{Watching: true, DaysWatched: 10, WatchStartPrice: f64(15)})
	same := Advise(f, AdviceInput{Watching: true, DaysWatched: 10, WatchStartPrice: f64(18)})

	if impact(cheaper, "cheaper_than_when_watched") != 10 {
		t.Errorf("cheaper: %+v", cheaper.Reasons)
	}
	if impact(pricier, "pricier_than_when_watched") != -5 {
		t.Errorf("pricier: %+v", pricier.Reasons)
	}
	if has(same, "cheaper_than_when_watched") || has(same, "pricier_than_when_watched") {
		t.Errorf("an unchanged price should say nothing: %v", reasonCodes(same))
	}
}

func TestAdviceDiscountRules(t *testing.T) {
	base := fcOpts{price: 14, low: 10, typicalDepth: 40, pLower: 0.4, expLow: 12}
	for _, tc := range []struct {
		disc int
		want string
		sign int
	}{
		{0, "full_price", -1}, {15, "shallow_discount", -1}, {20, "", 0}, {39, "", 0}, {40, "deep_discount", 1}, {60, "deep_discount", 1},
	} {
		o := base
		o.disc = tc.disc
		a := Advise(fc(o), AdviceInput{})
		codes := map[string]bool{}
		for _, c := range reasonCodes(a) {
			codes[c] = true
		}
		for _, code := range []string{"full_price", "shallow_discount", "deep_discount"} {
			if (code == tc.want) != codes[code] {
				t.Errorf("discount %d%%: reasons %v, want only %q", tc.disc, reasonCodes(a), tc.want)
			}
		}
	}
}

func TestWeakForecastsPushLess(t *testing.T) {
	o := fcOpts{price: 20, low: 10, disc: 0, typicalDepth: 50, pLower: 0.9, expLow: 11}
	strong, weak := o, o
	strong.confidence, weak.confidence = 1, 0.15
	s, w := Advise(fc(strong), AdviceInput{}), Advise(fc(weak), AdviceInput{})

	if impact(w, "full_price") >= 0 || impact(w, "full_price") <= impact(s, "full_price") {
		t.Errorf("full_price impact: strong %d, weak %d; the weak forecast should push less but in the same direction",
			impact(s, "full_price"), impact(w, "full_price"))
	}
	if w.Score <= s.Score {
		t.Errorf("scores strong %d weak %d: with a weak forecast the score stays nearer 50", s.Score, w.Score)
	}
	if w.Confidence >= s.Confidence {
		t.Errorf("advice confidence strong %.2f weak %.2f", s.Confidence, w.Confidence)
	}
}

func TestAdviceScoreAndConfidenceAreBounded(t *testing.T) {
	cases := []fcOpts{
		{price: 5, low: 5, disc: 80, typicalDepth: 50, pLower: 0, expLow: 5},
		{price: 60, low: 10, disc: 0, typicalDepth: 80, pLower: 1, expLow: 20},
		{price: 20, low: 10, disc: 25, typicalDepth: 50, pLower: 0.5, expLow: 15},
	}
	for _, o := range cases {
		for _, in := range []AdviceInput{{}, {Watching: true, DaysWatched: 400, TargetPrice: f64(1), WatchStartPrice: f64(100)}} {
			a := Advise(fc(o), in)
			if a.Score < 0 || a.Score > 100 || a.Confidence < 0 || a.Confidence > 1 {
				t.Errorf("score %d confidence %.2f out of range for %+v", a.Score, a.Confidence, o)
			}
			if a.Verdict != VerdictBuyNow && a.Verdict != VerdictWait && a.Verdict != VerdictTossUp {
				t.Errorf("verdict %q", a.Verdict)
			}
			if a.Reasons == nil {
				t.Error("reasons must be an empty list, not null")
			}
		}
	}
}

func TestAdviceVerdictThresholds(t *testing.T) {
	// A forecast engineered to land on each side of the thresholds.
	mid := fcOpts{price: 14, low: 10, disc: 20, typicalDepth: 40, pLower: 0.35, expLow: 13}
	if a := Advise(fc(mid), AdviceInput{}); a.Verdict != VerdictTossUp {
		t.Errorf("a middling situation should be a toss-up, got %s (score %d, %v)", a.Verdict, a.Score, reasonCodes(a))
	}
}

func TestAdviceReasonTextIsReadable(t *testing.T) {
	a := Advise(fc(fcOpts{price: 10, low: 10, disc: 50, typicalDepth: 50, pLower: 0.05, expLow: 9.9}), AdviceInput{})
	for _, r := range a.Reasons {
		if r.Text == "" || strings.Contains(r.Text, "%!") || strings.Contains(r.Text, "NaN") {
			t.Errorf("bad reason text for %s: %q", r.Code, r.Text)
		}
	}
	if !strings.Contains(a.Reasons[0].Text, "$10.00") {
		t.Errorf("text should quote the price: %q", a.Reasons[0].Text)
	}
}

func TestInterpolation(t *testing.T) {
	f := Forecast{
		CurrentPrice: 20,
		Curve:        []CurvePoint{{DaysAhead: 10, PLowerBy: 0.1}, {DaysAhead: 20, PLowerBy: 0.3}},
		Horizons:     []Horizon{{Days: 30, ExpectedLow: 17}, {Days: 90, ExpectedLow: 11}},
	}
	get := func(c CurvePoint) float64 { return c.PLowerBy }
	for _, tc := range []struct {
		days int
		want float64
	}{{10, 0.1}, {15, 0.2}, {20, 0.3}, {500, 0.3}} {
		if got := interpolate(f, tc.days, get); got < tc.want-1e-9 || got > tc.want+1e-9 {
			t.Errorf("interpolate(%d) = %v, want %v", tc.days, got, tc.want)
		}
	}
	for _, tc := range []struct {
		days int
		want float64
	}{{15, 18.5}, {30, 17}, {60, 14}, {90, 11}, {900, 11}} {
		if got := interpolateHorizon(f, tc.days); got < tc.want-1e-9 || got > tc.want+1e-9 {
			t.Errorf("interpolateHorizon(%d) = %v, want %v", tc.days, got, tc.want)
		}
	}
	if interpolate(Forecast{}, 30, get) != 0 || interpolateHorizon(Forecast{}, 30) != 0 {
		t.Error("an empty forecast interpolates to zero")
	}
}
