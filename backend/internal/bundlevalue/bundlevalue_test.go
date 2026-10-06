package bundlevalue

import (
	"math"
	"strings"
	"testing"
)

func items(prices ...[2]float64) []Item {
	var out []Item
	for i, p := range prices {
		out = append(out, Item{AppID: i + 1, Name: string(rune('A' + i)), Price: p[0], Regular: p[1]})
	}
	return out
}

func codes(v Value) map[string]int {
	m := map[string]int{}
	for _, r := range v.Reasons {
		m[r.Code] = r.Impact
	}
	return m
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.005 }

func TestAGenuinelyGoodBundle(t *testing.T) {
	// Three $20 games on sale to $12 each ($36 together), bundled for $20,
	// and at the lowest price this bundle has had.
	v := Evaluate(Input{
		BundlePrice: 20, DiscountPercent: 44,
		Items:  items([2]float64{12, 20}, [2]float64{12, 20}, [2]float64{12, 20}),
		Prices: []float64{36, 36, 30, 30, 20, 20},
	})

	if v.Verdict != GreatDeal {
		t.Fatalf("verdict = %s (score %d, %v), want great_deal", v.Verdict, v.Score, codes(v))
	}
	if !near(v.Totals.Regular, 60) || !near(v.Totals.Separate, 36) || !near(v.Totals.Bundle, 20) {
		t.Errorf("totals = %+v", v.Totals)
	}
	if !near(v.SavingsVsSeparate, 16) || !near(v.SavingsVsSeparatePercent, 44.44) {
		t.Errorf("vs separate: $%v (%v%%), want $16 (44.44%%)", v.SavingsVsSeparate, v.SavingsVsSeparatePercent)
	}
	if !near(v.SavingsVsRegular, 40) || !near(v.SavingsVsRegularPercent, 66.67) {
		t.Errorf("vs regular: $%v (%v%%), want $40 (66.67%%)", v.SavingsVsRegular, v.SavingsVsRegularPercent)
	}
	got := codes(v)
	if got["cheaper_than_separate"] != 30 || got["deep_vs_regular"] != 15 || got["bundle_record_low"] != 15 {
		t.Errorf("reasons = %v, want cheaper_than_separate +30, deep_vs_regular +15, bundle_record_low +15", got)
	}
	if v.Score != 100 || !v.AtRecordLow || v.RecordLow != 20 {
		t.Errorf("score %d, at_record_low %v, record low %v", v.Score, v.AtRecordLow, v.RecordLow)
	}
	if v.Completeness != 1 || v.CheaperAloneCount != 0 {
		t.Errorf("completeness %v, cheaper alone %d", v.Completeness, v.CheaperAloneCount)
	}
}

func TestABundleDearerThanTheGamesOnSale(t *testing.T) {
	// The bundle's own "discount" looks fine against regular prices, but the
	// games are on a bigger sale individually: $20 + $20 now, bundle $50.
	v := Evaluate(Input{
		BundlePrice: 50, DiscountPercent: 17,
		Items: items([2]float64{20, 30}, [2]float64{20, 30}),
	})
	if v.Verdict != PoorValue {
		t.Fatalf("verdict = %s (score %d, %v), want poor_value", v.Verdict, v.Score, codes(v))
	}
	if codes(v)["dearer_than_separate"] >= 0 {
		t.Errorf("reasons = %v, want dearer_than_separate to count against it", codes(v))
	}
	if v.SavingsVsSeparate >= 0 || !near(v.SavingsVsSeparate, -10) {
		t.Errorf("savings vs separate = %v, want -10 (the bundle costs $10 more)", v.SavingsVsSeparate)
	}
	if v.SavingsVsRegular <= 0 {
		t.Errorf("vs regular it still looks cheaper ($%v) - that is exactly the trap", v.SavingsVsRegular)
	}
	if v.CheaperAloneCount != 2 {
		t.Errorf("cheaper alone = %d, want both games", v.CheaperAloneCount)
	}
}

func TestACheapBundleOfFullPriceGames(t *testing.T) {
	// Games at full price today; the bundle is a real discount on them.
	v := Evaluate(Input{
		BundlePrice: 30, DiscountPercent: 50,
		Items: items([2]float64{30, 30}, [2]float64{30, 30}),
	})
	if v.Verdict != GoodDeal && v.Verdict != GreatDeal {
		t.Errorf("verdict = %s (score %d), want a good deal", v.Verdict, v.Score)
	}
	if codes(v)["cheaper_than_separate"] != 30 {
		t.Errorf("reasons = %v", codes(v))
	}
}

func TestTheSamePriceAsBuyingSeparately(t *testing.T) {
	v := Evaluate(Input{BundlePrice: 40, DiscountPercent: 5, Items: items([2]float64{20, 20}, [2]float64{20, 20})})
	if _, ok := codes(v)["about_same_as_separate"]; !ok {
		t.Errorf("reasons = %v, want about_same_as_separate", codes(v))
	}
	if v.Verdict != PoorValue && v.Verdict != Fair {
		t.Errorf("verdict = %s", v.Verdict)
	}
}

func TestEachGamesShareOfTheBundle(t *testing.T) {
	// A $30 and a $10 game bundled for $20: shares split 75/25 by regular price.
	v := Evaluate(Input{
		BundlePrice: 20, DiscountPercent: 50,
		Items: items([2]float64{30, 30}, [2]float64{4, 10}),
	})
	big, small := v.Items[0], v.Items[1]
	if !near(big.BundleShare, 15) || !near(small.BundleShare, 5) {
		t.Errorf("shares = %v and %v, want 15 and 5", big.BundleShare, small.BundleShare)
	}
	if big.CheaperAlone || !small.CheaperAlone {
		t.Errorf("cheaper alone: big=%v small=%v; the $4 game costs less than its $5 share", big.CheaperAlone, small.CheaperAlone)
	}
	if small.DiscountPercent != 60 || big.DiscountPercent != 0 {
		t.Errorf("discounts: %d%% and %d%%", big.DiscountPercent, small.DiscountPercent)
	}
	if v.CheaperAloneCount != 1 {
		t.Errorf("cheaper alone count = %d", v.CheaperAloneCount)
	}
	sum := big.BundleShare + small.BundleShare
	if !near(sum, 20) {
		t.Errorf("shares add up to %v, want the bundle price 20", sum)
	}
}

func TestMostGamesCheaperAloneCountsAgainstTheBundle(t *testing.T) {
	v := Evaluate(Input{
		BundlePrice: 40, DiscountPercent: 20,
		Items: items([2]float64{10, 20}, [2]float64{10, 20}, [2]float64{20, 20}),
	})
	if codes(v)["most_cheaper_alone"] != -10 {
		t.Errorf("reasons = %v, want most_cheaper_alone -10", codes(v))
	}
}

func TestMissingPricesWeakenTheVerdictAndSayWhy(t *testing.T) {
	full := Evaluate(Input{BundlePrice: 20, DiscountPercent: 50, Items: items([2]float64{20, 40}, [2]float64{20, 40})})
	partial := Evaluate(Input{BundlePrice: 20, DiscountPercent: 50, Items: append(items([2]float64{20, 40}, [2]float64{20, 40}), Item{AppID: 9, Name: "Mystery"})})

	if partial.Completeness != 0.6667 || partial.Totals.PricedItems != 2 || partial.Totals.Items != 3 {
		t.Errorf("completeness %v, totals %+v", partial.Completeness, partial.Totals)
	}
	if _, ok := codes(partial)["some_prices_unknown"]; !ok {
		t.Errorf("reasons = %v, want some_prices_unknown", codes(partial))
	}
	if partial.Score >= full.Score {
		t.Errorf("an incomplete picture should push the score less: full %d, partial %d", full.Score, partial.Score)
	}
	if partial.Items[2].Priced || partial.Items[2].BundleShare != 0 {
		t.Errorf("the unpriced game has no share: %+v", partial.Items[2])
	}
}

func TestNotEnoughDataToJudge(t *testing.T) {
	for name, in := range map[string]Input{
		"no games":              {BundlePrice: 10},
		"one priced game":       {BundlePrice: 10, Items: items([2]float64{20, 20})},
		"no prices known":       {BundlePrice: 10, Items: items([2]float64{0, 0}, [2]float64{0, 0})},
		"free bundle price":     {BundlePrice: 0, Items: items([2]float64{10, 10}, [2]float64{10, 10})},
		"negative bundle price": {BundlePrice: -5, Items: items([2]float64{10, 10}, [2]float64{10, 10})},
	} {
		v := Evaluate(in)
		if v.Verdict != NoData || v.Score != 50 {
			t.Errorf("%s: verdict %s score %d, want not_enough_data / 50", name, v.Verdict, v.Score)
		}
		if v.Reasons == nil || v.Items == nil {
			t.Errorf("%s: lists must be empty, not null", name)
		}
	}
}

func TestRecordLowStillReportedWithoutEnoughPrices(t *testing.T) {
	v := Evaluate(Input{BundlePrice: 10, DiscountPercent: 30, Prices: []float64{15, 15, 10}})
	if v.Verdict != NoData || !v.AtRecordLow || v.RecordLow != 10 {
		t.Errorf("verdict %s, at_record_low %v, low %v", v.Verdict, v.AtRecordLow, v.RecordLow)
	}
}

func TestRecordLow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		prices   []float64
		current  float64
		discount int
		wantLow  float64
		want     bool
	}{
		{"at the lowest", []float64{30, 30, 20, 25, 20}, 20, 33, 20, true},
		{"within half a cent", []float64{30, 20}, 20.004, 33, 20, true},
		{"above the lowest", []float64{30, 20, 25}, 25, 17, 20, false},
		{"never changed price", []float64{20, 20, 20}, 20, 30, 20, false},
		{"not discounted", []float64{30, 20, 30}, 20, 0, 20, false},
		{"no history", nil, 20, 30, 0, false},
		{"zero prices are ignored", []float64{0, 0, 15, 20}, 15, 25, 15, true},
	} {
		low, at := recordLow(tc.prices, tc.current, tc.discount)
		if low != tc.wantLow || at != tc.want {
			t.Errorf("%s: low=%v at=%v, want low=%v at=%v", tc.name, low, at, tc.wantLow, tc.want)
		}
	}
}

func TestBundleHistoryRules(t *testing.T) {
	base := Input{BundlePrice: 30, DiscountPercent: 25, Items: items([2]float64{20, 30}, [2]float64{20, 30})}

	near := base
	near.Prices = []float64{40, 29, 30}
	if codes(Evaluate(near))["near_bundle_record_low"] != 7 {
		t.Errorf("within 5%% of the low: %v", codes(Evaluate(near)))
	}

	above := base
	above.Prices = []float64{40, 20, 30}
	if codes(Evaluate(above))["above_bundle_record_low"] != -10 {
		t.Errorf("50%% above the low: %v", codes(Evaluate(above)))
	}

	flat := base
	flat.Prices = []float64{30, 30, 30}
	got := codes(Evaluate(flat))
	for _, c := range []string{"bundle_record_low", "near_bundle_record_low", "above_bundle_record_low"} {
		if _, ok := got[c]; ok {
			t.Errorf("a bundle whose price never moved should not get %s: %v", c, got)
		}
	}
}

func TestScoreAndVerdictBounds(t *testing.T) {
	for _, in := range []Input{
		{BundlePrice: 1, DiscountPercent: 99, Items: items([2]float64{50, 50}, [2]float64{50, 50}), Prices: []float64{100, 1}},
		{BundlePrice: 500, Items: items([2]float64{1, 100}, [2]float64{1, 100}), Prices: []float64{10, 500}},
	} {
		v := Evaluate(in)
		if v.Score < 0 || v.Score > 100 {
			t.Errorf("score %d out of range", v.Score)
		}
		switch v.Verdict {
		case GreatDeal, GoodDeal, Fair, PoorValue:
		default:
			t.Errorf("verdict %q", v.Verdict)
		}
	}
	if Evaluate(Input{BundlePrice: 1, DiscountPercent: 99, Items: items([2]float64{50, 50}, [2]float64{50, 50}), Prices: []float64{100, 1}}).Verdict != GreatDeal {
		t.Error("a bundle 99 percent off at its record low should be a great deal")
	}
	if Evaluate(Input{BundlePrice: 500, Items: items([2]float64{1, 100}, [2]float64{1, 100}), Prices: []float64{10, 500}}).Verdict != PoorValue {
		t.Error("a bundle far dearer than its games should be poor value")
	}
}

func TestVerdictThresholds(t *testing.T) {
	// Pin the boundaries so a change to them is deliberate.
	if greatScore != 75 || goodScore != 60 || fairScore != 40 {
		t.Errorf("thresholds changed: %d/%d/%d", greatScore, goodScore, fairScore)
	}
}

func TestReasonTextIsReadable(t *testing.T) {
	v := Evaluate(Input{BundlePrice: 20, DiscountPercent: 44, Items: items([2]float64{12, 20}, [2]float64{12, 20}, [2]float64{12, 20}), Prices: []float64{36, 20}})
	for _, r := range v.Reasons {
		if r.Text == "" || strings.Contains(r.Text, "%!") || strings.Contains(r.Text, "NaN") {
			t.Errorf("bad reason text for %s: %q", r.Code, r.Text)
		}
	}
	if !strings.Contains(v.Reasons[0].Text, "$36.00") || !strings.Contains(v.Reasons[0].Text, "$20.00") {
		t.Errorf("text should quote both prices: %q", v.Reasons[0].Text)
	}
}
