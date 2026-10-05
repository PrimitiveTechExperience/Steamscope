package router_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func TestRegister(t *testing.T) {
	app := testutil.NewApp(t)

	t.Run("creates the account, logs in and never leaks the password hash", func(t *testing.T) {
		c := app.NewClient()
		resp := c.Register("alice", "alice@example.com", "correct-horse-battery")

		if resp.Code() != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", resp.Code(), resp.Body.String())
		}
		user := resp.JSON()["user"].(map[string]any)
		if user["username"] != "alice" || user["email"] != "alice@example.com" {
			t.Errorf("unexpected user payload: %v", user)
		}
		if user["is_admin"] != false {
			t.Errorf("new users must not be admins, got is_admin=%v", user["is_admin"])
		}
		if strings.Contains(strings.ToLower(resp.Body.String()), "password") {
			t.Errorf("response leaks password data: %s", resp.Body.String())
		}
		ck := c.Cookie("ss_session")
		if ck == nil || ck.Value == "" {
			t.Fatal("no session cookie was set")
		}
		raw := resp.Header().Get("Set-Cookie")
		if !strings.Contains(raw, "HttpOnly") || !strings.Contains(raw, "SameSite=Lax") {
			t.Errorf("session cookie must be HttpOnly + SameSite=Lax, got %q", raw)
		}
		if me := c.Get("/api/auth/me").JSON()["user"]; me == nil {
			t.Error("the new session should be logged in")
		}
	})

	t.Run("rejects invalid input with 400", func(t *testing.T) {
		long := strings.Repeat("a", 129)
		cases := []struct{ name, username, email, password string }{
			{"username too short", "ab", "a@example.com", "correct-horse-battery"},
			{"username too long", strings.Repeat("u", 21), "a@example.com", "correct-horse-battery"},
			{"username with symbols", "bad name!", "a@example.com", "correct-horse-battery"},
			{"username with markup", "<script>", "a@example.com", "correct-horse-battery"},
			{"email without @", "validname", "nope", "correct-horse-battery"},
			{"email with a space", "validname", "a b@example.com", "correct-horse-battery"},
			{"email with display name", "validname", "Bob <bob@example.com>", "correct-horse-battery"},
			{"email with zero-width char", "validname", "a​@example.com", "correct-horse-battery"},
			{"email too long", "validname", strings.Repeat("a", 250) + "@example.com", "correct-horse-battery"},
			{"password too short", "validname", "a@example.com", "short"},
			{"password too long", "validname", "a@example.com", long},
			{"password with NUL", "validname", "a@example.com", "pass\x00word-long"},
			{"all empty", "", "", ""},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				resp := app.NewClient().Register(tc.username, tc.email, tc.password)
				if resp.Code() != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400: %s", resp.Code(), resp.Body.String())
				}
				if resp.Error() == "" {
					t.Error("400 responses should carry a JSON error message")
				}
			})
		}
	})

	t.Run("rejects malformed bodies with 400", func(t *testing.T) {
		bodies := map[string]string{
			"not json":      "{not json",
			"empty body":    "",
			"unknown field": `{"username":"zed","email":"z@example.com","password":"correct-horse-battery","is_admin":true}`,
			"wrong types":   `{"username":123,"email":[],"password":{}}`,
			"oversized":     `{"username":"` + strings.Repeat("x", 70<<10) + `"}`,
		}
		for name, body := range bodies {
			t.Run(name, func(t *testing.T) {
				resp := app.NewClient().Post("/api/auth/register", body)
				if resp.Code() != http.StatusBadRequest {
					t.Errorf("status = %d, want 400", resp.Code())
				}
			})
		}
	})

	t.Run("cannot self-promote to admin", func(t *testing.T) {
		c := app.NewClient()
		resp := c.Post("/api/auth/register", `{"username":"sneaky","email":"s@example.com","password":"correct-horse-battery","is_admin":true}`)
		if resp.Code() != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (unknown field)", resp.Code())
		}
		if got := app.NewClient().Get("/api/admin/users"); got.Code() != http.StatusUnauthorized {
			t.Errorf("anonymous admin access = %d, want 401", got.Code())
		}
	})

	t.Run("duplicate username or email returns 409", func(t *testing.T) {
		app.NewClient().Register("dupe", "dupe@example.com", "correct-horse-battery")

		sameName := app.NewClient().Register("DUPE", "other@example.com", "correct-horse-battery")
		if sameName.Code() != http.StatusConflict {
			t.Errorf("duplicate username (case-insensitive): status = %d, want 409", sameName.Code())
		}
		sameEmail := app.NewClient().Register("another", "DUPE@example.com", "correct-horse-battery")
		if sameEmail.Code() != http.StatusConflict {
			t.Errorf("duplicate email (case-insensitive): status = %d, want 409", sameEmail.Code())
		}
	})

	t.Run("is rate limited per IP", func(t *testing.T) {
		c := app.NewClient()
		for i := 0; i < 10; i++ {
			resp := c.Register(fmt.Sprintf("flood%02d", i), fmt.Sprintf("flood%d@example.com", i), "correct-horse-battery")
			if resp.Code() != http.StatusCreated {
				t.Fatalf("signup %d: status = %d, want 201", i, resp.Code())
			}
		}
		resp := c.Register("flood99", "flood99@example.com", "correct-horse-battery")
		if resp.Code() != http.StatusTooManyRequests {
			t.Errorf("11th signup from one IP: status = %d, want 429", resp.Code())
		}
	})
}

func TestLoginLogoutSessions(t *testing.T) {
	app := testutil.NewApp(t)
	setup := app.NewClient()
	setup.Register("bob", "bob@example.com", "correct-horse-battery")

	t.Run("logs in by username or by email, case-insensitively", func(t *testing.T) {
		for _, login := range []string{"bob", "BOB", "bob@example.com", "Bob@Example.com"} {
			resp := app.NewClient().Post("/api/auth/login", map[string]string{"login": login, "password": "correct-horse-battery"})
			if resp.Code() != http.StatusOK {
				t.Errorf("login as %q: status = %d, want 200", login, resp.Code())
			}
		}
	})

	t.Run("wrong password and unknown user are indistinguishable 401s", func(t *testing.T) {
		wrong := app.NewClient().Post("/api/auth/login", map[string]string{"login": "bob", "password": "wrong-password-1"})
		unknown := app.NewClient().Post("/api/auth/login", map[string]string{"login": "nobody", "password": "wrong-password-1"})
		if wrong.Code() != http.StatusUnauthorized || unknown.Code() != http.StatusUnauthorized {
			t.Fatalf("statuses = %d / %d, want 401 / 401", wrong.Code(), unknown.Code())
		}
		if wrong.Error() != unknown.Error() {
			t.Errorf("error messages differ (%q vs %q): usernames could be enumerated", wrong.Error(), unknown.Error())
		}
	})

	t.Run("rejects empty and hostile credentials with 401", func(t *testing.T) {
		cases := []map[string]string{
			{"login": "", "password": ""},
			{"login": "bob", "password": ""},
			{"login": "bob", "password": "pass\x00word"},
			{"login": "' OR '1'='1", "password": "' OR '1'='1"},
			{"login": "bob'; DROP TABLE users;--", "password": "x"},
			{"login": "bob", "password": strings.Repeat("p", 5000)},
		}
		for _, body := range cases {
			if resp := app.NewClient().Post("/api/auth/login", body); resp.Code() != http.StatusUnauthorized {
				t.Errorf("login %q: status = %d, want 401", body["login"], resp.Code())
			}
		}
		// ...and the users table is intact.
		if resp := app.NewClient().Post("/api/auth/login", map[string]string{"login": "bob", "password": "correct-horse-battery"}); resp.Code() != http.StatusOK {
			t.Errorf("bob can no longer log in after the injection attempts: %d", resp.Code())
		}
	})

	t.Run("rejects malformed bodies with 400", func(t *testing.T) {
		if resp := app.NewClient().Post("/api/auth/login", "{nope"); resp.Code() != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.Code())
		}
	})

	t.Run("me is 200 with null user when logged out", func(t *testing.T) {
		resp := app.NewClient().Get("/api/auth/me")
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d, want 200 (a 401 here spams the console)", resp.Code())
		}
		if resp.JSON()["user"] != nil {
			t.Errorf("user = %v, want null", resp.JSON()["user"])
		}
	})

	t.Run("logout ends the session server-side", func(t *testing.T) {
		c := app.NewClient()
		c.Post("/api/auth/login", map[string]string{"login": "bob", "password": "correct-horse-battery"})
		stolen := c.Cookie("ss_session").Value

		if resp := c.Post("/api/auth/logout", nil); resp.Code() != http.StatusNoContent {
			t.Fatalf("logout status = %d, want 204", resp.Code())
		}
		if c.Get("/api/auth/me").JSON()["user"] != nil {
			t.Error("still logged in after logout")
		}
		// Replaying the old cookie must not work: the session is deleted in Redis.
		replay := app.NewClient()
		replay.SetCookie("ss_session", stolen)
		if replay.Get("/api/auth/me").JSON()["user"] != nil {
			t.Error("a logged-out session cookie still authenticates")
		}
	})

	t.Run("tampered or garbage cookies are treated as logged out", func(t *testing.T) {
		valid := app.NewClient()
		valid.Post("/api/auth/login", map[string]string{"login": "bob", "password": "correct-horse-battery"})
		good := valid.Cookie("ss_session").Value

		for name, value := range map[string]string{
			"garbage":   "not-a-real-cookie",
			"truncated": good[:len(good)/2],
			"flipped":   good[:len(good)-2] + "AA",
			"empty-ish": "x",
		} {
			c := app.NewClient()
			c.SetCookie("ss_session", value)
			if c.Get("/api/auth/me").JSON()["user"] != nil {
				t.Errorf("%s cookie authenticated", name)
			}
			if resp := c.Get("/api/me/preferences"); resp.Code() != http.StatusUnauthorized {
				t.Errorf("%s cookie on a protected route: status = %d, want 401", name, resp.Code())
			}
		}
	})

	t.Run("login attempts are rate limited per IP and login name", func(t *testing.T) {
		c := app.NewClient()
		for i := 0; i < 10; i++ {
			if resp := c.Post("/api/auth/login", map[string]string{"login": "ratelimited", "password": "wrong-password-1"}); resp.Code() != http.StatusUnauthorized {
				t.Fatalf("attempt %d: status = %d, want 401", i, resp.Code())
			}
		}
		if resp := c.Post("/api/auth/login", map[string]string{"login": "ratelimited", "password": "wrong-password-1"}); resp.Code() != http.StatusTooManyRequests {
			t.Errorf("11th attempt: status = %d, want 429", resp.Code())
		}
	})

	t.Run("banned users cannot log in and lose their live session", func(t *testing.T) {
		victim := app.NewUser("banme")
		app.SetFlag(victim.UserID, "is_banned", true)

		if victim.Get("/api/auth/me").JSON()["user"] != nil {
			t.Error("a banned user's existing session still works")
		}
		if resp := victim.Get("/api/me/preferences"); resp.Code() != http.StatusUnauthorized {
			t.Errorf("banned user on a protected route: status = %d, want 401", resp.Code())
		}
		var login string
		app.Pool.QueryRow(t.Context(), "SELECT username FROM users WHERE user_id = $1", victim.UserID).Scan(&login)
		resp := app.NewClient().Post("/api/auth/login", map[string]string{"login": login, "password": "correct-horse-battery"})
		if resp.Code() != http.StatusForbidden {
			t.Errorf("banned login: status = %d, want 403", resp.Code())
		}
	})
}

func TestCSRFAndCORS(t *testing.T) {
	app := testutil.NewApp(t)

	t.Run("state-changing requests need the frontend's Origin", func(t *testing.T) {
		noOrigin := app.NewClient()
		noOrigin.NoOrigin = true
		if resp := noOrigin.Post("/api/auth/login", map[string]string{"login": "a", "password": "b"}); resp.Code() != http.StatusForbidden {
			t.Errorf("missing Origin: status = %d, want 403", resp.Code())
		}
		evil := app.NewClient()
		evil.Origin = "https://evil.example"
		if resp := evil.Post("/api/auth/login", map[string]string{"login": "a", "password": "b"}); resp.Code() != http.StatusForbidden {
			t.Errorf("foreign Origin: status = %d, want 403", resp.Code())
		}
		lookalike := app.NewClient()
		lookalike.Origin = testutil.FrontendURL + ".evil.example"
		if resp := lookalike.Post("/api/auth/login", map[string]string{"login": "a", "password": "b"}); resp.Code() != http.StatusForbidden {
			t.Errorf("look-alike Origin: status = %d, want 403", resp.Code())
		}
	})

	t.Run("blocked cross-origin writes never reach the handler", func(t *testing.T) {
		victim := app.NewUser("csrf")
		evil := app.NewClient()
		evil.Origin = "https://evil.example"
		evil.SetCookie("ss_session", victim.Cookie("ss_session").Value)
		if resp := evil.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/730"}); resp.Code() != http.StatusForbidden {
			t.Errorf("status = %d, want 403", resp.Code())
		}
		if subs := victim.Get("/api/me/submissions").JSONArray(); len(subs) != 0 {
			t.Errorf("a cross-origin request created %d submissions", len(subs))
		}
	})

	t.Run("reads do not need an Origin", func(t *testing.T) {
		c := app.NewClient()
		c.NoOrigin = true
		if resp := c.Get("/api/health"); resp.Code() != http.StatusOK {
			t.Errorf("status = %d, want 200", resp.Code())
		}
	})

	t.Run("CORS preflight is answered for the frontend origin only", func(t *testing.T) {
		resp := app.NewClient().Do(http.MethodOptions, "/api/submissions", nil)
		if resp.Code() != http.StatusNoContent {
			t.Fatalf("preflight status = %d, want 204", resp.Code())
		}
		h := resp.Header()
		if h.Get("Access-Control-Allow-Origin") != testutil.FrontendURL {
			t.Errorf("Allow-Origin = %q, want %q", h.Get("Access-Control-Allow-Origin"), testutil.FrontendURL)
		}
		if h.Get("Access-Control-Allow-Credentials") != "true" {
			t.Error("credentials must be allowed for cookie sessions")
		}
		if !strings.Contains(h.Get("Access-Control-Allow-Headers"), "X-Request-ID") {
			t.Error("X-Request-ID should be an allowed request header")
		}
		if !strings.Contains(h.Get("Access-Control-Expose-Headers"), "X-Request-ID") {
			t.Error("X-Request-ID should be exposed so the browser can read it")
		}
	})
}
