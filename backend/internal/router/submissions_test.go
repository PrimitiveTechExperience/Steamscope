package router_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func TestSubmissions(t *testing.T) {
	app := testutil.NewApp(t)

	t.Run("require login", func(t *testing.T) {
		anon := app.NewClient()
		if got := anon.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/730"}).Code(); got != http.StatusUnauthorized {
			t.Errorf("submit: status = %d, want 401", got)
		}
		if got := anon.Get("/api/me/submissions").Code(); got != http.StatusUnauthorized {
			t.Errorf("list: status = %d, want 401", got)
		}
	})

	t.Run("reject URLs that are not Steam store links with 400", func(t *testing.T) {
		user := app.NewUser("badurl")
		for _, url := range []string{
			"",
			"not a url",
			"javascript:alert(1)",
			"https://evil.example/app/730",
			"https://store.steampowered.com.evil.example/app/730",
			"https://evil.example/?u=store.steampowered.com/app/730",
			"https://store.steampowered.com/app/abc",
			"https://store.steampowered.com/app/0",
			"https://store.steampowered.com/app/-5",
			"https://store.steampowered.com/app/99999999999999999999",
			"https://store.steampowered.com/sub/123",
			"https://store.steampowered.com/",
			"ftp://store.steampowered.com/app/730",
		} {
			resp := user.Post("/api/submissions", map[string]string{"url": url})
			if resp.Code() != http.StatusBadRequest {
				t.Errorf("%q: status = %d, want 400", url, resp.Code())
			} else if resp.Error() == "" {
				t.Errorf("%q: 400 has no error message", url)
			}
		}
		if n := len(user.Get("/api/me/submissions").JSONArray()); n != 0 {
			t.Errorf("rejected URLs created %d submissions", n)
		}
	})

	t.Run("rejects malformed bodies with 400", func(t *testing.T) {
		user := app.NewUser("badbody")
		for name, body := range map[string]string{"not json": "{", "unknown field": `{"url":"https://store.steampowered.com/app/730","status":"tracked"}`, "missing url": `{}`} {
			if got := user.Post("/api/submissions", body).Code(); got != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", name, got)
			}
		}
	})

	t.Run("a user's submission waits for admin approval", func(t *testing.T) {
		user := app.NewUser("polite")
		resp := user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/730/Counter-Strike_2/?l=english"})
		if resp.Code() != http.StatusAccepted {
			t.Fatalf("status = %d, want 202: %s", resp.Code(), resp.Body.String())
		}
		body := resp.JSON()
		if body["kind"] != "app" || body["id"] != float64(730) || body["status"] != "awaiting_approval" {
			t.Errorf("unexpected response: %v", body)
		}

		// Nothing is scraped before approval: no games row, status stays awaiting_approval.
		var games int
		app.Pool.QueryRow(t.Context(), "SELECT count(*) FROM games WHERE app_id = 730").Scan(&games)
		if games != 0 {
			t.Error("a game was added before an admin approved it")
		}

		again := user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/730"})
		if again.Code() != http.StatusOK || again.JSON()["status"] != "awaiting_approval" {
			t.Errorf("resubmitting: status = %d body = %s, want 200 awaiting_approval", again.Code(), again.Body.String())
		}

		list := user.Get("/api/me/submissions").JSONArray()
		if len(list) != 1 {
			t.Fatalf("submissions listed = %d, want 1 (no duplicates)", len(list))
		}
		first := list[0].(map[string]any)
		if first["kind"] != "app" || first["id"] != float64(730) || first["status"] != "awaiting_approval" {
			t.Errorf("listed submission = %v", first)
		}
	})

	t.Run("another user submitting the same game does not reset it", func(t *testing.T) {
		other := app.NewUser("copycat")
		resp := other.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/730"})
		if resp.Code() != http.StatusOK || resp.JSON()["status"] != "awaiting_approval" {
			t.Errorf("status = %d body = %s", resp.Code(), resp.Body.String())
		}
	})

	t.Run("bundle links are accepted", func(t *testing.T) {
		user := app.NewUser("bundler")
		resp := user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/bundle/232/Valve_Complete_Pack/"})
		if resp.Code() != http.StatusAccepted || resp.JSON()["kind"] != "bundle" || resp.JSON()["id"] != float64(232) {
			t.Errorf("status = %d body = %s", resp.Code(), resp.Body.String())
		}
		kinds := map[string]bool{}
		for _, s := range user.Get("/api/me/submissions").JSONArray() {
			kinds[s.(map[string]any)["kind"].(string)] = true
		}
		if !kinds["bundle"] {
			t.Error("bundle submission missing from the user's list")
		}
	})

	t.Run("an already-tracked game is reported, not re-queued", func(t *testing.T) {
		app.SeedGame(660001, "Already Here", []string{"d"}, []string{"p"})
		user := app.NewUser("dupgame")
		resp := user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/660001"})
		if resp.Code() != http.StatusOK || resp.JSON()["status"] != "tracked" {
			t.Errorf("status = %d body = %s, want 200 tracked", resp.Code(), resp.Body.String())
		}
	})

	t.Run("a failed submission can be retried", func(t *testing.T) {
		user := app.NewUser("retry")
		user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/660002"})
		app.Pool.Exec(t.Context(), "UPDATE tracked_games SET status = 'failed' WHERE app_id = 660002")

		resp := user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/660002"})
		if resp.Code() != http.StatusAccepted || resp.JSON()["status"] != "awaiting_approval" {
			t.Errorf("retry: status = %d body = %s, want 202 awaiting_approval", resp.Code(), resp.Body.String())
		}
	})

	t.Run("a rejected game stays rejected", func(t *testing.T) {
		user := app.NewUser("rejected")
		user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/660003"})
		app.Pool.Exec(t.Context(), "UPDATE tracked_games SET status = 'rejected' WHERE app_id = 660003")

		resp := user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/660003"})
		if resp.Code() != http.StatusOK || resp.JSON()["status"] != "rejected" {
			t.Errorf("status = %d body = %s, want 200 rejected", resp.Code(), resp.Body.String())
		}
	})

	t.Run("users are limited to 5 submissions an hour", func(t *testing.T) {
		user := app.NewUser("spammer")
		for i := 0; i < 5; i++ {
			resp := user.Post("/api/submissions", map[string]string{"url": fmt.Sprintf("https://store.steampowered.com/app/%d", 670000+i)})
			if resp.Code() != http.StatusAccepted {
				t.Fatalf("submission %d: status = %d, want 202", i, resp.Code())
			}
		}
		resp := user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/670099"})
		if resp.Code() != http.StatusTooManyRequests {
			t.Fatalf("6th submission: status = %d, want 429", resp.Code())
		}
		if resp.Error() != "you can submit up to 5 games or bundles an hour" {
			t.Errorf("error = %q", resp.Error())
		}
	})

	t.Run("admins skip approval and the rate limit", func(t *testing.T) {
		admin := app.NewAdmin()
		for i := 0; i < 7; i++ {
			resp := admin.Post("/api/submissions", map[string]string{"url": fmt.Sprintf("https://store.steampowered.com/app/%d", 680000+i)})
			if resp.Code() != http.StatusAccepted {
				t.Fatalf("admin submission %d: status = %d, want 202", i, resp.Code())
			}
			if resp.JSON()["status"] != "pending" {
				t.Errorf("admin submission %d: status = %v, want pending (straight to the scrape queue)", i, resp.JSON()["status"])
			}
		}
	})

	t.Run("submission-blocked users get 403", func(t *testing.T) {
		user := app.NewUser("muted")
		app.SetFlag(user.UserID, "submissions_blocked", true)
		resp := user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/690001"})
		if resp.Code() != http.StatusForbidden {
			t.Errorf("status = %d, want 403", resp.Code())
		}
	})

	t.Run("each user only sees their own submissions", func(t *testing.T) {
		a, b := app.NewUser("seer"), app.NewUser("other")
		a.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/691001"})
		if n := len(b.Get("/api/me/submissions").JSONArray()); n != 0 {
			t.Errorf("another user sees %d of someone else's submissions", n)
		}
	})
}
