package router_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func TestAdminAccessControl(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("plain")
	admin := app.NewAdmin()

	endpoints := []struct{ method, path string }{
		{"GET", "/api/admin/users"},
		{"DELETE", "/api/admin/users/987654"},
		{"PUT", "/api/admin/users/987654/moderation"},
		{"GET", "/api/admin/stats"},
		{"GET", "/api/admin/blacklist"},
		{"POST", "/api/admin/blacklist"},
		{"DELETE", "/api/admin/blacklist/987654"},
		{"GET", "/api/admin/blacklist/987654/matches"},
		{"POST", "/api/admin/blacklist/987654/purge"},
		{"GET", "/api/admin/items"},
		{"DELETE", "/api/admin/items/app/987654"},
		{"POST", "/api/admin/items/app/987654/approve"},
		{"POST", "/api/admin/items/app/987654/reject"},
	}
	for _, e := range endpoints {
		name := e.method + " " + e.path
		t.Run(name, func(t *testing.T) {
			anon := app.NewClient().Do(e.method, e.path, map[string]any{})
			if anon.Code() != http.StatusUnauthorized {
				t.Errorf("anonymous: status = %d, want 401", anon.Code())
			}
			plain := user.Do(e.method, e.path, map[string]any{})
			if plain.Code() != http.StatusForbidden {
				t.Errorf("regular user: status = %d, want 403", plain.Code())
			}
			if admin.Do(e.method, e.path, map[string]any{}).Code() == http.StatusForbidden {
				t.Error("an admin must not be forbidden")
			}
		})
	}

	t.Run("revoking admin takes effect on the existing session", func(t *testing.T) {
		temp := app.NewAdmin()
		if temp.Get("/api/admin/stats").Code() != http.StatusOK {
			t.Fatal("admin should reach /api/admin/stats")
		}
		app.SetFlag(temp.UserID, "is_admin", false)
		if got := temp.Get("/api/admin/stats").Code(); got != http.StatusForbidden {
			t.Errorf("after demotion: status = %d, want 403", got)
		}
	})
}

func TestAdminUsers(t *testing.T) {
	app := testutil.NewApp(t)
	admin := app.NewAdmin()

	t.Run("list includes flags and no password hashes", func(t *testing.T) {
		app.NewUser("listed")
		resp := admin.Get("/api/admin/users")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d", resp.Code())
		}
		users := resp.JSONArray()
		if len(users) < 2 {
			t.Fatalf("got %d users, want at least 2", len(users))
		}
		first := users[0].(map[string]any)
		for _, key := range []string{"user_id", "username", "email", "is_admin", "is_banned", "submissions_blocked", "created_at"} {
			if _, ok := first[key]; !ok {
				t.Errorf("user payload missing %q", key)
			}
		}
		if strings.Contains(strings.ToLower(resp.Body.String()), "password") {
			t.Error("admin user list leaks password data")
		}
	})

	t.Run("delete validates the id and protects admins", func(t *testing.T) {
		victim := app.NewUser("victim")
		other := app.NewAdmin()
		cases := []struct {
			name, path string
			want       int
		}{
			{"non-numeric id", "/api/admin/users/abc", http.StatusBadRequest},
			{"zero id", "/api/admin/users/0", http.StatusBadRequest},
			{"negative id", "/api/admin/users/-4", http.StatusBadRequest},
			{"yourself", fmt.Sprintf("/api/admin/users/%d", admin.UserID), http.StatusBadRequest},
			{"another admin", fmt.Sprintf("/api/admin/users/%d", other.UserID), http.StatusNotFound},
			{"nonexistent user", "/api/admin/users/99999999", http.StatusNotFound},
		}
		for _, tc := range cases {
			if got := admin.Delete(tc.path).Code(); got != tc.want {
				t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
			}
		}
		if got := admin.Delete(fmt.Sprintf("/api/admin/users/%d", victim.UserID)).Code(); got != http.StatusNoContent {
			t.Fatalf("delete user: status = %d, want 204", got)
		}
		if got := victim.Get("/api/me/preferences").Code(); got != http.StatusUnauthorized {
			t.Errorf("deleted user's session: status = %d, want 401", got)
		}
		if got := admin.Delete(fmt.Sprintf("/api/admin/users/%d", victim.UserID)).Code(); got != http.StatusNotFound {
			t.Errorf("deleting twice: status = %d, want 404", got)
		}
	})

	t.Run("deleting a user keeps the games they submitted", func(t *testing.T) {
		submitter := app.NewUser("submitter")
		submitter.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/555001"})
		admin.Delete(fmt.Sprintf("/api/admin/users/%d", submitter.UserID))

		var n int
		app.Pool.QueryRow(t.Context(), "SELECT count(*) FROM tracked_games WHERE app_id = 555001").Scan(&n)
		if n != 1 {
			t.Errorf("submission row count = %d, want 1 (submitted_by should be nulled, not cascaded)", n)
		}
	})

	t.Run("moderation validates input", func(t *testing.T) {
		target := app.NewUser("modtarget")
		other := app.NewAdmin()
		cases := []struct {
			name, path string
			body       any
			want       int
		}{
			{"bad id", "/api/admin/users/abc/moderation", map[string]bool{"is_banned": true}, http.StatusBadRequest},
			{"nothing to change", fmt.Sprintf("/api/admin/users/%d/moderation", target.UserID), map[string]any{}, http.StatusBadRequest},
			{"unknown field", fmt.Sprintf("/api/admin/users/%d/moderation", target.UserID), `{"is_admin":true}`, http.StatusBadRequest},
			{"malformed json", fmt.Sprintf("/api/admin/users/%d/moderation", target.UserID), "{oops", http.StatusBadRequest},
			{"unknown user", "/api/admin/users/99999999/moderation", map[string]bool{"is_banned": true}, http.StatusNotFound},
			{"another admin", fmt.Sprintf("/api/admin/users/%d/moderation", other.UserID), map[string]bool{"is_banned": true}, http.StatusNotFound},
			{"yourself", fmt.Sprintf("/api/admin/users/%d/moderation", admin.UserID), map[string]bool{"is_banned": true}, http.StatusNotFound},
		}
		for _, tc := range cases {
			if got := admin.Put(tc.path, tc.body).Code(); got != tc.want {
				t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
			}
		}
	})

	t.Run("ban, unban and block submissions", func(t *testing.T) {
		target := app.NewUser("banhammer")
		var login string
		app.Pool.QueryRow(t.Context(), "SELECT username FROM users WHERE user_id = $1", target.UserID).Scan(&login)
		path := fmt.Sprintf("/api/admin/users/%d/moderation", target.UserID)

		// Blocking submissions leaves the account working.
		if got := admin.Put(path, map[string]bool{"submissions_blocked": true}).Code(); got != http.StatusNoContent {
			t.Fatalf("block submissions: status = %d, want 204", got)
		}
		if got := target.Get("/api/me/preferences").Code(); got != http.StatusOK {
			t.Errorf("a submission-blocked user should still use the site: status = %d", got)
		}
		blocked := target.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/555002"})
		if blocked.Code() != http.StatusForbidden {
			t.Errorf("blocked submit: status = %d, want 403", blocked.Code())
		}
		admin.Put(path, map[string]bool{"submissions_blocked": false})
		if got := target.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/555002"}).Code(); got != http.StatusAccepted {
			t.Errorf("after unblocking: status = %d, want 202", got)
		}

		// Banning logs them out everywhere and blocks sign-in.
		if got := admin.Put(path, map[string]bool{"is_banned": true}).Code(); got != http.StatusNoContent {
			t.Fatalf("ban: status = %d, want 204", got)
		}
		if got := target.Get("/api/me/preferences").Code(); got != http.StatusUnauthorized {
			t.Errorf("banned user's session: status = %d, want 401", got)
		}
		loginBody := map[string]string{"login": login, "password": "correct-horse-battery"}
		if got := app.NewClient().Post("/api/auth/login", loginBody).Code(); got != http.StatusForbidden {
			t.Errorf("banned login: status = %d, want 403", got)
		}
		admin.Put(path, map[string]bool{"is_banned": false})
		if got := app.NewClient().Post("/api/auth/login", loginBody).Code(); got != http.StatusOK {
			t.Errorf("after unban: login status = %d, want 200", got)
		}
	})
}

func TestAdminItemsAndApprovals(t *testing.T) {
	app := testutil.NewApp(t)
	admin := app.NewAdmin()
	user := app.NewUser("suggester")

	t.Run("approve and reject validate their path", func(t *testing.T) {
		cases := []struct {
			name, method, path string
			want               int
		}{
			{"unknown kind", "POST", "/api/admin/items/movie/5/approve", http.StatusBadRequest},
			{"non-numeric id", "POST", "/api/admin/items/app/abc/approve", http.StatusBadRequest},
			{"zero id", "POST", "/api/admin/items/app/0/reject", http.StatusBadRequest},
			{"not awaiting approval", "POST", "/api/admin/items/app/424242/approve", http.StatusNotFound},
			{"reject not awaiting", "POST", "/api/admin/items/bundle/424242/reject", http.StatusNotFound},
			{"delete unknown kind", "DELETE", "/api/admin/items/movie/5", http.StatusBadRequest},
			{"delete nonexistent", "DELETE", "/api/admin/items/app/424242", http.StatusNotFound},
		}
		for _, tc := range cases {
			if got := admin.Do(tc.method, tc.path, map[string]any{}).Code(); got != tc.want {
				t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
			}
		}
	})

	t.Run("approving moves a submission into the scrape queue once", func(t *testing.T) {
		user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/777001"})

		var pending []any
		for _, it := range admin.Get("/api/admin/items").JSONArray() {
			if m := it.(map[string]any); m["status"] == "awaiting_approval" && m["id"] == float64(777001) {
				pending = append(pending, m)
			}
		}
		if len(pending) != 1 {
			t.Fatalf("admin items should list the awaiting submission once, got %d", len(pending))
		}
		if pending[0].(map[string]any)["submitted_by"] == nil {
			t.Error("the admin list should say who submitted it")
		}

		if got := admin.Post("/api/admin/items/app/777001/approve", nil).Code(); got != http.StatusNoContent {
			t.Fatalf("approve: status = %d, want 204", got)
		}
		var status string
		app.Pool.QueryRow(t.Context(), "SELECT status FROM tracked_games WHERE app_id = 777001").Scan(&status)
		if status != "pending" {
			t.Errorf("status after approval = %q, want pending (queued for scraping)", status)
		}
		if got := admin.Post("/api/admin/items/app/777001/approve", nil).Code(); got != http.StatusNotFound {
			t.Errorf("approving twice: status = %d, want 404", got)
		}
	})

	t.Run("rejecting notifies the submitter", func(t *testing.T) {
		user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/777002"})
		if got := admin.Post("/api/admin/items/app/777002/reject", nil).Code(); got != http.StatusNoContent {
			t.Fatalf("reject: status = %d, want 204", got)
		}
		found := false
		for _, n := range user.Get("/api/me/notifications").JSONArray() {
			if n.(map[string]any)["kind"] == "submission_rejected" {
				found = true
			}
		}
		if !found {
			t.Error("the submitter received no submission_rejected notification")
		}
		if got := admin.Post("/api/admin/items/app/777002/reject", nil).Code(); got != http.StatusNotFound {
			t.Errorf("rejecting twice: status = %d, want 404", got)
		}
	})

	t.Run("approves and rejects bundles too", func(t *testing.T) {
		user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/bundle/8101"})
		if got := admin.Post("/api/admin/items/bundle/8101/approve", nil).Code(); got != http.StatusNoContent {
			t.Errorf("approve bundle: status = %d, want 204", got)
		}
		user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/bundle/8102"})
		if got := admin.Post("/api/admin/items/bundle/8102/reject", nil).Code(); got != http.StatusNoContent {
			t.Errorf("reject bundle: status = %d, want 204", got)
		}
	})

	t.Run("deleting a game removes it from the public API", func(t *testing.T) {
		app.SeedGame(880001, "Doomed Game", []string{"Dev"}, []string{"Pub"})
		if got := app.NewClient().Get("/api/games/880001").Code(); got != http.StatusOK {
			t.Fatalf("seeded game: status = %d", got)
		}
		if got := admin.Delete("/api/admin/items/app/880001").Code(); got != http.StatusNoContent {
			t.Fatalf("delete: status = %d, want 204", got)
		}
		if got := app.NewClient().Get("/api/games/880001").Code(); got != http.StatusNotFound {
			t.Errorf("deleted game: status = %d, want 404", got)
		}
	})

	t.Run("deleting a bundle removes it from the public API", func(t *testing.T) {
		app.SeedBundle(990001, "Doomed Bundle", models.BundleGame{AppID: 1, Name: "x"})
		if got := admin.Delete("/api/admin/items/bundle/990001").Code(); got != http.StatusNoContent {
			t.Fatalf("delete: status = %d, want 204", got)
		}
		if got := app.NewClient().Get("/api/bundles/990001").Code(); got != http.StatusNotFound {
			t.Errorf("deleted bundle: status = %d, want 404", got)
		}
	})
}

func TestAdminBlacklist(t *testing.T) {
	app := testutil.NewApp(t)
	admin := app.NewAdmin()

	t.Run("rejects invalid rules with 400", func(t *testing.T) {
		cases := []struct {
			name string
			body any
		}{
			{"unknown field type", map[string]string{"field": "price", "pattern": "x"}},
			{"empty pattern", map[string]string{"field": "name", "pattern": ""}},
			{"whitespace pattern", map[string]string{"field": "name", "pattern": "   "}},
			{"invalid regex", map[string]string{"field": "developer", "pattern": "("}},
			{"non-numeric app id", map[string]string{"field": "app_id", "pattern": "abc"}},
			{"negative app id", map[string]string{"field": "app_id", "pattern": "-5"}},
			{"pattern too long", map[string]string{"field": "name", "pattern": strings.Repeat("a", 201)}},
			{"unknown json field", `{"field":"name","pattern":"x","admin":true}`},
			{"malformed json", "{"},
		}
		for _, tc := range cases {
			resp := admin.Post("/api/admin/blacklist", tc.body)
			if resp.Code() != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400 (%s)", tc.name, resp.Code(), resp.Body.String())
			}
		}
	})

	t.Run("a catastrophic-backtracking pattern cannot hang the server", func(t *testing.T) {
		resp := admin.Post("/api/admin/blacklist", map[string]string{"field": "name", "pattern": "(a+)+$"})
		if resp.Code() != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (RE2 accepts it and runs it in linear time)", resp.Code())
		}
		app.SeedGame(880010, strings.Repeat("a", 5000)+"!", nil, nil)
		id := int(resp.JSON()["rule_id"].(float64))
		if got := admin.Get(fmt.Sprintf("/api/admin/blacklist/%d/matches", id)).Code(); got != http.StatusOK {
			t.Errorf("matches: status = %d, want 200", got)
		}
		admin.Delete(fmt.Sprintf("/api/admin/blacklist/%d", id))
	})

	t.Run("creates, lists, previews, purges and deletes a rule", func(t *testing.T) {
		app.SeedGame(880020, "Shady Shooter", []string{"Shady Games Ltd"}, []string{"Shady Pub"})
		app.SeedGame(880021, "Honest Platformer", []string{"Honest Devs"}, []string{"Honest Pub"})

		resp := admin.Post("/api/admin/blacklist", map[string]string{"field": "developer", "pattern": "^shady games", "note": "  spam studio  "})
		if resp.Code() != http.StatusCreated {
			t.Fatalf("create: status = %d, want 201: %s", resp.Code(), resp.Body.String())
		}
		rule := resp.JSON()
		id := int(rule["rule_id"].(float64))
		if rule["field"] != "developer" || rule["pattern"] != "^shady games" || rule["note"] != "spam studio" {
			t.Errorf("unexpected rule payload (note should be cleaned): %v", rule)
		}
		if rule["created_by"] == nil {
			t.Error("rule should record which admin created it")
		}

		if dup := admin.Post("/api/admin/blacklist", map[string]string{"field": "developer", "pattern": "^shady games"}); dup.Code() != http.StatusConflict {
			t.Errorf("duplicate rule: status = %d, want 409", dup.Code())
		}
		if len(admin.Get("/api/admin/blacklist").JSONArray()) == 0 {
			t.Error("rule missing from the list")
		}

		matches := admin.Get(fmt.Sprintf("/api/admin/blacklist/%d/matches", id)).JSONArray()
		if len(matches) != 1 || matches[0].(map[string]any)["app_id"] != float64(880020) {
			t.Fatalf("preview should match only the shady game, got %v", matches)
		}

		purge := admin.Post(fmt.Sprintf("/api/admin/blacklist/%d/purge", id), nil)
		if purge.Code() != http.StatusOK || purge.JSON()["removed"] != float64(1) {
			t.Fatalf("purge: status = %d body = %s", purge.Code(), purge.Body.String())
		}
		if got := app.NewClient().Get("/api/games/880020").Code(); got != http.StatusNotFound {
			t.Errorf("purged game: status = %d, want 404", got)
		}
		if got := app.NewClient().Get("/api/games/880021").Code(); got != http.StatusOK {
			t.Errorf("unrelated game was removed: status = %d", got)
		}
		var status string
		app.Pool.QueryRow(t.Context(), "SELECT status FROM tracked_games WHERE app_id = 880020").Scan(&status)
		if status != "rejected" {
			t.Errorf("purged game's tracking status = %q, want rejected (so it isn't scraped again)", status)
		}

		if got := admin.Delete(fmt.Sprintf("/api/admin/blacklist/%d", id)).Code(); got != http.StatusNoContent {
			t.Errorf("delete rule: status = %d, want 204", got)
		}
		if got := admin.Delete(fmt.Sprintf("/api/admin/blacklist/%d", id)).Code(); got != http.StatusNotFound {
			t.Errorf("delete twice: status = %d, want 404", got)
		}
	})

	t.Run("rule id handling", func(t *testing.T) {
		if got := admin.Delete("/api/admin/blacklist/abc").Code(); got != http.StatusBadRequest {
			t.Errorf("non-numeric: status = %d, want 400", got)
		}
		if got := admin.Get("/api/admin/blacklist/abc/matches").Code(); got != http.StatusBadRequest {
			t.Errorf("matches non-numeric: status = %d, want 400", got)
		}
		if got := admin.Get("/api/admin/blacklist/99999999/matches").Code(); got != http.StatusNotFound {
			t.Errorf("matches missing: status = %d, want 404", got)
		}
		if got := admin.Post("/api/admin/blacklist/99999999/purge", nil).Code(); got != http.StatusNotFound {
			t.Errorf("purge missing: status = %d, want 404", got)
		}
	})

	t.Run("an app id rule blocks the submission outright with 422", func(t *testing.T) {
		admin.Post("/api/admin/blacklist", map[string]string{"field": "app_id", "pattern": "123456"})
		user := app.NewUser("blocked")
		resp := user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/123456"})
		if resp.Code() != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want 422", resp.Code())
		}
		var n int
		app.Pool.QueryRow(t.Context(), "SELECT count(*) FROM tracked_games WHERE app_id = 123456").Scan(&n)
		if n != 0 {
			t.Error("a blacklisted submission was still recorded")
		}
	})
}

func TestAdminStats(t *testing.T) {
	app := testutil.NewApp(t)
	admin := app.NewAdmin()
	fan := app.NewUser("fan")
	app.SeedGame(881001, "Popular Game", []string{"Dev"}, []string{"Pub"})
	fan.Put("/api/me/watchlist/881001", map[string]any{"pinned": true})
	admin.Put("/api/me/watchlist/881001", map[string]any{"pinned": false})

	resp := admin.Get("/api/admin/stats")
	if resp.Code() != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
	}
	stats := resp.JSON()
	for _, key := range []string{"users", "tracked_games", "awaiting_approval", "watchlist_entries", "most_watched", "most_pinned", "top_submitters", "top_watchers", "activity", "recent_users", "recent_items"} {
		if _, ok := stats[key]; !ok {
			t.Errorf("stats missing %q", key)
		}
	}
	if stats["users"] != float64(2) {
		t.Errorf("users = %v, want 2", stats["users"])
	}
	if stats["watchlist_entries"] != float64(2) || stats["pinned_entries"] != float64(1) {
		t.Errorf("watchlist/pinned = %v/%v, want 2/1", stats["watchlist_entries"], stats["pinned_entries"])
	}
	watched := stats["most_watched"].([]any)
	if len(watched) != 1 || watched[0].(map[string]any)["name"] != "Popular Game" || watched[0].(map[string]any)["count"] != float64(2) {
		t.Errorf("most_watched = %v", watched)
	}
	pinned := stats["most_pinned"].([]any)
	if len(pinned) != 1 || pinned[0].(map[string]any)["count"] != float64(1) {
		t.Errorf("most_pinned = %v", pinned)
	}
	if got := len(stats["activity"].([]any)); got != 14 {
		t.Errorf("activity has %d days, want 14 (including empty days)", got)
	}
}
