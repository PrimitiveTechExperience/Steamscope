package router_test

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/prediction"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func utcToday() time.Time {
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// periodicHistory is `days` days ending today: a 50% sale lasting 10 days every
// 60 days off a $20 regular price. The last day is a full-price day, 30 days
// after the previous sale ended, so the next sale is due in about three weeks.
func periodicHistory(days int) []models.PricePoint {
	today := utcToday()
	var pts []models.PricePoint
	for i := 0; i < days; i++ {
		// Count back from today so the last sale ended exactly 30 days ago.
		back := days - 1 - i
		price, disc := 20.0, 0
		if back >= 30 && (back-30)%60 < 10 {
			price, disc = 10, 50
		}
		pts = append(pts, models.PricePoint{
			Date: today.AddDate(0, 0, -back), Price: price, OriginalPrice: 20, DiscountPercentage: disc,
		})
	}
	return pts
}

func TestPredictionEndpoint(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(7001, "Cyclic Game", []string{"d"}, []string{"p"})
	app.SeedPriceHistory(7001, periodicHistory(730))
	c := app.NewClient()

	t.Run("rejects a bad id with 400 and an unknown game with 404", func(t *testing.T) {
		for _, id := range []string{"abc", "0", "-3", "1.5"} {
			if got := c.Get("/api/games/" + id + "/prediction").Code(); got != http.StatusBadRequest {
				t.Errorf("id %q: status = %d, want 400", id, got)
			}
		}
		resp := c.Get("/api/games/999999/prediction")
		if resp.Code() != http.StatusNotFound || resp.Error() == "" {
			t.Errorf("unknown game: status = %d body = %s, want a JSON 404", resp.Code(), resp.Body.String())
		}
		if got := c.Get("/api/games/999999/advice").Code(); got != http.StatusNotFound {
			t.Errorf("advice for an unknown game: status = %d, want 404", got)
		}
	})

	t.Run("returns a forecast for two years ahead", func(t *testing.T) {
		resp := c.Get("/api/games/7001/prediction")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		f := resp.JSON()
		if f["model"] != "weibull_renewal" || f["app_id"] != float64(7001) || f["current_price"] != float64(20) || f["on_sale"] != false {
			t.Errorf("forecast = model %v app %v price %v on_sale %v", f["model"], f["app_id"], f["current_price"], f["on_sale"])
		}
		curve := f["curve"].([]any)
		first, last := curve[0].(map[string]any), curve[len(curve)-1].(map[string]any)
		if first["days_ahead"] != float64(1) || last["days_ahead"] != float64(730) {
			t.Errorf("curve spans days %v..%v, want 1..730", first["days_ahead"], last["days_ahead"])
		}
		if first["date"] != utcToday().AddDate(0, 0, 1).Format("2006-01-02") {
			t.Errorf("first date = %v, want tomorrow", first["date"])
		}
		if len(f["horizons"].([]any)) != 5 {
			t.Errorf("horizons = %v", f["horizons"])
		}
		if score := f["score"].(float64); score < 80 {
			t.Errorf("score = %v, want high: a sale is due within a month", score)
		}
		typical := f["typical_sale"].(map[string]any)
		if typical["median_depth_percent"] != float64(50) || typical["count"] != float64(12) {
			t.Errorf("typical sale = %v", typical)
		}
	})

	t.Run("the second request is served from the cache", func(t *testing.T) {
		other := app.NewClient()
		first, second := app.NewClient().Get("/api/games/7001/prediction"), other.Get("/api/games/7001/prediction")
		if second.JSON()["cached"] != true {
			t.Error("the repeat request was not cached")
		}
		if first.JSON()["generated_at"] != second.JSON()["generated_at"] || first.JSON()["score"] != second.JSON()["score"] {
			t.Error("cached and fresh results differ")
		}
	})

	t.Run("new price data invalidates the cached forecast", func(t *testing.T) {
		c.Get("/api/games/7001/prediction") // make sure it is cached
		app.SeedPriceHistory(7001, []models.PricePoint{{Date: utcToday().AddDate(0, 0, 1), Price: 20, OriginalPrice: 20}})
		resp := c.Get("/api/games/7001/prediction")
		if resp.JSON()["cached"] != false {
			t.Error("a forecast built from older history was served")
		}
		if c.Get("/api/games/7001/prediction").JSON()["cached"] != true {
			t.Error("the recomputed forecast should be cached again")
		}
	})
}

func TestPredictionWithTooLittleHistory(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(7002, "Brand New", []string{"d"}, []string{"p"})
	app.SeedPriceHistory(7002, periodicHistory(12))
	c := app.NewClient()

	resp := c.Get("/api/games/7002/prediction")
	if resp.Code() != http.StatusOK {
		t.Fatalf("status = %d, want 200 (not an error, just no forecast)", resp.Code())
	}
	f := resp.JSON()
	if f["model"] != "insufficient" || f["current_price"] != float64(20) {
		t.Errorf("forecast = %v", f)
	}
	if body := resp.Body.String(); !strings.Contains(body, `"curve":[]`) || !strings.Contains(body, `"horizons":[]`) {
		t.Errorf("empty lists must be [] not null: %s", body)
	}
	if keys := predictionKeys(app); len(keys) != 0 {
		t.Errorf("an empty forecast took up cache space: %v", keys)
	}

	advice := c.Get("/api/games/7002/advice")
	if advice.Code() != http.StatusOK || advice.JSON()["verdict"] != "not_enough_data" {
		t.Errorf("advice = %d %s", advice.Code(), advice.Body.String())
	}
}

func predictionKeys(app *testutil.App) []string {
	var keys []string
	for _, k := range app.Mini.Keys() {
		if strings.HasPrefix(k, "prediction:") {
			keys = append(keys, k)
		}
	}
	return keys
}

func TestPredictionCacheHoldsFiveGames(t *testing.T) {
	app := testutil.NewApp(t)
	c := app.NewClient()
	for id := 7101; id <= 7107; id++ {
		app.SeedGame(id, fmt.Sprintf("Game %d", id), []string{"d"}, []string{"p"})
		app.SeedPriceHistory(id, periodicHistory(400))
	}
	for id := 7101; id <= 7107; id++ {
		if got := c.Get(fmt.Sprintf("/api/games/%d/prediction", id)).Code(); got != http.StatusOK {
			t.Fatalf("game %d: status %d", id, got)
		}
		time.Sleep(3 * time.Millisecond) // distinct timestamps decide who is evicted
	}

	keys := predictionKeys(app)
	if len(keys) != 5 {
		t.Fatalf("cache holds %d forecasts %v, want 5", len(keys), keys)
	}
	for _, evicted := range []int{7101, 7102} {
		if app.Mini.Exists(fmt.Sprintf("prediction:%d", evicted)) {
			t.Errorf("game %d should have been evicted", evicted)
		}
		if c.Get(fmt.Sprintf("/api/games/%d/prediction", evicted)).JSON()["cached"] != false {
			t.Errorf("game %d should have been recomputed", evicted)
		}
	}
	// Recomputing two games pushed out two more (the oldest of the rest): still five.
	if n := len(predictionKeys(app)); n != 5 {
		t.Errorf("cache grew to %d entries", n)
	}
	members, _ := app.Mini.ZMembers("predictions:recent")
	if len(members) != 5 {
		t.Errorf("recency index has %d entries %v, want 5", len(members), members)
	}
}

func TestPredictionRequestsAreCoalescedAndConsistent(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(7201, "Popular", []string{"d"}, []string{"p"})
	app.SeedPriceHistory(7201, periodicHistory(500))

	const n = 10
	clients := make([]*testutil.Client, n)
	for i := range clients {
		clients[i] = app.NewClient()
	}
	scores := make([]any, n)
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := clients[i].Get("/api/games/7201/prediction")
			codes[i] = resp.Code()
			scores[i] = resp.JSON()["score"]
		}()
	}
	wg.Wait()

	for i := range clients {
		if codes[i] != http.StatusOK || scores[i] != scores[0] {
			t.Errorf("request %d: status %d score %v, want 200 and %v", i, codes[i], scores[i], scores[0])
		}
	}
	if keys := predictionKeys(app); len(keys) != 1 {
		t.Errorf("cache holds %v, want exactly one forecast", keys)
	}
}

func TestPredictionIsRateLimitedPerAddress(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(7301, "Limited", []string{"d"}, []string{"p"})
	app.SeedPriceHistory(7301, periodicHistory(300))
	c := app.NewClient()

	for i := 0; i < 30; i++ {
		if got := c.Get("/api/games/7301/prediction").Code(); got != http.StatusOK {
			t.Fatalf("request %d: status %d, want 200", i+1, got)
		}
	}
	resp := c.Get("/api/games/7301/prediction")
	if resp.Code() != http.StatusTooManyRequests {
		t.Fatalf("31st request: status %d, want 429", resp.Code())
	}
	if got := c.Get("/api/games/7301/advice").Code(); got != http.StatusTooManyRequests {
		t.Errorf("advice shares the limit: status %d, want 429", got)
	}
	if got := app.NewClient().Get("/api/games/7301/prediction").Code(); got != http.StatusOK {
		t.Errorf("another address should not be limited: status %d", got)
	}
}

func TestAdviceEndpoint(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(7401, "Advised", []string{"d"}, []string{"p"})
	app.SeedPriceHistory(7401, periodicHistory(730))

	t.Run("anyone gets general advice", func(t *testing.T) {
		resp := app.NewClient().Get("/api/games/7401/advice")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		a := resp.JSON()
		if a["verdict"] != "wait" {
			t.Errorf("verdict = %v, want wait: full price now with a sale due soon", a["verdict"])
		}
		if a["personalized"] != false || a["patience_days"] != float64(90) {
			t.Errorf("personalized %v patience %v", a["personalized"], a["patience_days"])
		}
		if a["wait_until"] == nil {
			t.Error("a wait verdict should name when the next sale is expected")
		}
		reasons := a["reasons"].([]any)
		if len(reasons) == 0 {
			t.Fatal("advice must explain itself")
		}
		for _, r := range reasons {
			m := r.(map[string]any)
			if m["code"] == "" || m["text"] == "" {
				t.Errorf("reason = %v", m)
			}
		}
		if score := a["score"].(float64); score < 0 || score > 40 {
			t.Errorf("score = %v, want a low buy score for a wait verdict", score)
		}
	})

	t.Run("a signed-in user who is not watching gets general advice", func(t *testing.T) {
		resp := app.NewUser("bystander").Get("/api/games/7401/advice")
		if resp.JSON()["personalized"] != false {
			t.Errorf("advice = %s", resp.Body.String())
		}
	})

	t.Run("a watcher's target price is weighed", func(t *testing.T) {
		met := app.NewUser("hasmet")
		met.Put("/api/me/watchlist/7401", map[string]any{"target_price": 25})
		resp := met.Get("/api/games/7401/advice")
		a := resp.JSON()
		if a["verdict"] != "buy_now" || a["personalized"] != true {
			t.Fatalf("target above the price: %s", resp.Body.String())
		}
		if !hasReason(a, "target_met") {
			t.Errorf("reasons = %v", a["reasons"])
		}

		unmet := app.NewUser("hasnotmet")
		unmet.Put("/api/me/watchlist/7401", map[string]any{"target_price": 8})
		a = unmet.Get("/api/games/7401/advice").JSON()
		if a["verdict"] != "wait" || !hasReason(a, "above_target") || hasReason(a, "target_met") {
			t.Errorf("target below the price: verdict %v reasons %v", a["verdict"], a["reasons"])
		}
	})

	t.Run("how long someone has watched changes the call", func(t *testing.T) {
		user := app.NewUser("longwatch")
		user.Put("/api/me/watchlist/7401", map[string]any{})
		// Pretend they started watching 250 days ago.
		app.Pool.Exec(t.Context(), `UPDATE watched_games SET created_at = now() - interval '250 days' WHERE user_id = $1`, user.UserID)

		a := user.Get("/api/games/7401/advice").JSON()
		if a["patience_days"] != float64(30) {
			t.Errorf("patience = %v, want 30 after 250 days of watching", a["patience_days"])
		}
		if !hasReason(a, "waited_long") {
			t.Errorf("reasons = %v", a["reasons"])
		}
	})

	t.Run("advice and prediction come from the same forecast", func(t *testing.T) {
		c := app.NewClient()
		advice := c.Get("/api/games/7401/advice").JSON()
		prediction := c.Get("/api/games/7401/prediction").JSON()
		if advice["generated_at"] != prediction["generated_at"] {
			t.Errorf("generated_at differs: %v vs %v", advice["generated_at"], prediction["generated_at"])
		}
	})
}

func hasReason(advice map[string]any, code string) bool {
	for _, r := range advice["reasons"].([]any) {
		if r.(map[string]any)["code"] == code {
			return true
		}
	}
	return false
}

func TestPredictionMetrics(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(7501, "Measured", []string{"d"}, []string{"p"})
	app.SeedPriceHistory(7501, periodicHistory(400))
	c := app.NewClient()

	// Counters are shared by the whole test process, so compare before and after.
	scrape := func(name string) float64 {
		c.Headers = map[string]string{"Authorization": "Bearer test-token"}
		for _, line := range strings.Split(c.Get("/metrics").Body.String(), "\n") {
			if strings.HasPrefix(line, name+" ") {
				var v float64
				fmt.Sscanf(strings.TrimPrefix(line, name+" "), "%g", &v)
				return v
			}
		}
		return 0
	}
	const hit = `steamscope_prediction_requests_total{result="hit"}`
	const miss = `steamscope_prediction_requests_total{result="miss"}`
	const computed = `steamscope_prediction_compute_seconds_count`
	hit0, miss0, computed0 := scrape(hit), scrape(miss), scrape(computed)

	c.Headers = nil
	c.Get("/api/games/7501/prediction") // computed
	c.Get("/api/games/7501/prediction") // served from the cache
	c.Get("/api/games/7501/advice")     // served from the cache

	if got := scrape(miss) - miss0; got != 1 {
		t.Errorf("misses grew by %v, want 1", got)
	}
	if got := scrape(hit) - hit0; got != 2 {
		t.Errorf("hits grew by %v, want 2", got)
	}
	if got := scrape(computed) - computed0; got != 1 {
		t.Errorf("computations grew by %v, want 1", got)
	}
}

func TestDeleteFabricatedHistory(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(7601, "Backfilled", []string{"d"}, []string{"p"})
	today := utcToday()

	// Fabricated: ten days of the first event's price before the event.
	// Genuine: ten days from the event on, and one earlier row with a different
	// price (never produced by the old bug, so it must survive).
	var pts []models.PricePoint
	for i := 20; i > 10; i-- {
		pts = append(pts, models.PricePoint{Date: today.AddDate(0, 0, -i), Price: 5, OriginalPrice: 10, DiscountPercentage: 50})
	}
	pts = append(pts, models.PricePoint{Date: today.AddDate(0, 0, -25), Price: 10, OriginalPrice: 10})
	for i := 10; i >= 0; i-- {
		pts = append(pts, models.PricePoint{Date: today.AddDate(0, 0, -i), Price: 5, OriginalPrice: 10, DiscountPercentage: 50})
	}
	app.SeedPriceHistory(7601, pts)

	eventDay := today.AddDate(0, 0, -10)
	n, err := app.DB.DeleteFabricatedHistory(t.Context(), 7601, eventDay, 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 10 {
		t.Errorf("deleted %d rows, want the 10 fabricated ones", n)
	}
	var left int
	app.Pool.QueryRow(t.Context(), `SELECT count(*) FROM price_history WHERE app_id = 7601`).Scan(&left)
	if left != 12 {
		t.Errorf("%d rows left, want 12 (11 genuine since the event + 1 earlier different price)", left)
	}
	if n, _ := app.DB.DeleteFabricatedHistory(t.Context(), 7601, eventDay, 5, 10); n != 0 {
		t.Errorf("a second run deleted %d rows, want 0 (idempotent)", n)
	}
}

func TestCachedForecastsAreTiedToTheModelVersion(t *testing.T) {
	// A forecast made by a different version of the model must never be served:
	// after a model change, old numbers would otherwise linger for hours.
	app := testutil.NewApp(t)
	app.SeedGame(7701, "Versioned", []string{"d"}, []string{"p"})
	app.SeedPriceHistory(7701, periodicHistory(400))
	c := app.NewClient()

	first := c.Get("/api/games/7701/prediction").JSON()
	if v, _ := first["data_version"].(string); !strings.HasPrefix(v, "m"+prediction.ModelVersion+"|") {
		t.Fatalf("data_version = %q, want it to start with the model version m%s|", first["data_version"], prediction.ModelVersion)
	}
	if c.Get("/api/games/7701/prediction").JSON()["cached"] != true {
		t.Fatal("the second request should be cached")
	}

	// Rewrite the stored forecast as if an older model had made it.
	raw, err := app.Mini.Get("prediction:7701")
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Replace(raw, `"data_version":"m`+prediction.ModelVersion+`|`, `"data_version":"m0|`, 1)
	if old == raw {
		t.Fatal("could not find the data version in the cached forecast")
	}
	app.Mini.Set("prediction:7701", old)

	resp := c.Get("/api/games/7701/prediction").JSON()
	if resp["cached"] != false {
		t.Error("a forecast from another model version was served from the cache")
	}
}
