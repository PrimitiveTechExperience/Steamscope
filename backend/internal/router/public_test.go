package router_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func TestHealthReadinessAndMetrics(t *testing.T) {
	app := testutil.NewApp(t)
	c := app.NewClient()

	t.Run("health is a cheap liveness probe", func(t *testing.T) {
		resp := c.Get("/api/health")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d", resp.Code())
		}
		body := resp.JSON()
		if body["status"] != "ok" || body["version"] == nil || body["uptime_seconds"] == nil {
			t.Errorf("health body = %v", body)
		}
	})

	t.Run("every response carries a request id", func(t *testing.T) {
		for _, path := range []string{"/api/health", "/api/games/999999", "/api/me/feed", "/nope"} {
			id := c.Get(path).Header().Get("X-Request-ID")
			if len(id) < 8 {
				t.Errorf("%s: X-Request-ID = %q", path, id)
			}
		}
		c.Headers = map[string]string{"X-Request-ID": "trace-from-client-1"}
		if got := c.Get("/api/health").Header().Get("X-Request-ID"); got != "trace-from-client-1" {
			t.Errorf("client request id not honoured: %q", got)
		}
		c.Headers = nil
	})

	t.Run("readiness reports ok with a healthy database and redis", func(t *testing.T) {
		resp := c.Get("/api/ready")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		checks := resp.JSON()["checks"].(map[string]any)
		if checks["database"] != "ok" || checks["redis"] != "ok" {
			t.Errorf("checks = %v", checks)
		}
	})

	t.Run("metrics require the bearer token", func(t *testing.T) {
		if got := c.Get("/metrics").Code(); got != http.StatusUnauthorized {
			t.Errorf("no token: status = %d, want 401", got)
		}
		c.Headers = map[string]string{"Authorization": "Bearer wrong"}
		if got := c.Get("/metrics").Code(); got != http.StatusUnauthorized {
			t.Errorf("wrong token: status = %d, want 401", got)
		}
		c.Headers = nil
	})

	t.Run("metrics expose request counts by route pattern", func(t *testing.T) {
		c.Get("/api/games/424242") // 404 on a path parameter route
		c.Headers = map[string]string{"Authorization": "Bearer test-token"}
		resp := c.Get("/metrics")
		c.Headers = nil

		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d", resp.Code())
		}
		body := resp.Body.String()
		for _, want := range []string{
			`steamscope_http_requests_total{method="GET",route="GET /api/games/{appID}",status="404"}`,
			`steamscope_http_requests_total{method="GET",route="GET /api/health",status="200"}`,
			`steamscope_http_request_duration_seconds_bucket`,
			`steamscope_http_requests_in_flight`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("metrics missing %s", want)
			}
		}
		if strings.Contains(body, "424242") {
			t.Error("the raw path leaked into a metrics label")
		}
	})

	t.Run("readiness turns 503 when redis is down", func(t *testing.T) {
		app.Mini.Close()
		resp := c.Get("/api/ready")
		if resp.Code() != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", resp.Code())
		}
		body := resp.JSON()
		checks := body["checks"].(map[string]any)
		if body["status"] != "unavailable" || checks["redis"] != "unavailable" || checks["database"] != "ok" {
			t.Errorf("body = %v", body)
		}
		if got := c.Get("/api/health").Code(); got != http.StatusOK {
			t.Errorf("liveness must stay 200 while a dependency is down, got %d", got)
		}
	})
}

func TestPublicGames(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(300001, "Alpha Quest", []string{"Alpha Devs"}, []string{"Alpha Pub"})
	app.SeedGame(300002, "Beta Racer", []string{"Beta Devs"}, []string{"Beta Pub"})
	c := app.NewClient()

	t.Run("lists games as JSON", func(t *testing.T) {
		resp := c.Get("/api/games")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d", resp.Code())
		}
		if ct := resp.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		games := resp.JSON()["games"].([]any)
		if len(games) != 2 {
			t.Errorf("got %d games, want 2", len(games))
		}
	})

	t.Run("search filters and results are cached", func(t *testing.T) {
		first := c.Get("/api/games?search=alpha")
		games := first.JSON()["games"].([]any)
		if len(games) != 1 || games[0].(map[string]any)["name"] != "Alpha Quest" {
			t.Errorf("search results = %v", games)
		}
		if first.Header().Get("X-Cache") != "MISS" {
			t.Errorf("first request X-Cache = %q, want MISS", first.Header().Get("X-Cache"))
		}
		if c.Get("/api/games?search=alpha").Header().Get("X-Cache") != "HIT" {
			t.Error("repeat request should be served from cache")
		}
	})

	t.Run("rejects bad query parameters with a JSON 400", func(t *testing.T) {
		for _, q := range []string{
			"limit=abc", "limit=0", "limit=-3", "offset=-1", "offset=x",
			"minPrice=abc", "minPrice=-1", "maxPrice=-5", "minPrice=50&maxPrice=10",
		} {
			resp := c.Get("/api/games?" + q)
			if resp.Code() != http.StatusBadRequest {
				t.Errorf("?%s: status = %d, want 400", q, resp.Code())
			} else if resp.Error() == "" {
				t.Errorf("?%s: error body is not JSON: %s", q, resp.Body.String())
			}
		}
	})

	t.Run("limit is capped rather than rejected", func(t *testing.T) {
		resp := c.Get("/api/games?limit=100000")
		if resp.Code() != http.StatusOK || resp.JSON()["limit"] != float64(100) {
			t.Errorf("status = %d limit = %v, want 200 capped at 100", resp.Code(), resp.JSON()["limit"])
		}
	})

	t.Run("search is safe against injection", func(t *testing.T) {
		for _, q := range []string{"%27%20OR%201%3D1--", "%25", "%5C", "%00"} {
			if got := c.Get("/api/games?search=" + q).Code(); got != http.StatusOK && got != http.StatusBadRequest {
				t.Errorf("search=%s: status = %d, want 200 or 400, never 500", q, got)
			}
		}
		if got := c.Get("/api/games").Code(); got != http.StatusOK {
			t.Errorf("table damaged by injection attempts: %d", got)
		}
	})

	t.Run("single game: details, sanitized HTML description, 404 and 400", func(t *testing.T) {
		resp := c.Get("/api/games/300001")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d", resp.Code())
		}
		g := resp.JSON()
		if g["name"] != "Alpha Quest" || g["description_html"] != "<p>desc</p>" {
			t.Errorf("game = %v", g)
		}
		if devs := g["developers"].([]any); len(devs) != 1 || devs[0] != "Alpha Devs" {
			t.Errorf("developers = %v", devs)
		}

		if resp := c.Get("/api/games/999999"); resp.Code() != http.StatusNotFound || resp.Error() == "" {
			t.Errorf("unknown game: status = %d body = %s, want a JSON 404", resp.Code(), resp.Body.String())
		}
		if got := c.Get("/api/games/abc").Code(); got != http.StatusBadRequest {
			t.Errorf("non-numeric id: status = %d, want 400", got)
		}
	})

	t.Run("price history and reviews of a game", func(t *testing.T) {
		if got := c.Get("/api/games/300001/price-history").Code(); got != http.StatusOK {
			t.Errorf("price history: status = %d, want 200", got)
		}
		if got := c.Get("/api/games/abc/price-history").Code(); got != http.StatusBadRequest {
			t.Errorf("price history bad id: status = %d, want 400", got)
		}
		if got := c.Get("/api/games/300001/reviews").Code(); got != http.StatusOK {
			t.Errorf("reviews: status = %d, want 200", got)
		}
		if got := c.Get("/api/games/300001/reviews?limit=abc").Code(); got != http.StatusBadRequest {
			t.Errorf("reviews bad limit: status = %d, want 400", got)
		}
	})

	t.Run("filter options", func(t *testing.T) {
		if got := c.Get("/api/filters").Code(); got != http.StatusOK {
			t.Errorf("status = %d, want 200", got)
		}
	})

	t.Run("unknown routes are 404 and wrong methods are 405", func(t *testing.T) {
		if got := c.Get("/api/does-not-exist").Code(); got != http.StatusNotFound {
			t.Errorf("unknown route: status = %d, want 404", got)
		}
		if got := c.Delete("/api/games").Code(); got != http.StatusMethodNotAllowed {
			t.Errorf("wrong method: status = %d, want 405", got)
		}
	})
}

func TestPublicBundles(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(400001, "On The Site", []string{"d"}, []string{"p"})
	app.SeedBundle(5001, "Starter Pack",
		models.BundleGame{AppID: 400001, Name: "On The Site"},
		models.BundleGame{AppID: 400002, Name: "Not On The Site"},
		models.BundleGame{AppID: 400003, Name: "Requested Game"},
	)
	app.Pool.Exec(t.Context(), `INSERT INTO tracked_games (app_id, status) VALUES (400003, 'awaiting_approval')`)
	c := app.NewClient()

	t.Run("validates query parameters", func(t *testing.T) {
		for _, q := range []string{"app_id=abc", "app_id=0", "app_id=-1", "limit=abc", "limit=0"} {
			if got := c.Get("/api/bundles?" + q).Code(); got != http.StatusBadRequest {
				t.Errorf("?%s: status = %d, want 400", q, got)
			}
		}
	})

	t.Run("lists tracked bundles with preview games for the cover collage", func(t *testing.T) {
		list := c.Get("/api/bundles").JSONArray()
		if len(list) != 1 {
			t.Fatalf("got %d bundles, want 1", len(list))
		}
		b := list[0].(map[string]any)
		if b["name"] != "Starter Pack" || b["game_count"] != float64(3) || b["discount_percentage"] != float64(50) {
			t.Errorf("bundle = %v", b)
		}
		if games := b["games"].([]any); len(games) != 3 {
			t.Errorf("preview games = %d, want 3", len(games))
		}
	})

	t.Run("filters by a contained game", func(t *testing.T) {
		if n := len(c.Get("/api/bundles?app_id=400001").JSONArray()); n != 1 {
			t.Errorf("bundles containing the game = %d, want 1", n)
		}
		if n := len(c.Get("/api/bundles?app_id=123").JSONArray()); n != 0 {
			t.Errorf("bundles containing an unrelated game = %d, want 0", n)
		}
		if body := strings.TrimSpace(c.Get("/api/bundles?app_id=123").Body.String()); body != "[]" {
			t.Errorf("empty result body = %s, want []", body)
		}
	})

	t.Run("bundle detail marks which games can be requested", func(t *testing.T) {
		resp := c.Get("/api/bundles/5001")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d", resp.Code())
		}
		byID := map[float64]map[string]any{}
		for _, g := range resp.JSON()["games"].([]any) {
			m := g.(map[string]any)
			byID[m["app_id"].(float64)] = m
		}
		if byID[400001]["tracked"] != true || byID[400001]["track_status"] != "" {
			t.Errorf("a game on the site = %v", byID[400001])
		}
		if byID[400002]["tracked"] != false || byID[400002]["track_status"] != "" {
			t.Errorf("a never-requested game = %v (should offer the Request button)", byID[400002])
		}
		if byID[400003]["tracked"] != false || byID[400003]["track_status"] != "awaiting_approval" {
			t.Errorf("an already-requested game = %v", byID[400003])
		}
		if hist := resp.JSON()["price_history"].([]any); len(hist) != 1 {
			t.Errorf("price history has %d points, want 1", len(hist))
		}
	})

	t.Run("404 for missing or untracked bundles and 400 for bad ids", func(t *testing.T) {
		if got := c.Get("/api/bundles/999999").Code(); got != http.StatusNotFound {
			t.Errorf("missing: status = %d, want 404", got)
		}
		if got := c.Get("/api/bundles/abc").Code(); got != http.StatusBadRequest {
			t.Errorf("bad id: status = %d, want 400", got)
		}
		app.Pool.Exec(t.Context(), `UPDATE bundles SET status = 'awaiting_approval' WHERE bundle_id = 5001`)
		if got := c.Get("/api/bundles/5001").Code(); got != http.StatusNotFound {
			t.Errorf("unapproved bundle must not be public: status = %d, want 404", got)
		}
		if n := len(c.Get("/api/bundles").JSONArray()); n != 0 {
			t.Errorf("unapproved bundle listed publicly (%d)", n)
		}
	})
}
