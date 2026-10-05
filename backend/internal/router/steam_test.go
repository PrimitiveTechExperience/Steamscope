package router_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func location(t *testing.T, resp testutil.Response) *url.URL {
	t.Helper()
	u, err := url.Parse(resp.Header().Get("Location"))
	if err != nil {
		t.Fatalf("bad Location header %q: %v", resp.Header().Get("Location"), err)
	}
	return u
}

func TestSteamSignIn(t *testing.T) {
	app := testutil.NewApp(t)

	t.Run("login redirects to Steam with a single-use state token", func(t *testing.T) {
		resp := app.NewClient().Get("/api/auth/steam/login")
		if resp.Code() != http.StatusFound {
			t.Fatalf("status = %d, want 302", resp.Code())
		}
		loc := location(t, resp)
		if loc.Host != "steamcommunity.com" || loc.Path != "/openid/login" {
			t.Errorf("redirects to %s, want steamcommunity.com OpenID", loc)
		}
		returnTo := loc.Query().Get("openid.return_to")
		if !strings.HasPrefix(returnTo, testutil.BackendURL+"/api/auth/steam/callback?state=") {
			t.Errorf("return_to = %q", returnTo)
		}
		state := strings.TrimPrefix(returnTo, testutil.BackendURL+"/api/auth/steam/callback?state=")
		if v, err := app.Mini.Get("steamstate:" + state); err != nil || v != "login" {
			t.Errorf("state not stored for a login intent: %q, %v", v, err)
		}
	})

	t.Run("each sign-in gets a different state", func(t *testing.T) {
		a := location(t, app.NewClient().Get("/api/auth/steam/login")).Query().Get("openid.return_to")
		b := location(t, app.NewClient().Get("/api/auth/steam/login")).Query().Get("openid.return_to")
		if a == b {
			t.Error("two sign-ins shared a state token")
		}
	})

	t.Run("link needs a logged-in user", func(t *testing.T) {
		resp := app.NewClient().Get("/api/auth/steam/link")
		if resp.Code() != http.StatusFound {
			t.Fatalf("status = %d, want 302", resp.Code())
		}
		loc := location(t, resp)
		if loc.Host != "localhost:4200" || loc.Path != "/login" || loc.Query().Get("returnUrl") != "/account" {
			t.Errorf("anonymous link redirects to %s", loc)
		}
	})

	t.Run("link binds the state to the logged-in user", func(t *testing.T) {
		user := app.NewUser("linker")
		resp := user.Get("/api/auth/steam/link")
		returnTo := location(t, resp).Query().Get("openid.return_to")
		state := strings.TrimPrefix(returnTo, testutil.BackendURL+"/api/auth/steam/callback?state=")
		want := "link:" + itoa(user.UserID)
		if v, _ := app.Mini.Get("steamstate:" + state); v != want {
			t.Errorf("stored intent = %q, want %q", v, want)
		}
	})

	t.Run("callback redirects to the frontend with a reason when it cannot sign you in", func(t *testing.T) {
		seed := func(state, intent string) { app.Mini.Set("steamstate:"+state, intent) }
		cases := []struct {
			name, query, intent, wantPath, wantSteam string
		}{
			{"no state", "", "", "/login", "error"},
			{"unknown or expired state", "state=nope", "", "/login", "expired"},
			{"cancelled on Steam (login)", "state=s1&openid.mode=cancel", "login", "/login", "cancelled"},
			{"cancelled on Steam (link)", "state=s2&openid.mode=cancel", "link:1", "/account", "cancelled"},
			{"unexpected openid mode", "state=s3&openid.mode=bogus", "login", "/login", "error"},
			{"missing openid response", "state=s4", "login", "/login", "error"},
			{"forged claimed id", "state=s5&openid.mode=id_res&openid.ns=http://specs.openid.net/auth/2.0&openid.op_endpoint=https://evil.example/login", "login", "/login", "error"},
			{"link failure goes back to the account page", "state=s6&openid.mode=bogus", "link:1", "/account", "error"},
		}
		for _, tc := range cases {
			if tc.intent != "" {
				seed(strings.TrimPrefix(strings.Split(tc.query, "&")[0], "state="), tc.intent)
			}
			resp := app.NewClient().Get("/api/auth/steam/callback?" + tc.query)
			if resp.Code() != http.StatusFound {
				t.Errorf("%s: status = %d, want 302", tc.name, resp.Code())
				continue
			}
			loc := location(t, resp)
			if loc.Host != "localhost:4200" || loc.Path != tc.wantPath || loc.Query().Get("steam") != tc.wantSteam {
				t.Errorf("%s: redirected to %s, want %s?steam=%s", tc.name, loc, tc.wantPath, tc.wantSteam)
			}
		}
	})

	t.Run("a state token can only be used once", func(t *testing.T) {
		app.Mini.Set("steamstate:once", "login")
		first := location(t, app.NewClient().Get("/api/auth/steam/callback?state=once&openid.mode=cancel"))
		if first.Query().Get("steam") != "cancelled" {
			t.Fatalf("first use: %s", first)
		}
		second := location(t, app.NewClient().Get("/api/auth/steam/callback?state=once&openid.mode=cancel"))
		if second.Query().Get("steam") != "expired" {
			t.Errorf("replayed state: %s, want steam=expired", second)
		}
	})

	t.Run("a failed sign-in never creates a session", func(t *testing.T) {
		app.Mini.Set("steamstate:nosession", "login")
		c := app.NewClient()
		c.Get("/api/auth/steam/callback?state=nosession&openid.mode=bogus")
		if c.Cookie("ss_session") != nil {
			t.Error("a session cookie was issued for a failed Steam sign-in")
		}
	})
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestSteamProfileEndpoints(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(1001, "Tracked Game", []string{"d"}, []string{"p"})

	t.Run("profile is null when Steam is not linked", func(t *testing.T) {
		resp := app.NewUser("nolink").Get("/api/me/steam-profile")
		if resp.Code() != http.StatusOK || resp.JSON()["profile"] != nil {
			t.Errorf("status = %d body = %s, want 200 {profile:null}", resp.Code(), resp.Body.String())
		}
	})

	t.Run("a linked account with no Steam API key gets a clear 503", func(t *testing.T) {
		user := app.NewUser("nokey")
		app.Pool.Exec(t.Context(), "UPDATE users SET steam_id = '76561198000000001' WHERE user_id = $1", user.UserID)
		resp := user.Get("/api/me/steam-profile")
		if resp.Code() != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", resp.Code())
		}
		if !strings.Contains(resp.Error(), "aren't configured") {
			t.Errorf("error = %q", resp.Error())
		}
	})

	t.Run("recently played games say whether they're already tracked", func(t *testing.T) {
		user := app.NewUser("played")
		const steamID = "76561198000000002"
		app.Pool.Exec(t.Context(), "UPDATE users SET steam_id = $2 WHERE user_id = $1", user.UserID, steamID)
		app.Pool.Exec(t.Context(), `INSERT INTO tracked_games (app_id, status) VALUES (1002, 'awaiting_approval'), (1003, 'rejected')`)

		cached, _ := json.Marshal(models.SteamProfile{
			SteamID: steamID, PersonaName: "Gamer", Status: "Online",
			RecentlyPlayed: []models.SteamPlayedGame{
				{AppID: 1001, Name: "Tracked Game"},
				{AppID: 1002, Name: "Requested Game"},
				{AppID: 1003, Name: "Declined Game"},
				{AppID: 1004, Name: "Unknown Game"},
			},
		})
		app.Mini.Set("steamprofile:"+steamID, string(cached))

		resp := user.Get("/api/me/steam-profile")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		games := resp.JSON()["profile"].(map[string]any)["recently_played"].([]any)
		got := map[float64]any{}
		for _, g := range games {
			m := g.(map[string]any)
			got[m["app_id"].(float64)] = m["track_status"]
		}
		want := map[float64]any{1001: "tracked", 1002: "awaiting_approval", 1003: "rejected", 1004: ""}
		for id, status := range want {
			if got[id] != status {
				t.Errorf("game %v track_status = %v, want %q", id, got[id], status)
			}
		}
	})

	t.Run("unlinking Steam clears the account link", func(t *testing.T) {
		user := app.NewUser("unlink")
		app.Pool.Exec(t.Context(), "UPDATE users SET steam_id = '76561198000000003' WHERE user_id = $1", user.UserID)
		if got := user.Delete("/api/me/steam").Code(); got != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", got)
		}
		if user.Get("/api/auth/me").JSON()["user"].(map[string]any)["steam_id"] != nil {
			t.Error("steam_id still set after unlinking")
		}
		if got := user.Delete("/api/me/steam").Code(); got != http.StatusNoContent {
			t.Errorf("unlinking twice: status = %d, want 204", got)
		}
	})

	t.Run("a Steam account can only be linked to one user", func(t *testing.T) {
		a, b := app.NewUser("owner"), app.NewUser("thief")
		app.Pool.Exec(t.Context(), "UPDATE users SET steam_id = '76561198000000004' WHERE user_id = $1", a.UserID)
		err := app.DB.SetSteamID(t.Context(), b.UserID, ptr("76561198000000004"))
		if err == nil {
			t.Error("the same Steam ID was linked to two accounts")
		}
	})
}

func ptr[T any](v T) *T { return &v }

func TestFeedEndpoint(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(2001, "Feed Game", []string{"d"}, []string{"p"})
	user := app.NewUser("feeder")

	resp := user.Get("/api/me/feed")
	if resp.Code() != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
	}
	feed := resp.JSON()
	for _, key := range []string{"watchlist", "deals", "suggestions"} {
		if _, ok := feed[key]; !ok {
			t.Errorf("feed missing %q", key)
		}
	}

	user.Put("/api/me/watchlist/2001", map[string]any{"pinned": true})
	watchlist := user.Get("/api/me/feed").JSON()["watchlist"].([]any)
	if len(watchlist) != 1 || watchlist[0].(map[string]any)["pinned"] != true {
		t.Errorf("watchlist in feed = %v", watchlist)
	}
}
