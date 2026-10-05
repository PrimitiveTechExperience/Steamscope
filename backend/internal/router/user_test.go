package router_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func TestProtectedRoutesRequireLogin(t *testing.T) {
	app := testutil.NewApp(t)
	anon := app.NewClient()
	routes := []struct{ method, path string }{
		{"GET", "/api/me/preferences"},
		{"PUT", "/api/me/preferences"},
		{"GET", "/api/me/watchlist"},
		{"PUT", "/api/me/watchlist/730"},
		{"DELETE", "/api/me/watchlist/730"},
		{"GET", "/api/me/notifications"},
		{"POST", "/api/me/notifications/read"},
		{"GET", "/api/me/recent-searches"},
		{"POST", "/api/me/recent-searches"},
		{"GET", "/api/me/steam-profile"},
		{"DELETE", "/api/me/steam"},
		{"GET", "/api/me/feed"},
		{"GET", "/api/me/submissions"},
		{"POST", "/api/submissions"},
	}
	for _, r := range routes {
		resp := anon.Do(r.method, r.path, map[string]any{})
		if resp.Code() != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", r.method, r.path, resp.Code())
		} else if resp.Error() != "not logged in" {
			t.Errorf("%s %s: error = %q", r.method, r.path, resp.Error())
		}
	}
}

func TestPreferences(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("prefs")

	t.Run("new accounts get sensible defaults", func(t *testing.T) {
		resp := user.Get("/api/me/preferences")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d", resp.Code())
		}
		p := resp.JSON()
		if p["theme"] != "dark" || p["notify_price_drops"] != true || p["price_drop_threshold_percent"] != float64(10) {
			t.Errorf("defaults = %v", p)
		}
	})

	t.Run("rejects invalid values with 400", func(t *testing.T) {
		ok := func(over map[string]any) map[string]any {
			base := map[string]any{"theme": "dark", "notify_price_drops": true, "price_drop_threshold_percent": 10, "preferred_genres": []string{}}
			for k, v := range over {
				base[k] = v
			}
			return base
		}
		cases := map[string]any{
			"unknown theme":       ok(map[string]any{"theme": "neon"}),
			"threshold zero":      ok(map[string]any{"price_drop_threshold_percent": 0}),
			"threshold negative":  ok(map[string]any{"price_drop_threshold_percent": -5}),
			"threshold over 100":  ok(map[string]any{"price_drop_threshold_percent": 101}),
			"too many genres":     ok(map[string]any{"preferred_genres": strings.Split(strings.Repeat("g,", 21), ",")}),
			"genre only controls": ok(map[string]any{"preferred_genres": []string{"\x00\x01"}}),
			"unknown field":       `{"theme":"dark","is_admin":true}`,
			"malformed":           "{",
		}
		for name, body := range cases {
			if got := user.Put("/api/me/preferences", body).Code(); got != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", name, got)
			}
		}
	})

	t.Run("saves valid values and cleans genre names", func(t *testing.T) {
		resp := user.Put("/api/me/preferences", map[string]any{
			"theme": "light", "notify_price_drops": false, "price_drop_threshold_percent": 25,
			"preferred_genres": []string{"  Action​  ", "RPG"},
		})
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		got := user.Get("/api/me/preferences").JSON()
		if got["theme"] != "light" || got["notify_price_drops"] != false || got["price_drop_threshold_percent"] != float64(25) {
			t.Errorf("not persisted: %v", got)
		}
		genres := got["preferred_genres"].([]any)
		if len(genres) != 2 || genres[0] != "Action" {
			t.Errorf("genres = %v, want the first cleaned to \"Action\"", genres)
		}
	})

	t.Run("preferences are per user", func(t *testing.T) {
		other := app.NewUser("prefs2")
		if other.Get("/api/me/preferences").JSON()["theme"] != "dark" {
			t.Error("another user's preferences were changed")
		}
	})
}

func TestWatchlist(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(100001, "Watchable", []string{"d"}, []string{"p"})
	user := app.NewUser("watcher")

	t.Run("validates the request", func(t *testing.T) {
		cases := []struct {
			name, path string
			body       any
			want       int
		}{
			{"non-numeric id", "/api/me/watchlist/abc", map[string]any{}, http.StatusBadRequest},
			{"unknown game", "/api/me/watchlist/999999", map[string]any{}, http.StatusNotFound},
			{"negative target", "/api/me/watchlist/100001", map[string]any{"target_price": -1}, http.StatusBadRequest},
			{"absurd target", "/api/me/watchlist/100001", map[string]any{"target_price": 10001}, http.StatusBadRequest},
			{"unknown field", "/api/me/watchlist/100001", `{"pinned":true,"owner":1}`, http.StatusBadRequest},
			{"malformed", "/api/me/watchlist/100001", "{", http.StatusBadRequest},
		}
		for _, tc := range cases {
			if got := user.Put(tc.path, tc.body).Code(); got != tc.want {
				t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
			}
		}
	})

	t.Run("watch, pin, set a target, then unwatch", func(t *testing.T) {
		if got := user.Put("/api/me/watchlist/100001", map[string]any{"pinned": true, "target_price": 5.5}).Code(); got != http.StatusNoContent {
			t.Fatalf("watch: status = %d, want 204", got)
		}
		list := user.Get("/api/me/watchlist").JSONArray()
		if len(list) != 1 {
			t.Fatalf("watchlist has %d entries, want 1", len(list))
		}
		entry := list[0].(map[string]any)
		if entry["pinned"] != true || entry["target_price"] != 5.5 || entry["game"].(map[string]any)["name"] != "Watchable" {
			t.Errorf("entry = %v", entry)
		}

		// Watching again updates in place rather than duplicating.
		user.Put("/api/me/watchlist/100001", map[string]any{"pinned": false})
		if n := len(user.Get("/api/me/watchlist").JSONArray()); n != 1 {
			t.Errorf("re-watching left %d entries, want 1", n)
		}

		if got := user.Delete("/api/me/watchlist/100001").Code(); got != http.StatusNoContent {
			t.Errorf("unwatch: status = %d, want 204", got)
		}
		if n := len(user.Get("/api/me/watchlist").JSONArray()); n != 0 {
			t.Errorf("watchlist still has %d entries", n)
		}
		if got := user.Delete("/api/me/watchlist/100001").Code(); got != http.StatusNoContent {
			t.Errorf("unwatching twice: status = %d, want 204 (idempotent)", got)
		}
		if got := user.Delete("/api/me/watchlist/abc").Code(); got != http.StatusBadRequest {
			t.Errorf("unwatch bad id: status = %d, want 400", got)
		}
	})

	t.Run("watchlists are private to each user", func(t *testing.T) {
		a, b := app.NewUser("wa"), app.NewUser("wb")
		a.Put("/api/me/watchlist/100001", map[string]any{})
		if n := len(b.Get("/api/me/watchlist").JSONArray()); n != 0 {
			t.Errorf("user b sees %d of user a's watched games", n)
		}
	})

	t.Run("an empty watchlist is [] not null", func(t *testing.T) {
		fresh := app.NewUser("emptywl")
		if body := strings.TrimSpace(fresh.Get("/api/me/watchlist").Body.String()); body != "[]" {
			t.Errorf("body = %s, want []", body)
		}
	})
}

func TestNotificationsAndRecentSearches(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("notif")

	t.Run("notifications start empty as [] and mark-read validates its body", func(t *testing.T) {
		if body := strings.TrimSpace(user.Get("/api/me/notifications").Body.String()); body != "[]" {
			t.Errorf("body = %s, want []", body)
		}
		if got := user.Post("/api/me/notifications/read", "{").Code(); got != http.StatusBadRequest {
			t.Errorf("malformed: status = %d, want 400", got)
		}
		if got := user.Post("/api/me/notifications/read", `{"ids":["x"]}`).Code(); got != http.StatusBadRequest {
			t.Errorf("wrong id type: status = %d, want 400", got)
		}
		if got := user.Post("/api/me/notifications/read", map[string]any{"ids": []int{}}).Code(); got != http.StatusNoContent {
			t.Errorf("empty ids: status = %d, want 204", got)
		}
	})

	t.Run("you can only mark your own notifications read", func(t *testing.T) {
		victim := app.NewUser("victim")
		app.Pool.Exec(t.Context(), `INSERT INTO notifications (user_id, kind, message) VALUES ($1, 'price_drop', 'hello')`, victim.UserID)
		list := victim.Get("/api/me/notifications").JSONArray()
		id := list[0].(map[string]any)["notification_id"]

		attacker := app.NewUser("attacker")
		attacker.Post("/api/me/notifications/read", map[string]any{"ids": []any{id}})
		if victim.Get("/api/me/notifications").JSONArray()[0].(map[string]any)["read_at"] != nil {
			t.Error("another user marked someone else's notification read")
		}
		victim.Post("/api/me/notifications/read", map[string]any{"ids": []any{id}})
		if victim.Get("/api/me/notifications").JSONArray()[0].(map[string]any)["read_at"] == nil {
			t.Error("the owner could not mark their notification read")
		}
	})

	t.Run("recent searches ignore empty ones and clean the rest", func(t *testing.T) {
		if got := user.Post("/api/me/recent-searches", map[string]any{"search": "   "}).Code(); got != http.StatusNoContent {
			t.Errorf("empty search: status = %d, want 204", got)
		}
		if n := len(user.Get("/api/me/recent-searches").JSONArray()); n != 0 {
			t.Errorf("an empty search was saved (%d entries)", n)
		}

		user.Post("/api/me/recent-searches", map[string]any{"search": "  half​   life\x00 ", "genres": []string{"Action", "\x00"}})
		saved := user.Get("/api/me/recent-searches").JSONArray()
		if len(saved) != 1 {
			t.Fatalf("saved %d searches, want 1", len(saved))
		}
		s := saved[0].(map[string]any)
		if s["search"] != "half life" {
			t.Errorf("search = %q, want \"half life\"", s["search"])
		}
		if g := s["genres"].([]any); len(g) != 1 || g[0] != "Action" {
			t.Errorf("genres = %v, want [Action]", g)
		}

		if got := user.Post("/api/me/recent-searches", `{"bogus":1}`).Code(); got != http.StatusBadRequest {
			t.Errorf("unknown field: status = %d, want 400", got)
		}
	})
}
