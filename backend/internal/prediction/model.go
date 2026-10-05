// Package prediction forecasts a game's future price from its price history.
//
// The model is statistical, not machine learning. It reads the history as a
// series of sale episodes (runs of days priced below the regular price) and
// fits how often they start, how deep they go and how long they last. It then
// runs a seeded Monte Carlo simulation of the next two years from today's
// state and reads every output (probabilities, expected prices, the likely date
// of the next sale) off the simulated paths. Because the generator is seeded
// from the data, the same history always yields the same forecast.
//
// See docs/PREDICTIONS.md for the assumptions and limits.
package prediction

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"sort"
	"time"
)

const (
	// MinHistoryDays is the history needed before any forecast is made.
	MinHistoryDays = 60
	// MinRealPoints is the number of recorded days needed within that history.
	MinRealPoints = 30
	// HorizonDays is how far ahead the forecast looks.
	HorizonDays = 730
	// DropThreshold is how far below today's price counts as "a lower price".
	DropThreshold = 0.05
	// SaleMinDiscount is the discount from the regular price that counts as a sale.
	SaleMinDiscount = 0.05
	// DefaultPaths is the number of simulated futures.
	DefaultPaths = 1500

	maxDailyHazard = 0.35
	defaultDepth   = 0.30
	defaultLength  = 7
)

// Model names reported in Forecast.Model.
const (
	ModelWeibull      = "weibull_renewal"
	ModelPoisson      = "poisson"
	ModelNoSales      = "no_sales_seen"
	ModelInsufficient = "insufficient"
)

// Point is one recorded day. Regular is the undiscounted price; 0 means unknown.
type Point struct {
	Date    time.Time
	Price   float64
	Regular float64
}

// Options tune a forecast. The zero value is fine.
type Options struct {
	Now   time.Time // the forecast's "today"; defaults to the current UTC day
	Paths int       // simulated futures; defaults to DefaultPaths
	Seed  uint64    // 0 derives the seed from the data
}

// CurvePoint is the forecast for one future date.
type CurvePoint struct {
	Date          string  `json:"date"`
	DaysAhead     int     `json:"days_ahead"`
	ExpectedPrice float64 `json:"expected_price"`
	// POnSale is the chance a sale is running on this date.
	POnSale float64 `json:"p_on_sale"`
	// PLowerBy is the chance that, at some point from now until this date, the
	// price is at least DropThreshold below today's. It never decreases.
	PLowerBy float64 `json:"p_lower_by"`
}

// Horizon summarises one look-ahead window.
type Horizon struct {
	Days   int     `json:"days"`
	PLower float64 `json:"p_lower"`
	// ExpectedPrice is the average price on the last day of the window.
	ExpectedPrice float64 `json:"expected_price"`
	// ExpectedLow is the average lowest price reached within the window.
	ExpectedLow float64 `json:"expected_low"`
}

// TypicalSale describes the game's usual sale.
type TypicalSale struct {
	Count              int     `json:"count"`
	MedianDepthPercent float64 `json:"median_depth_percent"`
	MedianPrice        float64 `json:"median_price"`
	MedianDurationDays int     `json:"median_duration_days"`
	MedianIntervalDays *int    `json:"median_interval_days"`
}

// NextSale is when the next sale is likely to start, in days from today.
// A field is nil when fewer than that share of simulated futures see a sale
// within the horizon.
type NextSale struct {
	P25Days    *int `json:"p25_days"`
	MedianDays *int `json:"median_days"`
	P75Days    *int `json:"p75_days"`
}

// Forecast is the result for one game.
type Forecast struct {
	AppID        int         `json:"app_id"`
	GeneratedAt  time.Time   `json:"generated_at"`
	DataVersion  string      `json:"data_version"`
	Model        string      `json:"model"`
	CurrentPrice float64     `json:"current_price"`
	RegularPrice float64     `json:"regular_price"`
	CurrentDisc  int         `json:"current_discount_percent"`
	OnSale       bool        `json:"on_sale"`
	HistoryDays  int         `json:"history_days"`
	HistoricLow  float64     `json:"historic_low"`
	UsedExtended bool        `json:"used_extended_history"`
	Typical      TypicalSale `json:"typical_sale"`
	Next         NextSale    `json:"next_sale"`
	// Score is the chance (0-100) of a lower price within 180 days.
	Score      int          `json:"score"`
	Confidence float64      `json:"confidence"`
	Curve      []CurvePoint `json:"curve"`
	Horizons   []Horizon    `json:"horizons"`
	Cached     bool         `json:"cached"`
}

var horizonDays = []int{30, 90, 180, 365, 730}

type day struct {
	date    time.Time
	price   float64
	regular float64 // resolved reference price, always >= price
	real    bool
}

type episode struct {
	start, end int
	depth      float64
}

func (e episode) length() int { return e.end - e.start + 1 }

// Predict forecasts the price of one game. dataVersion is stored verbatim on
// the result so a cache can tell when the history it was built from changed.
func Predict(appID int, points []Point, dataVersion string, opt Options) Forecast {
	today := truncate(opt.Now)
	if opt.Now.IsZero() {
		today = truncate(time.Now())
	}
	paths := opt.Paths
	if paths <= 0 {
		paths = DefaultPaths
	}

	f := Forecast{
		AppID: appID, GeneratedAt: today, DataVersion: dataVersion, Model: ModelInsufficient,
		Curve: []CurvePoint{}, Horizons: []Horizon{},
	}

	days := buildDays(points, today)
	if len(days) == 0 {
		return f
	}
	last := days[len(days)-1]
	f.CurrentPrice, f.RegularPrice = round2(last.price), round2(last.regular)
	f.HistoryDays = len(days)
	f.HistoricLow = round2(minPrice(days))
	f.CurrentDisc = int(math.Round(100 * discount(last)))

	real := 0
	for _, d := range days {
		if d.real {
			real++
		}
	}
	if len(days) < MinHistoryDays || real < MinRealPoints {
		return f
	}

	episodes := findEpisodes(days)
	f.OnSale = len(episodes) > 0 && episodes[len(episodes)-1].end == len(days)-1
	f.Typical = typicalSale(episodes, last.regular)
	fit := fitRenewal(episodes, len(days))
	f.Model = fit.model

	years := float64(len(days)) / 365
	season := seasonality(days, episodes, years)

	seed := opt.Seed
	if seed == 0 {
		seed = deriveSeed(appID, days, len(episodes), today)
	}
	sim := simulate(days, episodes, fit, season, today, paths, seed)

	f.Curve, f.Horizons = sim.curve(today, f.CurrentPrice)
	f.Next = sim.nextSale()
	f.Score = int(math.Round(100 * sim.pLowerBy[180]))
	f.Confidence = confidence(fit.model, len(episodes), len(days))
	return f
}

// ---- preparing the series ----

func truncate(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// buildDays turns recorded points into one entry per calendar day from the
// first record to today, carrying the last known price over gaps, and
// resolves each day's regular price.
func buildDays(points []Point, today time.Time) []day {
	byDate := map[time.Time]Point{}
	var first time.Time
	for _, p := range points {
		d := truncate(p.Date)
		if d.After(today) || p.Price < 0 || math.IsNaN(p.Price) {
			continue
		}
		byDate[d] = p
		if first.IsZero() || d.Before(first) {
			first = d
		}
	}
	if len(byDate) == 0 {
		return nil
	}

	var days []day
	var prev Point
	for d := first; !d.After(today); d = d.AddDate(0, 0, 1) {
		p, ok := byDate[d]
		if !ok {
			p = prev
		}
		prev = p
		days = append(days, day{date: d, price: p.Price, regular: p.Regular, real: ok})
	}

	// A day without a recorded regular price uses the highest price of the
	// trailing year as its reference.
	for i := range days {
		if days[i].regular <= 0 {
			from := max(0, i-365)
			hi := days[i].price
			for j := from; j <= i; j++ {
				hi = math.Max(hi, days[j].price)
			}
			days[i].regular = hi
		}
		days[i].regular = math.Max(days[i].regular, days[i].price)
	}
	return days
}

func discount(d day) float64 {
	if d.regular <= 0 {
		return 0
	}
	return math.Max(0, (d.regular-d.price)/d.regular)
}

func minPrice(days []day) float64 {
	lo := math.Inf(1)
	for _, d := range days {
		lo = math.Min(lo, d.price)
	}
	return lo
}

// findEpisodes returns the runs of sale days. Two runs separated by a single
// full-price day are merged: stores sometimes blip a price for a day.
func findEpisodes(days []day) []episode {
	var eps []episode
	in := false
	var cur episode
	for i, d := range days {
		on := discount(d) >= SaleMinDiscount-1e-9
		switch {
		case on && !in:
			cur = episode{start: i, end: i, depth: discount(d)}
			in = true
		case on && in:
			cur.end = i
			cur.depth = math.Max(cur.depth, discount(d))
		case !on && in:
			eps = append(eps, cur)
			in = false
		}
	}
	if in {
		eps = append(eps, cur)
	}

	merged := eps[:0:0]
	for _, e := range eps {
		if n := len(merged); n > 0 && e.start-merged[n-1].end <= 2 {
			merged[n-1].end = e.end
			merged[n-1].depth = math.Max(merged[n-1].depth, e.depth)
			continue
		}
		merged = append(merged, e)
	}
	return merged
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if n := len(s); n%2 == 1 {
		return s[n/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

func typicalSale(eps []episode, regular float64) TypicalSale {
	t := TypicalSale{Count: len(eps), MedianDurationDays: defaultLength}
	if len(eps) == 0 {
		return t
	}
	depths := make([]float64, len(eps))
	var lengths, intervals []float64
	for i, e := range eps {
		depths[i] = e.depth
		lengths = append(lengths, float64(e.length()))
		if i > 0 {
			intervals = append(intervals, float64(e.start-eps[i-1].start))
		}
	}
	t.MedianDepthPercent = round2(100 * median(depths))
	t.MedianPrice = round2(regular * (1 - median(depths)))
	t.MedianDurationDays = max(1, int(math.Round(median(lengths))))
	if len(intervals) > 0 {
		v := int(math.Round(median(intervals)))
		t.MedianIntervalDays = &v
	}
	return t
}

// ---- fitting when sales start ----

type renewal struct {
	model string
	shape float64 // Weibull shape k; 1 is a memoryless process
	scale float64 // Weibull scale, in days
}

// fitRenewal fits the time between sale starts. With regular spacing (low
// variation) the Weibull shape is large, so a sale becomes more likely the
// longer it has been since the last one. With erratic spacing the shape is 1.
func fitRenewal(eps []episode, spanDays int) renewal {
	span := float64(spanDays)
	switch {
	case len(eps) == 0:
		return renewal{ModelNoSales, 1, 2 * math.Max(span, 365)}
	case len(eps) < 3:
		return renewal{ModelPoisson, 1, math.Max(30, span/float64(len(eps)))}
	}
	var gaps []float64
	for i := 1; i < len(eps); i++ {
		gaps = append(gaps, float64(eps[i].start-eps[i-1].start))
	}
	mean, sd := meanStd(gaps)
	cv := math.Max(0.05, sd/mean)
	k := math.Min(4, math.Max(0.8, math.Pow(cv, -1.086)))
	return renewal{ModelWeibull, k, mean / math.Gamma(1+1/k)}
}

func meanStd(xs []float64) (float64, float64) {
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var ss float64
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(ss / float64(len(xs)))
}

// hazardTable[a] is the chance a sale starts tomorrow given the last one
// started a days ago.
func (r renewal) hazardTable(maxAge int) []float64 {
	h := make([]float64, maxAge+1)
	for a := range h {
		x := float64(a)
		cum := math.Pow((x+1)/r.scale, r.shape) - math.Pow(x/r.scale, r.shape)
		h[a] = math.Min(maxDailyHazard, 1-math.Exp(-cum))
	}
	return h
}

// ---- seasonality ----

// Approximate Steam seasonal-sale windows as zero-based days of a non-leap
// year, used only when there is too little history to learn seasonality.
var steamSaleWindows = [][2]int{{25, 36}, {72, 79}, {177, 191}, {326, 335}, {353, 365}}

const seasonDays = 366

// seasonality returns, for each day of the year, how much more (or less)
// likely a sale start is than average. It is learned from the days of the year
// on which past sales started, shrunk toward 1 when history is short. With
// under a year of history, or only a few sales, it falls back to the Steam
// seasonal-sale calendar.
func seasonality(days []day, eps []episode, years float64) [seasonDays]float64 {
	var m [seasonDays]float64
	for i := range m {
		m[i] = 1
	}

	if len(eps) >= 4 && years >= 1 {
		const sigma = 12.0
		var intensity [seasonDays]float64
		var total float64
		for d := range intensity {
			for _, e := range eps {
				dist := circular(d, days[e.start].date.YearDay()-1)
				intensity[d] += math.Exp(-0.5 * dist * dist / (sigma * sigma))
			}
			total += intensity[d]
		}
		mean := total / seasonDays
		weight := math.Min(1, math.Max(0, (years-0.8)/2.2)) * math.Min(1, float64(len(eps))/8)
		for d := range m {
			norm := math.Min(3, math.Max(0.3, intensity[d]/mean))
			m[d] = 1 + weight*(norm-1)
		}
		return m
	}

	const inside, outside = 1.8, 0.84
	for d := range m {
		prior := outside
		for _, w := range steamSaleWindows {
			if d >= w[0] && d <= w[1] {
				prior = inside
			}
		}
		if d <= 1 { // the winter sale runs into early January
			prior = inside
		}
		m[d] = 1 + 0.6*(prior-1)
	}
	return m
}

func circular(a, b int) float64 {
	d := math.Abs(float64(a - b))
	return math.Min(d, seasonDays-d)
}

// ---- simulation ----

type simulation struct {
	paths      int
	pOn        []float64 // by day 1..HorizonDays (index 0 unused)
	expPrice   []float64
	expLow     []float64
	pLowerBy   []float64
	firstStart []int // per path; -1 if no new sale within the horizon
}

func deriveSeed(appID int, days []day, episodes int, today time.Time) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	for _, v := range []uint64{
		uint64(appID), uint64(days[len(days)-1].date.Unix()), uint64(len(days)),
		uint64(math.Round(days[len(days)-1].price * 100)), uint64(episodes), uint64(today.Unix()),
	} {
		binary.LittleEndian.PutUint64(buf[:], v)
		h.Write(buf[:])
	}
	return h.Sum64()
}

func simulate(days []day, eps []episode, fit renewal, season [seasonDays]float64, today time.Time, paths int, seed uint64) simulation {
	last := days[len(days)-1]
	regular := last.regular
	current := last.price

	// Sale depths and durations are drawn from the game's own past sales.
	depths := make([]float64, 0, len(eps))
	lengths := make([]int, 0, len(eps))
	for i, e := range eps {
		depths = append(depths, e.depth)
		ongoing := i == len(eps)-1 && e.end == len(days)-1
		if !ongoing {
			lengths = append(lengths, e.length())
		}
	}
	if len(depths) == 0 {
		depths = []float64{defaultDepth}
	}
	if len(lengths) == 0 {
		lengths = []int{defaultLength}
	}
	sort.Ints(lengths)
	medLen := lengths[len(lengths)/2]

	// Where the game stands today.
	onLeft, sinceStart, curDepth := 0, len(days), 0.0
	if n := len(eps); n > 0 {
		sinceStart = len(days) - 1 - eps[n-1].start
		if eps[n-1].end == len(days)-1 {
			onLeft = max(1, medLen-eps[n-1].length())
			curDepth = discount(last)
		}
	}

	hazard := fit.hazardTable(sinceStart + HorizonDays + 2)
	rng := rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
	threshold := current * (1 - DropThreshold)

	s := simulation{
		paths: paths, pOn: make([]float64, HorizonDays+1), expPrice: make([]float64, HorizonDays+1),
		expLow: make([]float64, HorizonDays+1), pLowerBy: make([]float64, HorizonDays+1),
		firstStart: make([]int, paths),
	}
	dropCount := make([]int, HorizonDays+1)
	onCount := make([]int, HorizonDays+1)

	for p := 0; p < paths; p++ {
		left, since, depth := onLeft, sinceStart, curDepth
		low := current
		dropped := false
		s.firstStart[p] = -1

		for t := 1; t <= HorizonDays; t++ {
			doy := today.AddDate(0, 0, t).YearDay() - 1
			price := regular
			switch {
			case left > 0:
				left--
				price = regular * (1 - depth)
			default:
				if rng.Float64() < math.Min(maxDailyHazard, hazard[min(since, len(hazard)-1)]*season[doy]) {
					depth = depths[rng.IntN(len(depths))]
					left = lengths[rng.IntN(len(lengths))] - 1
					since = 0
					price = regular * (1 - depth)
					if s.firstStart[p] < 0 {
						s.firstStart[p] = t
					}
				}
			}
			since++

			if price < regular-1e-9 {
				onCount[t]++
			}
			s.expPrice[t] += price
			if price < low {
				low = price
			}
			s.expLow[t] += low
			if !dropped && low <= threshold+1e-9 {
				dropped = true
			}
			if dropped {
				dropCount[t]++
			}
		}
	}

	n := float64(paths)
	for t := 1; t <= HorizonDays; t++ {
		s.pOn[t] = float64(onCount[t]) / n
		s.expPrice[t] /= n
		s.expLow[t] /= n
		s.pLowerBy[t] = float64(dropCount[t]) / n
	}
	return s
}

func (s simulation) curve(today time.Time, current float64) ([]CurvePoint, []Horizon) {
	var points []CurvePoint
	add := func(t int) {
		points = append(points, CurvePoint{
			Date: today.AddDate(0, 0, t).Format("2006-01-02"), DaysAhead: t,
			ExpectedPrice: round2(s.expPrice[t]), POnSale: round4(s.pOn[t]), PLowerBy: round4(s.pLowerBy[t]),
		})
	}
	add(1)
	for t := 7; t < HorizonDays; t += 7 {
		add(t)
	}
	add(HorizonDays)

	horizons := make([]Horizon, 0, len(horizonDays))
	for _, h := range horizonDays {
		horizons = append(horizons, Horizon{
			Days: h, PLower: round4(s.pLowerBy[h]), ExpectedPrice: round2(s.expPrice[h]),
			ExpectedLow: round2(math.Min(current, s.expLow[h])),
		})
	}
	return points, horizons
}

// nextSale reads the time to the next new sale off the simulated futures.
func (s simulation) nextSale() NextSale {
	var starts []int
	for _, t := range s.firstStart {
		if t >= 0 {
			starts = append(starts, t)
		}
	}
	sort.Ints(starts)
	at := func(q float64) *int {
		idx := int(math.Ceil(q*float64(s.paths))) - 1
		if idx < 0 || idx >= len(starts) {
			return nil
		}
		v := starts[idx]
		return &v
	}
	return NextSale{P25Days: at(0.25), MedianDays: at(0.5), P75Days: at(0.75)}
}

// confidence is a 0-1 measure of how much evidence the forecast rests on.
func confidence(model string, sales, historyDays int) float64 {
	if model == ModelNoSales {
		return 0.15
	}
	c := 0.6*math.Min(1, float64(sales)/8) + 0.3*math.Min(1, float64(historyDays)/730)
	if model == ModelWeibull {
		c += 0.1
	}
	return round2(math.Min(1, c))
}
