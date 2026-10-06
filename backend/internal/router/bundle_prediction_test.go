package router_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

// seedBundleWithHistory stores a tracked bundle and gives it `days` of saved
// history, as if it had been imported from ITAD.
func seedBundleWithHistory(t *testing.T, app *testutil.App, id int, days int) {
	t.Helper()
	app.SeedBundle(id, "Cyclic Bundle", models.BundleGame{AppID: id + 1, Name: "Some Game", Price: 5, RegularPrice: 10})
	if _, err := app.DB.InsertBundlePriceHistory(context.Background(), id, periodicHistory(days)); err != nil {
		t.Fatal(err)
	}
}

func TestBundlePredictionEndpoint(t *testing.T) {
	app := testutil.NewApp(t)
	seedBundleWithHistory(t, app, 7601, 730)
	c := app.NewClient()

	t.Run("rejects a bad id and an unknown bundle", func(t *testing.T) {
		for _, id := range []string{"abc", "0", "-3"} {
			if got := c.Get("/api/bundles/" + id + "/prediction").Code(); got != http.StatusBadRequest {
				t.Errorf("id %q: status = %d, want 400", id, got)
			}
		}
		if got := c.Get("/api/bundles/999999/prediction").Code(); got != http.StatusNotFound {
			t.Errorf("unknown bundle: status = %d, want 404", got)
		}
		if got := c.Get("/api/bundles/999999/advice").Code(); got != http.StatusNotFound {
			t.Errorf("advice for an unknown bundle: status = %d, want 404", got)
		}
	})

	t.Run("forecasts a bundle with the same model as a game", func(t *testing.T) {
		resp := c.Get("/api/bundles/7601/prediction")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		f := resp.JSON()
		if f["model"] != "weibull_renewal" || f["bundle_id"] != float64(7601) || f["app_id"] != float64(0) {
			t.Errorf("forecast = model %v bundle %v app %v", f["model"], f["bundle_id"], f["app_id"])
		}
		if f["current_price"] != float64(20) || f["on_sale"] != false {
			t.Errorf("price %v on_sale %v, want $20 full price", f["current_price"], f["on_sale"])
		}
		// The history alternates $20 and a 50% sale, so sales are found even though
		// the saved "original price" was not used (it is rebuilt from the prices).
		typical := f["typical_sale"].(map[string]any)
		if typical["median_depth_percent"] != float64(50) || typical["count"].(float64) < 10 {
			t.Errorf("typical sale = %v", typical)
		}
		if f["historic_low"] != float64(10) {
			t.Errorf("historic low = %v, want 10", f["historic_low"])
		}
	})

	t.Run("advice says to wait when a sale is due soon", func(t *testing.T) {
		resp := c.Get("/api/bundles/7601/advice")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		a := resp.JSON()
		if a["verdict"] != "wait" || a["personalized"] != false {
			t.Errorf("verdict %v personalized %v, want a general 'wait'", a["verdict"], a["personalized"])
		}
	})

	t.Run("is cached and refreshed when the history changes", func(t *testing.T) {
		if c.Get("/api/bundles/7601/prediction").JSON()["cached"] != true {
			t.Error("the repeat request was not cached")
		}
		next := periodicHistory(731)[730]
		next.Date = next.Date.AddDate(0, 0, 1)
		if _, err := app.DB.InsertBundlePriceHistory(context.Background(), 7601, []models.PricePoint{next}); err != nil {
			t.Fatal(err)
		}
		if c.Get("/api/bundles/7601/prediction").JSON()["cached"] != false {
			t.Error("a forecast built from older history was served")
		}
	})
}

func TestBundleAndGameForecastsDoNotCollide(t *testing.T) {
	// Steam bundle ids and app ids are separate number spaces, so the same
	// number can be both. Their forecasts must not be mixed up in the cache.
	app := testutil.NewApp(t)
	app.SeedGame(7701, "A Game", []string{"d"}, []string{"p"})
	app.SeedPriceHistory(7701, periodicHistory(730))
	seedBundleWithHistory(t, app, 7701, 400)
	c := app.NewClient()

	game := c.Get("/api/games/7701/prediction").JSON()
	bundle := c.Get("/api/bundles/7701/prediction").JSON()
	if game["app_id"] != float64(7701) || game["bundle_id"] != nil {
		t.Errorf("game forecast = app %v bundle %v", game["app_id"], game["bundle_id"])
	}
	if bundle["bundle_id"] != float64(7701) || bundle["app_id"] != float64(0) {
		t.Errorf("bundle forecast = app %v bundle %v", bundle["app_id"], bundle["bundle_id"])
	}
	if game["history_days"] == bundle["history_days"] {
		t.Errorf("both report %v days of history; one was served the other's forecast", game["history_days"])
	}
	// Both are served from the cache on a repeat, each as itself.
	if g := c.Get("/api/games/7701/prediction").JSON(); g["cached"] != true || g["bundle_id"] != nil {
		t.Errorf("repeat game forecast: cached %v bundle %v", g["cached"], g["bundle_id"])
	}
	if b := c.Get("/api/bundles/7701/prediction").JSON(); b["cached"] != true || b["bundle_id"] != float64(7701) {
		t.Errorf("repeat bundle forecast: cached %v bundle %v", b["cached"], b["bundle_id"])
	}
	keys := strings.Join(predictionKeys(app), " ")
	if !strings.Contains(keys, "prediction:7701") || !strings.Contains(keys, "prediction:-7701") {
		t.Errorf("cache keys = %s, want one for the game and one for the bundle", keys)
	}
}

func TestImportedBundleHistoryNeverReplacesScrapedDays(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedBundle(7801, "Real Bundle") // records today's scraped price: $20
	today := utcToday()
	added, err := app.DB.InsertBundlePriceHistory(context.Background(), 7801, []models.PricePoint{
		{Date: today.AddDate(0, 0, -5), Price: 30, OriginalPrice: 40},
		{Date: today.AddDate(0, 0, -4), Price: 31, OriginalPrice: 40},
	})
	if err != nil || added != 2 {
		t.Fatalf("added %d, err %v", added, err)
	}
	// Import the same days again, plus a clashing price for the day we scraped.
	scraped, err := app.DB.GetBundlePriceHistory(context.Background(), 7801)
	if err != nil {
		t.Fatal(err)
	}
	clash := scraped[len(scraped)-1]
	clash.Price = 99
	again, err := app.DB.InsertBundlePriceHistory(context.Background(), 7801, []models.PricePoint{
		{Date: today.AddDate(0, 0, -5), Price: 1}, clash,
	})
	if err != nil || again != 0 {
		t.Errorf("re-import added %d (err %v), want 0", again, err)
	}
	after, _ := app.DB.GetBundlePriceHistory(context.Background(), 7801)
	for _, p := range after {
		if p.Price == 99 || p.Price == 1 {
			t.Errorf("an imported price overwrote a stored one: %+v", p)
		}
	}
	if got := after[len(after)-1].Price; got != 20 {
		t.Errorf("today's scraped price = %v, want 20", got)
	}
}

func TestBundleDetailUsesTheFullImportedHistory(t *testing.T) {
	app := testutil.NewApp(t)
	seedBundleWithHistory(t, app, 7901, 400)
	resp := app.NewClient().Get("/api/bundles/7901")
	j := resp.JSON()
	if j["history_days"].(float64) < 400 {
		t.Errorf("history_days = %v, want the imported span", j["history_days"])
	}
	if j["record_low"] != float64(10) {
		t.Errorf("record_low = %v, want the imported low of 10", j["record_low"])
	}
}
