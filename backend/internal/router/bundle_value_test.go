package router_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func value(t *testing.T, resp testutil.Response) map[string]any {
	t.Helper()
	if resp.Code() != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
	}
	return resp.JSON()["value"].(map[string]any)
}

func codesOf(v map[string]any) map[string]float64 {
	out := map[string]float64{}
	for _, r := range v["reasons"].([]any) {
		m := r.(map[string]any)
		out[m["code"].(string)] = m["impact"].(float64)
	}
	return out
}

func TestBundleValueComparesAgainstTheGamesPrices(t *testing.T) {
	app := testutil.NewApp(t)
	// A tracked game: our own scraped price ($9.99, regular $19.99) must win
	// over whatever the bundle page claimed for it.
	app.SeedGame(810001, "Tracked One", []string{"d"}, []string{"p"})
	app.SeedBundle(6100, "Value Pack",
		models.BundleGame{AppID: 810001, Name: "Tracked One", Price: 1, RegularPrice: 2},
		models.BundleGame{AppID: 810002, Name: "Untracked Two", Price: 5, RegularPrice: 10},
		models.BundleGame{AppID: 810003, Name: "Untracked Three", Price: 15, RegularPrice: 30},
	)
	c := app.NewClient()

	resp := c.Get("/api/bundles/6100")
	v := value(t, resp)

	totals := v["totals"].(map[string]any)
	// 9.99 + 5 + 15 now; 19.99 + 10 + 30 regular; the bundle costs $20.
	if totals["separate"] != 29.99 || totals["regular"] != 59.99 || totals["bundle"] != float64(20) || totals["priced_items"] != float64(3) {
		t.Errorf("totals = %v, want separate 29.99, regular 59.99, bundle 20, 3 priced", totals)
	}
	if v["savings_vs_separate"] != 9.99 || v["savings_vs_regular"] != 39.99 {
		t.Errorf("savings = %v / %v", v["savings_vs_separate"], v["savings_vs_regular"])
	}
	if v["verdict"] != "great_deal" {
		t.Errorf("verdict = %v (score %v, %v), want great_deal", v["verdict"], v["score"], codesOf(v))
	}

	items := v["items"].([]any)
	byID := map[float64]map[string]any{}
	for _, it := range items {
		m := it.(map[string]any)
		byID[m["app_id"].(float64)] = m
	}
	if byID[810001]["price"] != 9.99 || byID[810001]["regular_price"] != 19.99 {
		t.Errorf("the tracked game should use our scraped prices, got %v / %v", byID[810001]["price"], byID[810001]["regular_price"])
	}
	if byID[810002]["price"] != float64(5) || byID[810003]["regular_price"] != float64(30) {
		t.Errorf("untracked games should use the bundle page's prices: %v %v", byID[810002], byID[810003])
	}
	// The $5 game's share is 20 x 10/59.99, about $3.33, so $5 alone is dearer.
	if byID[810002]["cheaper_alone"] != false || byID[810002]["bundle_share"].(float64) < 3 {
		t.Errorf("share for the $10 game = %v", byID[810002]["bundle_share"])
	}

	// The game list in the same response carries the prices too.
	for _, g := range resp.JSON()["games"].([]any) {
		m := g.(map[string]any)
		if m["app_id"] == float64(810001) && m["price"] != 9.99 {
			t.Errorf("games[].price = %v, want 9.99", m["price"])
		}
	}
}

func TestBundleDearerThanItsGames(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedBundle(6101, "Bad Pack", // SeedBundle prices the bundle at $20
		models.BundleGame{AppID: 820001, Name: "A", Price: 6, RegularPrice: 30},
		models.BundleGame{AppID: 820002, Name: "B", Price: 6, RegularPrice: 30},
	)
	v := value(t, app.NewClient().Get("/api/bundles/6101"))

	if v["verdict"] != "poor_value" && v["verdict"] != "fair" {
		t.Errorf("verdict = %v, want a warning against a bundle ($20) dearer than its games ($12 together)", v["verdict"])
	}
	if _, ok := codesOf(v)["dearer_than_separate"]; !ok {
		t.Errorf("reasons = %v", codesOf(v))
	}
	if v["savings_vs_separate"].(float64) >= 0 {
		t.Errorf("savings vs separate = %v, want negative", v["savings_vs_separate"])
	}
	if v["cheaper_alone_count"] != float64(2) {
		t.Errorf("cheaper alone count = %v, want both games", v["cheaper_alone_count"])
	}
}

func TestBundleWithUnknownPricesIsNotJudged(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedBundle(6102, "Mystery Pack",
		models.BundleGame{AppID: 830001, Name: "A"},
		models.BundleGame{AppID: 830002, Name: "B"},
	)
	v := value(t, app.NewClient().Get("/api/bundles/6102"))
	if v["verdict"] != "not_enough_data" || v["completeness"] != float64(0) {
		t.Errorf("verdict %v completeness %v", v["verdict"], v["completeness"])
	}
	if reasons := v["reasons"].([]any); reasons == nil {
		t.Error("reasons must be an empty list, not null")
	}
}

func TestBundleRecordLow(t *testing.T) {
	app := testutil.NewApp(t)
	// SeedBundle records today's price under the machine's local date.
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	// Bundle 6103 is $20 today; it has been $30, $30, then $25 before.
	app.SeedBundle(6103, "Falling Pack",
		models.BundleGame{AppID: 840001, Name: "A", Price: 20, RegularPrice: 40},
		models.BundleGame{AppID: 840002, Name: "B", Price: 20, RegularPrice: 40},
	)
	// Bundle 6104 is $20 today but was $15 last month.
	app.SeedBundle(6104, "Rising Pack",
		models.BundleGame{AppID: 850001, Name: "A", Price: 20, RegularPrice: 40},
		models.BundleGame{AppID: 850002, Name: "B", Price: 20, RegularPrice: 40},
	)
	// Bundle 6105 has never changed price.
	app.SeedBundle(6105, "Flat Pack",
		models.BundleGame{AppID: 860001, Name: "A", Price: 20, RegularPrice: 40},
		models.BundleGame{AppID: 860002, Name: "B", Price: 20, RegularPrice: 40},
	)
	insert := func(id int, daysAgo int, price float64) {
		if _, err := app.Pool.Exec(t.Context(),
			`INSERT INTO bundle_price_history (bundle_id, recorded_date, price, original_price, discount_percentage) VALUES ($1, $2::date, $3, 40, 0)`,
			id, today.AddDate(0, 0, -daysAgo).Format(time.DateOnly), price); err != nil {
			t.Fatal(err)
		}
	}
	insert(6103, 3, 30)
	insert(6103, 2, 30)
	insert(6103, 1, 25)
	insert(6104, 30, 15)
	insert(6104, 1, 25)

	c := app.NewClient()
	detail := func(id string) map[string]any { return c.Get("/api/bundles/" + id).JSON() }

	low := detail("6103")
	if low["at_record_low"] != true || low["record_low"] != float64(20) || low["history_days"] != float64(4) {
		t.Errorf("falling bundle: at_record_low=%v record_low=%v history_days=%v", low["at_record_low"], low["record_low"], low["history_days"])
	}
	if value(t, c.Get("/api/bundles/6103"))["at_record_low"] != true {
		t.Error("the value block should agree that it is a record low")
	}
	if _, ok := codesOf(low["value"].(map[string]any))["bundle_record_low"]; !ok {
		t.Errorf("reasons = %v, want bundle_record_low", codesOf(low["value"].(map[string]any)))
	}

	high := detail("6104")
	if high["at_record_low"] != false || high["record_low"] != float64(15) {
		t.Errorf("a bundle that was cheaper before is not a record low: %v / %v", high["at_record_low"], high["record_low"])
	}
	if _, ok := codesOf(high["value"].(map[string]any))["above_bundle_record_low"]; !ok {
		t.Errorf("$20 vs a $15 low should be flagged: %v", codesOf(high["value"].(map[string]any)))
	}

	if detail("6105")["at_record_low"] != false {
		t.Error("a bundle whose price never moved has no record low")
	}

	// The list endpoint carries the flag for the cards.
	flags := map[float64]bool{}
	for _, b := range c.Get("/api/bundles").JSONArray() {
		m := b.(map[string]any)
		flags[m["bundle_id"].(float64)] = m["at_record_low"].(bool)
	}
	if !flags[6103] || flags[6104] || flags[6105] {
		t.Errorf("list flags = %v, want only 6103", flags)
	}
}
