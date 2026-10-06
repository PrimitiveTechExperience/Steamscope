package prediction

import (
	"math"
	"reflect"
	"testing"
	"time"
)

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// cyclic builds a daily history of `total` days starting at epoch. A sale of
// the given depth lasting saleLen days begins on day firstSale and every
// `every` days after. regular is the full price. It returns the points and the
// date of the last day, which is the forecast's "today".
func cyclic(total, firstSale, every, saleLen int, depth, regular float64) ([]Point, time.Time) {
	var pts []Point
	for i := 0; i < total; i++ {
		price := regular
		if every > 0 && i >= firstSale && (i-firstSale)%every < saleLen {
			price = regular * (1 - depth)
		}
		pts = append(pts, Point{Date: epoch.AddDate(0, 0, i), Price: price, Regular: regular})
	}
	return pts, epoch.AddDate(0, 0, total-1)
}

func predict(t *testing.T, pts []Point, today time.Time) Forecast {
	t.Helper()
	return Predict(1, pts, "v", Options{Now: today})
}

func TestPeriodicSalesArePredictedOnSchedule(t *testing.T) {
	// Sales start on day 30, 90, ..., 690 and last 10 days. Day 729 is 30 days
	// after the last one ended, so the next starts on day 750: 21 days ahead.
	pts, today := cyclic(730, 30, 60, 10, 0.5, 20)
	f := predict(t, pts, today)

	if f.Model != ModelWeibull {
		t.Fatalf("model = %s, want %s", f.Model, ModelWeibull)
	}
	if f.OnSale || f.CurrentPrice != 20 {
		t.Errorf("OnSale=%v price=%v, want a full-price game", f.OnSale, f.CurrentPrice)
	}
	if f.Typical.Count != 12 || f.Typical.MedianDepthPercent != 50 || f.Typical.MedianPrice != 10 ||
		f.Typical.MedianDurationDays != 10 || f.Typical.MedianIntervalDays == nil || *f.Typical.MedianIntervalDays != 60 {
		t.Errorf("typical sale = %+v", f.Typical)
	}
	if f.Next.MedianDays == nil || *f.Next.MedianDays < 12 || *f.Next.MedianDays > 32 {
		t.Errorf("median days to next sale = %v, want about 21", deref(f.Next.MedianDays))
	}
	if f.Next.P25Days == nil || f.Next.P75Days == nil || *f.Next.P25Days > *f.Next.MedianDays || *f.Next.MedianDays > *f.Next.P75Days {
		t.Errorf("quartiles out of order: %+v", f.Next)
	}
	byDay := curveByDay(f)
	if p := byDay[7].PLowerBy; p > 0.15 {
		t.Errorf("chance of a lower price within a week = %.2f, want small (sale due in ~21 days)", p)
	}
	if p := byDay[63].PLowerBy; p < 0.9 {
		t.Errorf("chance of a lower price within 63 days = %.2f, want near 1 for a 60-day cycle", p)
	}
	if f.Score < 85 {
		t.Errorf("score = %d, want high: a sale is due within 180 days", f.Score)
	}
}

func TestRightAfterASaleAnotherIsUnlikelySoon(t *testing.T) {
	// Day 700: the sale that started on day 690 ended yesterday; the next is day 750.
	pts, today := cyclic(701, 30, 60, 10, 0.5, 20)
	f := predict(t, pts, today)
	byDay := curveByDay(f)

	if p := byDay[14].PLowerBy; p > 0.15 {
		t.Errorf("chance of a sale within 14 days of the last one ending = %.2f, want under 0.15", p)
	}
	if p := byDay[91].PLowerBy; p < 0.9 {
		t.Errorf("chance of a sale within 91 days = %.2f, want near 1", p)
	}
}

func TestDuringASaleAtItsUsualDepthNothingCheaperIsExpected(t *testing.T) {
	// Day 695 is mid-sale (690-699) at the usual 50% off. Future sales are no
	// deeper, so a price 5% lower than today's should not appear.
	pts, today := cyclic(696, 30, 60, 10, 0.5, 20)
	f := predict(t, pts, today)

	if !f.OnSale || f.CurrentPrice != 10 || f.CurrentDisc != 50 {
		t.Fatalf("on_sale=%v price=%v disc=%d", f.OnSale, f.CurrentPrice, f.CurrentDisc)
	}
	for _, c := range f.Curve {
		if c.PLowerBy > 0.001 {
			t.Fatalf("p_lower_by(%d) = %.3f, want ~0 (never deeper than the usual 50%%)", c.DaysAhead, c.PLowerBy)
		}
	}
	if f.Score != 0 {
		t.Errorf("score = %d, want 0", f.Score)
	}
	// The sale ends, so the expected price a month out is back near full price.
	if p := curveByDay(f)[35].ExpectedPrice; p < 12 {
		t.Errorf("expected price after the sale ends = %.2f, want it to rise toward $20", p)
	}
}

func TestAGameNeverOnSale(t *testing.T) {
	pts, today := cyclic(400, 0, 0, 0, 0, 20)
	f := predict(t, pts, today)

	if f.Model != ModelNoSales {
		t.Fatalf("model = %s, want %s", f.Model, ModelNoSales)
	}
	if f.Confidence > 0.2 {
		t.Errorf("confidence = %.2f, want low for a game with no sales", f.Confidence)
	}
	if f.Score > 35 {
		t.Errorf("score = %d, want low", f.Score)
	}
	if p := curveByDay(f)[28].ExpectedPrice; p < 19 {
		t.Errorf("expected price in a month = %.2f, want about $20", p)
	}
}

func TestTooLittleHistoryGivesNoForecast(t *testing.T) {
	t.Run("under 60 days", func(t *testing.T) {
		pts, today := cyclic(40, 10, 20, 5, 0.5, 20)
		f := predict(t, pts, today)
		if f.Model != ModelInsufficient || len(f.Curve) != 0 || len(f.Horizons) != 0 {
			t.Errorf("model=%s curve=%d horizons=%d", f.Model, len(f.Curve), len(f.Horizons))
		}
		if f.CurrentPrice != 20 {
			t.Errorf("current price = %v, want it reported even without a forecast", f.CurrentPrice)
		}
	})
	t.Run("long span but few recorded days", func(t *testing.T) {
		var pts []Point
		for i := 0; i < 10; i++ {
			pts = append(pts, Point{Date: epoch.AddDate(0, 0, i*9), Price: 20, Regular: 20})
		}
		if f := predict(t, pts, epoch.AddDate(0, 0, 100)); f.Model != ModelInsufficient {
			t.Errorf("model = %s, want insufficient", f.Model)
		}
	})
	t.Run("no points", func(t *testing.T) {
		if f := Predict(1, nil, "v", Options{Now: epoch}); f.Model != ModelInsufficient || f.CurrentPrice != 0 {
			t.Errorf("forecast = %+v", f)
		}
	})
}

func TestForecastIsDeterministic(t *testing.T) {
	pts, today := cyclic(730, 30, 60, 10, 0.5, 20)
	a := predict(t, pts, today)
	b := predict(t, pts, today)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("the same history produced two different forecasts")
	}

	one := Predict(1, pts, "v", Options{Now: today, Seed: 7})
	two := Predict(1, pts, "v", Options{Now: today, Seed: 7})
	other := Predict(1, pts, "v", Options{Now: today, Seed: 8})
	if !reflect.DeepEqual(one, two) {
		t.Error("an explicit seed is not repeatable")
	}
	if reflect.DeepEqual(one.Curve, other.Curve) {
		t.Error("different seeds should simulate different futures")
	}
}

func TestForecastShapeAndBounds(t *testing.T) {
	pts, today := cyclic(730, 30, 60, 10, 0.5, 20)
	f := predict(t, pts, today)

	if f.GeneratedAt != today || f.DataVersion != "v" || f.AppID != 1 {
		t.Errorf("metadata: %+v", f)
	}
	if first, last := f.Curve[0], f.Curve[len(f.Curve)-1]; first.DaysAhead != 1 || last.DaysAhead != HorizonDays {
		t.Errorf("curve spans days %d..%d, want 1..%d", first.DaysAhead, last.DaysAhead, HorizonDays)
	}
	prevDays, prevLower := 0, 0.0
	for _, c := range f.Curve {
		if c.DaysAhead <= prevDays {
			t.Fatalf("days not increasing at %d", c.DaysAhead)
		}
		if want := today.AddDate(0, 0, c.DaysAhead).Format("2006-01-02"); c.Date != want {
			t.Fatalf("day %d has date %s, want %s", c.DaysAhead, c.Date, want)
		}
		for name, v := range map[string]float64{"p_on_sale": c.POnSale, "p_lower_by": c.PLowerBy} {
			if v < 0 || v > 1 || math.IsNaN(v) {
				t.Fatalf("%s at day %d = %v", name, c.DaysAhead, v)
			}
		}
		if c.PLowerBy+1e-9 < prevLower {
			t.Fatalf("p_lower_by fell from %.4f to %.4f at day %d: it is cumulative", prevLower, c.PLowerBy, c.DaysAhead)
		}
		if c.ExpectedPrice < 10-0.01 || c.ExpectedPrice > 20+0.01 {
			t.Fatalf("expected price %.2f at day %d is outside the $10-$20 range of this game", c.ExpectedPrice, c.DaysAhead)
		}
		prevDays, prevLower = c.DaysAhead, c.PLowerBy
	}
	var days []int
	for _, h := range f.Horizons {
		days = append(days, h.Days)
		if h.ExpectedLow > f.CurrentPrice+1e-9 {
			t.Errorf("expected low %.2f exceeds today's price %.2f", h.ExpectedLow, f.CurrentPrice)
		}
	}
	if !reflect.DeepEqual(days, []int{30, 90, 180, 365, 730}) {
		t.Errorf("horizons = %v", days)
	}
	if f.HistoricLow != 10 || f.HistoryDays != 730 {
		t.Errorf("historic low %v over %d days, want $10 over 730", f.HistoricLow, f.HistoryDays)
	}
}

func TestSeasonalityLearnsTheCalendar(t *testing.T) {
	// A sale every year around 1 July and 1 December, for three years.
	var pts []Point
	for d := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC); d.Before(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)); d = d.AddDate(0, 0, 1) {
		price := 20.0
		if (d.Month() == time.July && d.Day() <= 10) || (d.Month() == time.December && d.Day() >= 1 && d.Day() <= 10) {
			price = 10
		}
		pts = append(pts, Point{Date: d, Price: price, Regular: 20})
	}
	days := buildDays(pts, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1))
	eps := findEpisodes(days)
	if len(eps) != 6 {
		t.Fatalf("found %d sales, want 6", len(eps))
	}
	m := seasonality(days, eps, float64(len(days))/365)

	doy := func(month time.Month, day int) int {
		return time.Date(2023, month, day, 0, 0, 0, 0, time.UTC).YearDay() - 1
	}
	if m[doy(time.July, 3)] < 1.4 {
		t.Errorf("multiplier on 3 July = %.2f, want well above 1", m[doy(time.July, 3)])
	}
	if m[doy(time.April, 15)] > 0.8 {
		t.Errorf("multiplier on 15 April = %.2f, want below 1 (no sales around then)", m[doy(time.April, 15)])
	}
	if m[doy(time.July, 3)] <= m[doy(time.April, 15)] {
		t.Error("a sale day should be more likely than a quiet one")
	}
}

func TestSeasonalityFallsBackToTheSteamCalendar(t *testing.T) {
	m := seasonality(nil, nil, 0.5) // no sales seen
	doy := func(month time.Month, day int) int {
		return time.Date(2023, month, day, 0, 0, 0, 0, time.UTC).YearDay() - 1
	}
	if m[doy(time.July, 1)] <= 1.2 {
		t.Errorf("summer sale multiplier = %.2f, want above 1", m[doy(time.July, 1)])
	}
	if m[doy(time.October, 10)] >= 1 {
		t.Errorf("quiet October multiplier = %.2f, want below 1", m[doy(time.October, 10)])
	}
}

func TestEpisodeDetection(t *testing.T) {
	prices := []float64{20, 20, 10, 10, 20, 10, 10, 20, 20, 20, 20, 19.5, 15, 20}
	var pts []Point
	for i, p := range prices {
		pts = append(pts, Point{Date: epoch.AddDate(0, 0, i), Price: p, Regular: 20})
	}
	eps := findEpisodes(buildDays(pts, epoch.AddDate(0, 0, len(prices)-1)))

	// Days 2-3 and 5-6 are one sale (a one-day blip between them is merged);
	// 19.50 is only 2.5% off, which is not a sale; day 12 is a separate 25% sale.
	if len(eps) != 2 {
		t.Fatalf("found %d sales, want 2: %+v", len(eps), eps)
	}
	if eps[0].start != 2 || eps[0].end != 6 || eps[0].depth != 0.5 {
		t.Errorf("first sale = %+v, want days 2-6 at 50%%", eps[0])
	}
	if eps[1].start != 12 || eps[1].end != 12 || math.Abs(eps[1].depth-0.25) > 1e-9 {
		t.Errorf("second sale = %+v, want day 12 at 25%%", eps[1])
	}
}

func TestRegularPriceFallsBackToTheTrailingYearHigh(t *testing.T) {
	// No regular prices recorded at all: the sale must still be found.
	var pts []Point
	for i := 0; i < 100; i++ {
		price := 40.0
		if i >= 60 && i < 70 {
			price = 20
		}
		pts = append(pts, Point{Date: epoch.AddDate(0, 0, i), Price: price})
	}
	days := buildDays(pts, epoch.AddDate(0, 0, 99))
	eps := findEpisodes(days)
	if len(eps) != 1 || eps[0].depth != 0.5 {
		t.Fatalf("sales = %+v, want one 50%% sale", eps)
	}
	if days[65].regular != 40 {
		t.Errorf("reference price during the sale = %v, want 40", days[65].regular)
	}
}

func TestBuildDays(t *testing.T) {
	today := epoch.AddDate(0, 0, 9)
	pts := []Point{
		{Date: epoch, Price: 10, Regular: 10},
		{Date: epoch.AddDate(0, 0, 3), Price: 8, Regular: 10},
		{Date: epoch.AddDate(0, 0, 3).Add(5 * time.Hour), Price: 7, Regular: 10}, // same day, later wins
		{Date: epoch.AddDate(0, 0, 50), Price: 1, Regular: 10},                   // in the future: ignored
		{Date: epoch.AddDate(0, 0, 4), Price: -5, Regular: 10},                   // invalid: ignored
	}
	days := buildDays(pts, today)

	if len(days) != 10 {
		t.Fatalf("got %d days, want one per day through today (10)", len(days))
	}
	want := []float64{10, 10, 10, 7, 7, 7, 7, 7, 7, 7}
	for i, d := range days {
		if d.price != want[i] {
			t.Errorf("day %d price = %v, want %v (gaps carry the last price forward)", i, d.price, want[i])
		}
	}
	if !days[0].real || days[1].real || !days[3].real {
		t.Error("real flags should mark only recorded days")
	}
}

func TestRenewalFit(t *testing.T) {
	mk := func(gaps ...int) []episode {
		eps := []episode{{start: 0, end: 5, depth: 0.3}}
		for _, g := range gaps {
			s := eps[len(eps)-1].start + g
			eps = append(eps, episode{start: s, end: s + 5, depth: 0.3})
		}
		return eps
	}
	if r := fitRenewal(mk(), 400, 5); r.model != ModelPoisson {
		t.Errorf("one sale: model %s, want poisson", r.model)
	}
	if r := fitRenewal(nil, 400, 0); r.model != ModelNoSales {
		t.Errorf("no sales: model %s", r.model)
	}
	regular := fitRenewal(mk(60, 60, 60, 60), 400, 245)
	if regular.model != ModelWeibull || regular.shape < 3.9 {
		t.Errorf("regular spacing: %+v, want a high Weibull shape", regular)
	}
	if math.Abs(regular.scale-60/math.Gamma(1+1/regular.shape)) > 1e-9 {
		t.Errorf("scale = %v", regular.scale)
	}
	erratic := fitRenewal(mk(10, 100, 20, 150, 15, 200), 600, 500)
	if erratic.shape > 1.4 {
		t.Errorf("erratic spacing: shape %.2f, want near 1 (memoryless)", erratic.shape)
	}
}

func TestHazardRisesForRegularSalesAndIsCapped(t *testing.T) {
	h := renewal{ModelWeibull, 4, 66}.hazardTable(300)
	for a := 1; a < len(h); a++ {
		if h[a]+1e-12 < h[a-1] {
			t.Fatalf("hazard fell from %.5f to %.5f at age %d for a regular process", h[a-1], h[a], a)
		}
		if h[a] > maxDailyHazard+1e-12 {
			t.Fatalf("hazard %.3f exceeds the cap at age %d", h[a], a)
		}
	}
	if h[20] >= h[100] {
		t.Error("a sale should be likelier 100 days after the last than 20 days after")
	}
	flat := renewal{ModelPoisson, 1, 90}.hazardTable(100)
	if math.Abs(flat[5]-flat[80]) > 1e-12 {
		t.Error("an exponential process has a constant hazard")
	}
}

func TestNextSaleQuantiles(t *testing.T) {
	none := simulation{paths: 4, firstStart: []int{-1, -1, -1, -1}}
	if n := none.nextSale(); n.P25Days != nil || n.MedianDays != nil || n.P75Days != nil {
		t.Errorf("no sale in any future: %+v", n)
	}
	some := simulation{paths: 4, firstStart: []int{40, 10, 20, -1}}
	n := some.nextSale()
	if deref(n.P25Days) != 10 || deref(n.MedianDays) != 20 || deref(n.P75Days) != 40 {
		t.Errorf("quartiles = %d/%d/%d, want 10/20/40", deref(n.P25Days), deref(n.MedianDays), deref(n.P75Days))
	}
	half := simulation{paths: 4, firstStart: []int{5, 9, -1, -1}}
	if n := half.nextSale(); n.P75Days != nil || deref(n.MedianDays) != 9 {
		t.Errorf("only half the futures see a sale: %+v", n)
	}
}

func TestConfidenceGrowsWithEvidence(t *testing.T) {
	if c := confidence(ModelNoSales, 0, 500); c > 0.2 {
		t.Errorf("no sales: %.2f", c)
	}
	few, many := confidence(ModelPoisson, 1, 120), confidence(ModelWeibull, 12, 730)
	if few >= many || many > 1 {
		t.Errorf("confidence few=%.2f many=%.2f", few, many)
	}
	if many != 1 {
		t.Errorf("a dozen sales over two years should be full confidence, got %.2f", many)
	}
}

func curveByDay(f Forecast) map[int]CurvePoint {
	m := map[int]CurvePoint{}
	for _, c := range f.Curve {
		m[c.DaysAhead] = c
	}
	return m
}

func deref(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

// regimes builds `total` days with a 10-day sale every 60 days from day 30,
// whose depth is chosen per sale by the day it starts. It returns the points
// and a "today" on the given day.
func regimes(total, todayIdx int, depthOf func(startDay int) float64) ([]Point, time.Time) {
	var pts []Point
	for i := 0; i < total; i++ {
		price := 20.0
		if i >= 30 && (i-30)%60 < 10 {
			price = 20 * (1 - depthOf(i-(i-30)%60))
		}
		pts = append(pts, Point{Date: epoch.AddDate(0, 0, i), Price: price, Regular: 20})
	}
	return pts[:todayIdx+1], epoch.AddDate(0, 0, todayIdx)
}

func TestRecentSalesCountForMoreThanOldOnes(t *testing.T) {
	// Both games are on a 30%-off sale today. In A the only deeper (70%) sales
	// were years ago (7 of 25); in B they were all within the last year (5 of
	// 25). Counted equally, A has the higher share of deep sales and would be
	// rated likelier to see another; weighted by recency, B should be.
	const todayIdx = 1415 // inside the sale that began on day 1410
	a, today := regimes(1460, todayIdx, func(d int) float64 {
		if d < 400 {
			return 0.7
		}
		return 0.3
	})
	b, _ := regimes(1460, todayIdx, func(d int) float64 {
		if d >= 1100 && d < 1410 {
			return 0.7
		}
		return 0.3
	})

	fa, fb := predict(t, a, today), predict(t, b, today)
	if !fa.OnSale || !fb.OnSale || fa.CurrentDisc != 30 || fb.CurrentDisc != 30 {
		t.Fatalf("setup: on sale %v/%v, discounts %d/%d", fa.OnSale, fb.OnSale, fa.CurrentDisc, fb.CurrentDisc)
	}
	pa, pb := curveByDay(fa)[91].PLowerBy, curveByDay(fb)[91].PLowerBy
	if pa <= 0 {
		t.Errorf("old deep sales still count a little: chance of a deeper sale = %.3f, want above 0", pa)
	}
	if pb < pa+0.1 {
		t.Errorf("chance of a deeper sale within 91 days: old deep sales %.3f, recent deep sales %.3f; recent should be clearly higher", pa, pb)
	}
}

func TestFreePromotionsAreNotSalesOrRecordLows(t *testing.T) {
	// A $10 game with a free weekend (price $0) every 30 days, and no real sales.
	var pts []Point
	for i := 0; i < 200; i++ {
		price := 10.0
		if i%30 < 3 {
			price = 0
		}
		pts = append(pts, Point{Date: epoch.AddDate(0, 0, i), Price: price, Regular: 10})
	}
	days := buildDays(pts, epoch.AddDate(0, 0, 199))
	if eps := findEpisodes(days); len(eps) != 0 {
		t.Errorf("found %d sales in a game that only has free weekends: %+v", len(eps), eps)
	}
	f := predict(t, pts, epoch.AddDate(0, 0, 199))
	if f.HistoricLow != 10 {
		t.Errorf("historic low = %v, want $10: a free weekend is not a price anyone paid", f.HistoricLow)
	}
	if f.Model != ModelNoSales {
		t.Errorf("model = %s, want %s", f.Model, ModelNoSales)
	}
	// A game that is free all the time is unaffected.
	free := []Point{{Date: epoch, Price: 0, Regular: 0}, {Date: epoch.AddDate(0, 0, 1), Price: 0, Regular: 0}}
	if d := buildDays(free, epoch.AddDate(0, 0, 1)); d[0].price != 0 {
		t.Errorf("a free-to-play game's price = %v, want 0", d[0].price)
	}
}

func TestAllTimeLowFromALongerRecordIsUsed(t *testing.T) {
	pts, today := cyclic(730, 30, 60, 10, 0.5, 20) // lowest price in this window: $10
	f := Predict(1, pts, "v", Options{Now: today, AllTimeLow: 4})
	if f.HistoricLow != 4 {
		t.Errorf("historic low = %v, want $4 from the longer record", f.HistoricLow)
	}
	// A "low" that is higher than what the data shows must not raise it.
	if f := Predict(1, pts, "v", Options{Now: today, AllTimeLow: 15}); f.HistoricLow != 10 {
		t.Errorf("historic low = %v, want it to stay at $10", f.HistoricLow)
	}
	if f := Predict(1, pts, "v", Options{Now: today}); f.HistoricLow != 10 {
		t.Errorf("without a known low: %v, want $10", f.HistoricLow)
	}
}

func TestRecencyWeights(t *testing.T) {
	if recency(0) != 1 || recency(-5) != 1 {
		t.Error("something that just happened has full weight")
	}
	if got := recency(730); got < 0.4999 || got > 0.5001 {
		t.Errorf("recency(730) = %v, want 0.5", got)
	}
	if recency(100) <= recency(1000) {
		t.Error("newer should weigh more than older")
	}
	mean, _ := weightedMeanStd([]float64{10, 100}, []float64{3, 1})
	if mean != 32.5 {
		t.Errorf("weighted mean = %v, want 32.5", mean)
	}
}

func TestAFreeGameHasNothingCheaperThanFree(t *testing.T) {
	var pts []Point
	for i := 0; i < 300; i++ {
		pts = append(pts, Point{Date: epoch.AddDate(0, 0, i), Price: 0, Regular: 0})
	}
	f := predict(t, pts, epoch.AddDate(0, 0, 299))
	if f.Model != ModelFree || f.CurrentPrice != 0 || f.Score != 0 {
		t.Errorf("model=%s price=%v score=%d", f.Model, f.CurrentPrice, f.Score)
	}
	if len(f.Curve) != 0 {
		t.Errorf("a free game gets no price curve, got %d points", len(f.Curve))
	}
	// Even a brand-new free game is recognised, whatever the history length.
	if f := predict(t, pts[:5], epoch.AddDate(0, 0, 4)); f.Model != ModelFree {
		t.Errorf("a new free game: model %s", f.Model)
	}
	// A game that went free-to-play after years of being paid is free now.
	paid := []Point{{Date: epoch, Price: 20, Regular: 20}, {Date: epoch.AddDate(0, 0, 100), Price: 0, Regular: 0}}
	if f := predict(t, paid, epoch.AddDate(0, 0, 200)); f.Model != ModelFree {
		t.Errorf("went free-to-play: model %s", f.Model)
	}
}

func TestALongSilenceIsNotReadAsASaleBeingDue(t *testing.T) {
	// Sales every 30 days for a year, then nothing for 600 days. A model that
	// treats the gap as "overdue" would put a sale within days; the game has
	// more likely stopped discounting.
	var pts []Point
	for i := 0; i < 1000; i++ {
		price := 20.0
		if i < 400 && i%30 < 5 {
			price = 10
		}
		pts = append(pts, Point{Date: epoch.AddDate(0, 0, i), Price: price, Regular: 20})
	}
	f := predict(t, pts, epoch.AddDate(0, 0, 999))

	p90 := curveByDay(f)[91].PLowerBy
	if p90 > 0.6 {
		t.Errorf("chance of a sale within 91 days after 600 silent days = %.2f, want well below certainty", p90)
	}
	if p90 < 0.05 {
		t.Errorf("chance of a sale within 91 days = %.2f, want it to stay possible", p90)
	}
	if f.Next.MedianDays != nil && *f.Next.MedianDays < 20 {
		t.Errorf("median days to the next sale = %d, want it far off after a long silence", *f.Next.MedianDays)
	}
}

func TestSilenceAdjustment(t *testing.T) {
	regular := renewal{ModelWeibull, 4, 66} // mean gap about 60 days
	if got := regular.adjustedForSilence(100); got != regular {
		t.Errorf("a normal wait must not change the fit: %+v", got)
	}
	got := regular.adjustedForSilence(600)
	if got.shape != 1 || got.scale != 300 || got.model != ModelWeibull {
		t.Errorf("after 600 silent days: %+v, want a memoryless fit with scale 300", got)
	}
	if r := (renewal{ModelPoisson, 1, 90}).adjustedForSilence(100); r.scale != 90 {
		t.Errorf("a short wait for a constant-rate game: %+v", r)
	}
}

func TestFreeRunsAreResolvedByLength(t *testing.T) {
	build := func(prices ...float64) []day {
		var d []day
		for i, p := range prices {
			d = append(d, day{date: epoch.AddDate(0, 0, i), price: p, regular: 10})
		}
		resolveFreeRuns(d)
		return d
	}
	got := func(d []day) []float64 {
		var out []float64
		for _, x := range d {
			out = append(out, x.price)
		}
		return out
	}

	if g := got(build(10, 0, 0, 10, 5)); !reflect.DeepEqual(g, []float64{10, 10, 10, 10, 5}) {
		t.Errorf("a free weekend in the middle: %v", g)
	}
	if g := got(build(0, 0, 8, 8)); !reflect.DeepEqual(g, []float64{8, 8, 8, 8}) {
		t.Errorf("a free run at the very start takes the next paid price: %v", g)
	}
	if g := got(build(10, 10, 0, 0)); !reflect.DeepEqual(g, []float64{10, 10, 10, 10}) {
		t.Errorf("a short free run ending today is a promotion that has not ended yet: %v", g)
	}
	long := append([]float64{10}, make([]float64, freeToPlayDays)...)
	if g := got(build(long...)); g[len(g)-1] != 0 {
		t.Errorf("free for %d days up to today means free to play: %v", freeToPlayDays, g[len(g)-3:])
	}
	if g := got(build(0, 0, 0)); !reflect.DeepEqual(g, []float64{0, 0, 0}) {
		t.Errorf("a game that never cost anything stays free: %v", g)
	}
}

func TestIsRecordLow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		price    float64
		discount int
		low      float64
		want     bool
	}{
		{"on sale at the lowest price", 4.99, 50, 4.99, true},
		{"within half a cent of it", 4.994, 50, 4.99, true},
		{"cheaper than any recorded price", 3.99, 60, 4.99, true},
		{"on sale but not the lowest", 7.49, 25, 4.99, false},
		{"full price that equals the low (never discounted)", 9.99, 0, 9.99, false},
		{"free", 0, 0, 0, false},
		{"unknown low", 4.99, 50, 0, false},
	} {
		if got := isRecordLow(tc.price, tc.discount, tc.low); got != tc.want {
			t.Errorf("%s: isRecordLow(%v, %d%%, %v) = %v, want %v", tc.name, tc.price, tc.discount, tc.low, got, tc.want)
		}
	}
}

func TestForecastFlagsARecordLow(t *testing.T) {
	// Sales every 60 days to $10; day 695 is mid-sale at the lowest price the game has had.
	pts, today := cyclic(696, 30, 60, 10, 0.5, 20)
	f := predict(t, pts, today)
	if !f.AtRecordLow || f.CurrentPrice != 10 || f.HistoricLow != 10 {
		t.Errorf("mid-sale at the usual low: at_record_low=%v price=%v low=%v", f.AtRecordLow, f.CurrentPrice, f.HistoricLow)
	}

	// The same game at full price is not at a record low.
	full, today := cyclic(730, 30, 60, 10, 0.5, 20)
	if f := predict(t, full, today); f.AtRecordLow {
		t.Error("a full-price day is not a record low")
	}

	// A sale that is shallower than an earlier one is not a record low.
	shallow, today := regimes(1415+1, 1415, func(d int) float64 {
		if d < 700 {
			return 0.7
		}
		return 0.3
	})
	if f := predict(t, shallow, today); f.AtRecordLow || !f.OnSale {
		t.Errorf("on sale at 30%% after 70%% sales: at_record_low=%v on_sale=%v", f.AtRecordLow, f.OnSale)
	}

	// A game whose price never moves has no record low to speak of.
	flat, today := cyclic(300, 0, 0, 0, 0, 20)
	if f := predict(t, flat, today); f.AtRecordLow {
		t.Error("a game that never changes price is not at a record low")
	}
	// A known all-time low from a longer record can rule it out.
	if f := Predict(1, pts, "v", Options{Now: epoch.AddDate(0, 0, 695), AllTimeLow: 4}); f.AtRecordLow {
		t.Error("$10 is not a record low when the game once cost $4")
	}
}

func TestRecordLowIsFlaggedEvenWithLittleHistory(t *testing.T) {
	// 40 days: too short to forecast, but the price facts are still known.
	var pts []Point
	for i := 0; i < 40; i++ {
		price := 20.0
		if i >= 30 {
			price = 8
		}
		pts = append(pts, Point{Date: epoch.AddDate(0, 0, i), Price: price, Regular: 20})
	}
	f := predict(t, pts, epoch.AddDate(0, 0, 39))
	if f.Model != ModelInsufficient || !f.AtRecordLow {
		t.Errorf("model=%s at_record_low=%v, want an insufficient forecast that still flags the record low", f.Model, f.AtRecordLow)
	}
}
