package router_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

const wishSteamID = "76561198000000555"

// wishlistUser registers a user who has linked a Steam account.
func wishlistUser(t *testing.T, app *testutil.App, steamID string) *testutil.Client {
	t.Helper()
	user := app.NewUser("wish")
	if _, err := app.Pool.Exec(context.Background(), "UPDATE users SET steam_id = $2 WHERE user_id = $1", user.UserID, steamID); err != nil {
		t.Fatal(err)
	}
	return user
}

func watchedCount(t *testing.T, app *testutil.App, userID int64) int {
	t.Helper()
	var n int
	if err := app.Pool.QueryRow(context.Background(), "SELECT count(*) FROM watched_games WHERE user_id = $1", userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func requestableIDs(resp map[string]any) []float64 {
	var ids []float64
	for _, g := range resp["requestable"].([]any) {
		ids = append(ids, g.(map[string]any)["app_id"].(float64))
	}
	return ids
}

func TestImportWishlistWatchesKnownGamesAndReportsTheRest(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	app.SeedGame(830001, "Known One", []string{"d"}, []string{"p"})
	app.SeedGame(830002, "Known Two", []string{"d"}, []string{"p"})
	steam.SetWishlist(wishSteamID, 830001, 830002, 830003, 830004, 830001) // 830001 listed twice
	steam.SetName(830003, "Missing Three")
	user := wishlistUser(t, app, wishSteamID)

	resp := user.Post("/api/me/wishlist/import", nil)
	if resp.Code() != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
	}
	j := resp.JSON()
	if j["wishlist_size"] != float64(4) || j["watched"] != float64(2) || j["already_watched"] != float64(0) {
		t.Errorf("size/watched/already = %v/%v/%v, want 4/2/0", j["wishlist_size"], j["watched"], j["already_watched"])
	}
	if got := watchedCount(t, app, user.UserID); got != 2 {
		t.Errorf("watching %d games, want the 2 we have", got)
	}
	if ids := requestableIDs(j); len(ids) != 2 || ids[0] != 830003 || ids[1] != 830004 {
		t.Errorf("requestable = %v, want the 2 we do not have", ids)
	}
	names := map[float64]string{}
	for _, g := range j["requestable"].([]any) {
		m := g.(map[string]any)
		names[m["app_id"].(float64)] = m["name"].(string)
	}
	if names[830003] != "Missing Three" || names[830004] != "" {
		t.Errorf("names = %v, want a name where Steam has one and blank otherwise", names)
	}

	t.Run("importing again changes nothing", func(t *testing.T) {
		j := user.Post("/api/me/wishlist/import", nil).JSON()
		if j["watched"] != float64(0) || j["already_watched"] != float64(2) {
			t.Errorf("watched/already = %v/%v, want 0/2", j["watched"], j["already_watched"])
		}
		if got := watchedCount(t, app, user.UserID); got != 2 {
			t.Errorf("watching %d games after a repeat", got)
		}
	})
}

func TestImportKeepsExistingWatchSettings(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	app.SeedGame(830101, "Pinned Already", []string{"d"}, []string{"p"})
	steam.SetWishlist(wishSteamID, 830101)
	user := wishlistUser(t, app, wishSteamID)
	if got := user.Put("/api/me/watchlist/830101", map[string]any{"pinned": true, "target_price": 4.5}).Code(); got != http.StatusNoContent {
		t.Fatalf("watch status = %d", got)
	}

	if j := user.Post("/api/me/wishlist/import", nil).JSON(); j["already_watched"] != float64(1) || j["watched"] != float64(0) {
		t.Errorf("response = %v", j)
	}
	var pinned bool
	var target *float64
	if err := app.Pool.QueryRow(context.Background(), "SELECT pinned, target_price FROM watched_games WHERE user_id = $1 AND app_id = 830101", user.UserID).Scan(&pinned, &target); err != nil {
		t.Fatal(err)
	}
	if !pinned || target == nil || *target != 4.5 {
		t.Errorf("pinned=%v target=%v, want the user's own settings kept", pinned, target)
	}
}

func TestImportClassifiesGamesWeCannotJustRequest(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	admin := app.NewAdmin()
	if _, err := app.DB.AddBlacklistRule(context.Background(), "app_id", "830203", "no", admin.UserID); err != nil {
		t.Fatal(err)
	}
	for id, status := range map[int]string{830201: "awaiting_approval", 830202: "rejected", 830204: "pending", 830205: "failed"} {
		if _, err := app.Pool.Exec(context.Background(), "INSERT INTO tracked_games (app_id, status) VALUES ($1, $2)", id, status); err != nil {
			t.Fatal(err)
		}
	}
	steam.SetWishlist(wishSteamID, 830201, 830202, 830203, 830204, 830205, 830206)
	user := wishlistUser(t, app, wishSteamID)

	j := user.Post("/api/me/wishlist/import", nil).JSON()
	if j["awaiting_review"] != float64(2) { // awaiting_approval + pending
		t.Errorf("awaiting_review = %v, want 2", j["awaiting_review"])
	}
	if j["unavailable"] != float64(2) { // rejected + blacklisted
		t.Errorf("unavailable = %v, want 2", j["unavailable"])
	}
	// A game whose earlier attempt failed can be tried again, as can a new one.
	if ids := requestableIDs(j); len(ids) != 2 || ids[0] != 830205 || ids[1] != 830206 {
		t.Errorf("requestable = %v, want 830205 and 830206", ids)
	}
}

func TestImportWishlistErrors(t *testing.T) {
	app := testutil.NewApp(t)

	t.Run("needs a signed-in user", func(t *testing.T) {
		if got := app.NewClient().Post("/api/me/wishlist/import", nil).Code(); got != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
		if got := app.NewClient().Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{1}}).Code(); got != http.StatusUnauthorized {
			t.Errorf("request status = %d, want 401", got)
		}
	})

	t.Run("needs a linked Steam account", func(t *testing.T) {
		app.FakeSteam()
		resp := app.NewUser("nolink").Post("/api/me/wishlist/import", nil)
		if resp.Code() != http.StatusBadRequest || resp.Error() == "" {
			t.Errorf("status = %d body = %s, want a JSON 400", resp.Code(), resp.Body.String())
		}
	})

	t.Run("an empty or private wishlist is not an error", func(t *testing.T) {
		app.FakeSteam()
		resp := wishlistUser(t, app, "76561198000000600").Post("/api/me/wishlist/import", nil)
		if resp.Code() != http.StatusOK || resp.JSON()["wishlist_size"] != float64(0) || len(resp.JSON()["requestable"].([]any)) != 0 {
			t.Errorf("status = %d body = %s", resp.Code(), resp.Body.String())
		}
	})

	t.Run("a Steam outage is a 502 and nothing is watched", func(t *testing.T) {
		steam := app.FakeSteam()
		steam.Fail = true
		user := wishlistUser(t, app, "76561198000000601")
		if got := user.Post("/api/me/wishlist/import", nil).Code(); got != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", got)
		}
	})

	t.Run("without a Steam key it says it is not set up", func(t *testing.T) {
		// The default API client has no key.
		fresh := testutil.NewApp(t)
		user := wishlistUser(t, fresh, wishSteamID)
		if got := user.Post("/api/me/wishlist/import", nil).Code(); got != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", got)
		}
	})

	t.Run("importing is rate limited per user", func(t *testing.T) {
		steam := app.FakeSteam()
		user := wishlistUser(t, app, "76561198000000602")
		steam.SetWishlist("76561198000000602")
		var last int
		for i := 0; i < 7; i++ {
			last = user.Post("/api/me/wishlist/import", nil).Code()
		}
		if last != http.StatusTooManyRequests {
			t.Errorf("7th import: status = %d, want 429", last)
		}
		if steam.Calls > 6 {
			t.Errorf("Steam was called %d times; the limit should stop calls before they reach it", steam.Calls)
		}
	})
}

func TestRequestWishlistGames(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	app.SeedGame(830301, "Have It", []string{"d"}, []string{"p"})
	steam.SetWishlist(wishSteamID, 830301, 830302, 830303)
	user := wishlistUser(t, app, wishSteamID)

	status := func(id int) string {
		var s string
		if err := app.Pool.QueryRow(context.Background(), "SELECT status FROM tracked_games WHERE app_id = $1", id).Scan(&s); err != nil {
			return ""
		}
		return s
	}

	t.Run("submits the chosen wishlist games for approval", func(t *testing.T) {
		resp := user.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{830302, 830303}})
		if resp.Code() != http.StatusAccepted {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		if resp.JSON()["requested"] != float64(2) || resp.JSON()["status"] != "awaiting_approval" {
			t.Errorf("response = %v", resp.JSON())
		}
		if status(830302) != "awaiting_approval" || status(830303) != "awaiting_approval" {
			t.Errorf("statuses = %q %q, want both awaiting approval", status(830302), status(830303))
		}
		if app.Queue.Len() != 0 {
			t.Error("a user's request must wait for an admin before anything is scraped")
		}
	})

	t.Run("asking again is harmless", func(t *testing.T) {
		resp := user.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{830302}})
		if resp.Code() != http.StatusAccepted || resp.JSON()["requested"] != float64(0) {
			t.Errorf("status = %d body = %s, want nothing new requested", resp.Code(), resp.Body.String())
		}
	})

	t.Run("they then show up for the admin to review", func(t *testing.T) {
		items := app.NewAdmin().Get("/api/admin/items").JSONArray()
		n := 0
		for _, it := range items {
			m := it.(map[string]any)
			if m["status"] == "awaiting_approval" && (m["id"] == float64(830302) || m["id"] == float64(830303)) {
				n++
			}
		}
		if n != 2 {
			t.Errorf("admin sees %d of the 2 requests awaiting approval", n)
		}
	})
}

func TestRequestOnlyAcceptsGamesActuallyOnTheWishlist(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	app.SeedGame(830401, "Already Here", []string{"d"}, []string{"p"})
	steam.SetWishlist(wishSteamID, 830401, 830402)
	user := wishlistUser(t, app, wishSteamID)

	resp := user.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{830402, 999999, -4, 0, 830401, 830402}})
	if resp.Code() != http.StatusAccepted || resp.JSON()["requested"] != float64(1) {
		t.Fatalf("status = %d body = %s, want just the one real wishlist game requested", resp.Code(), resp.Body.String())
	}
	var n int
	app.Pool.QueryRow(context.Background(), "SELECT count(*) FROM tracked_games WHERE app_id IN (999999, 830401)").Scan(&n)
	if n != 1 { // 830401 was seeded as tracked; 999999 must not have been added
		t.Errorf("tracked_games rows for the unwanted ids = %d, want only the seeded one", n)
	}
}

func TestRequestValidationAndLimits(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	steam.SetWishlist(wishSteamID, 830501)
	user := wishlistUser(t, app, wishSteamID)

	t.Run("rejects an empty or oversized list", func(t *testing.T) {
		if got := user.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{}}).Code(); got != http.StatusBadRequest {
			t.Errorf("empty: status = %d, want 400", got)
		}
		big := make([]int, 101)
		for i := range big {
			big[i] = 840000 + i
		}
		if got := user.Post("/api/me/wishlist/request", map[string]any{"app_ids": big}).Code(); got != http.StatusBadRequest {
			t.Errorf("101 ids: status = %d, want 400", got)
		}
		if got := user.Post("/api/me/wishlist/request", map[string]any{"app_ids": []string{"x"}}).Code(); got != http.StatusBadRequest {
			t.Errorf("wrong type: status = %d, want 400", got)
		}
	})

	t.Run("blocked submitters cannot request", func(t *testing.T) {
		blocked := wishlistUser(t, app, "76561198000000556")
		app.SetFlag(blocked.UserID, "submissions_blocked", true)
		if got := blocked.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{830501}}).Code(); got != http.StatusForbidden {
			t.Errorf("status = %d, want 403", got)
		}
	})

	t.Run("requests are rate limited", func(t *testing.T) {
		var last int
		for i := 0; i < 4; i++ {
			last = user.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{830501}}).Code()
		}
		if last != http.StatusTooManyRequests {
			t.Errorf("4th request: status = %d, want 429", last)
		}
	})
}

func TestAdminWishlistRequestsAreQueuedForScraping(t *testing.T) {
	app := testutil.NewApp(t)
	steam := app.FakeSteam()
	admin := app.NewAdmin()
	if _, err := app.Pool.Exec(context.Background(), "UPDATE users SET steam_id = $2 WHERE user_id = $1", admin.UserID, wishSteamID); err != nil {
		t.Fatal(err)
	}
	steam.SetWishlist(wishSteamID, 830601, 830602)

	resp := admin.Post("/api/me/wishlist/request", map[string]any{"app_ids": []int{830601, 830602}})
	if resp.Code() != http.StatusAccepted || resp.JSON()["status"] != "pending" || resp.JSON()["requested"] != float64(2) {
		t.Fatalf("status = %d body = %s", resp.Code(), resp.Body.String())
	}
	if got := app.Queue.Len(); got != 2 {
		t.Errorf("queued %d scrape jobs, want 2 (admins skip approval)", got)
	}
}
