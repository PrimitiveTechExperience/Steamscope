package router_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func TestSecurityHeaders(t *testing.T) {
	app := testutil.NewApp(t)
	c := app.NewClient()

	t.Run("every response carries them", func(t *testing.T) {
		for _, path := range []string{"/api/games", "/api/auth/me", "/api/does-not-exist"} {
			h := c.Get(path).Header()
			want := map[string]string{
				"X-Content-Type-Options": "nosniff",
				"X-Frame-Options":        "DENY",
				"Referrer-Policy":        "no-referrer",
			}
			for k, v := range want {
				if got := h.Get(k); got != v {
					t.Errorf("%s: %s = %q, want %q", path, k, got, v)
				}
			}
			csp := h.Get("Content-Security-Policy")
			for _, part := range []string{"default-src 'none'", "frame-ancestors 'none'"} {
				if !strings.Contains(csp, part) {
					t.Errorf("%s: CSP %q lacks %q", path, csp, part)
				}
			}
		}
	})

	t.Run("answers about the user are not stored, public data can be", func(t *testing.T) {
		if got := c.Get("/api/auth/me").Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("/api/auth/me Cache-Control = %q, want no-store", got)
		}
		if got := c.Get("/api/games").Header().Get("Cache-Control"); got != "" {
			t.Errorf("/api/games Cache-Control = %q, want none", got)
		}
	})

	t.Run("no HSTS over plain http", func(t *testing.T) {
		if got := c.Get("/api/games").Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("HSTS sent without HTTPS: %q", got)
		}
	})

	t.Run("CORS preflights still work", func(t *testing.T) {
		if got := c.Do(http.MethodOptions, "/api/auth/login", nil).Code(); got != http.StatusNoContent {
			t.Errorf("preflight: status = %d, want 204", got)
		}
	})
}

func username(t *testing.T, c *testutil.Client) string {
	t.Helper()
	return c.Get("/api/auth/me").JSON()["user"].(map[string]any)["username"].(string)
}

func login(c *testutil.Client, name, password string) testutil.Response {
	return c.Post("/api/auth/login", map[string]string{"login": name, "password": password})
}

// failLogins makes n wrong-password attempts, from fresh addresses so the
// per-address rate limit never steps in: this is the spread-out attack the
// lockout is for.
func failLogins(t *testing.T, app *testutil.App, name string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if got := login(app.NewClient(), name, "wrong-password-1").Code(); got != http.StatusUnauthorized {
			t.Fatalf("failure %d: status = %d, want 401", i+1, got)
		}
	}
}

func waitForMail(t *testing.T, m *testutil.FakeMailer, n int) []testutil.SentMail {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if sent := m.Sent(); len(sent) >= n {
			return sent
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d emails, got %d", n, len(m.Sent()))
	return nil
}

func TestAccountLockout(t *testing.T) {
	app := testutil.NewApp(t)
	fake := app.FakeAlerts(true, false)

	t.Run("ten failures from different addresses lock the account", func(t *testing.T) {
		victim := app.NewUser("lock")
		name := username(t, victim)
		failLogins(t, app, name, 10)

		resp := login(app.NewClient(), name, "correct-horse-battery")
		if resp.Code() != http.StatusTooManyRequests {
			t.Fatalf("correct password while locked: status = %d, want 429", resp.Code())
		}
		if !strings.Contains(resp.Error(), "Try again in 30 minutes") {
			t.Errorf("lockout message = %q", resp.Error())
		}
		if resp.Header().Get("Set-Cookie") != "" {
			t.Error("a locked account was given a session")
		}
		// The existing session is not affected.
		if victim.Get("/api/auth/me").JSON()["user"] == nil {
			t.Error("locking ended the owner's existing session")
		}

		mails := waitForMail(t, fake.Mail, 1)
		if !strings.Contains(mails[0].To, "@example.com") || !strings.Contains(mails[0].Body, "/forgot-password") {
			t.Errorf("lockout email: %+v", mails[0])
		}
	})

	t.Run("username and email share one count", func(t *testing.T) {
		victim := app.NewUser("both")
		name := username(t, victim)
		failLogins(t, app, name, 5)
		failLogins(t, app, name+"@example.com", 5)
		if got := login(app.NewClient(), name, "correct-horse-battery").Code(); got != http.StatusTooManyRequests {
			t.Errorf("status = %d, want 429", got)
		}
	})

	t.Run("a name with no account locks the same way, with the same message", func(t *testing.T) {
		failLogins(t, app, "nobody-by-this-name", 10)
		ghost := login(app.NewClient(), "nobody-by-this-name", "whatever-password")

		victim := app.NewUser("same")
		name := username(t, victim)
		failLogins(t, app, name, 10)
		real := login(app.NewClient(), name, "whatever-password")

		if ghost.Code() != http.StatusTooManyRequests || real.Code() != http.StatusTooManyRequests {
			t.Fatalf("statuses %d and %d, want 429 for both", ghost.Code(), real.Code())
		}
		if ghost.Error() != real.Error() {
			t.Errorf("messages differ (%q vs %q): accounts could be told apart", ghost.Error(), real.Error())
		}
	})

	t.Run("a successful sign-in resets the count", func(t *testing.T) {
		victim := app.NewUser("reset")
		name := username(t, victim)
		failLogins(t, app, name, 9)
		if got := login(app.NewClient(), name, "correct-horse-battery").Code(); got != http.StatusOK {
			t.Fatalf("correct password after 9 failures: status = %d, want 200", got)
		}
		failLogins(t, app, name, 9)
		if got := login(app.NewClient(), name, "correct-horse-battery").Code(); got != http.StatusOK {
			t.Errorf("count was not reset: status = %d, want 200", got)
		}
	})

	t.Run("the lock lifts after 30 minutes", func(t *testing.T) {
		victim := app.NewUser("wait")
		name := username(t, victim)
		failLogins(t, app, name, 10)
		app.Mini.FastForward(29 * time.Minute)
		if got := login(app.NewClient(), name, "correct-horse-battery").Code(); got != http.StatusTooManyRequests {
			t.Fatalf("after 29 minutes: status = %d, want 429", got)
		}
		app.Mini.FastForward(2 * time.Minute)
		if got := login(app.NewClient(), name, "correct-horse-battery").Code(); got != http.StatusOK {
			t.Errorf("after 31 minutes: status = %d, want 200", got)
		}
	})

	t.Run("failures spread over more than an hour don't add up", func(t *testing.T) {
		victim := app.NewUser("slow")
		name := username(t, victim)
		failLogins(t, app, name, 9)
		app.Mini.FastForward(61 * time.Minute)
		failLogins(t, app, name, 9)
		if got := login(app.NewClient(), name, "correct-horse-battery").Code(); got != http.StatusOK {
			t.Errorf("status = %d, want 200", got)
		}
	})
}

var tokenInLink = regexp.MustCompile(`/reset-password\?token=([A-Za-z0-9_%-]+)`)

func resetToken(t *testing.T, mail testutil.SentMail) string {
	t.Helper()
	m := tokenInLink.FindStringSubmatch(mail.Body)
	if m == nil {
		t.Fatalf("no reset link in email: %q", mail.Body)
	}
	token, err := url.QueryUnescape(m[1])
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestPasswordReset(t *testing.T) {
	t.Run("is unavailable without email", func(t *testing.T) {
		app := testutil.NewApp(t)
		resp := app.NewClient().Post("/api/auth/password-reset", map[string]string{"email": "a@example.com"})
		if resp.Code() != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", resp.Code())
		}
	})

	app := testutil.NewApp(t)
	fake := app.FakeAlerts(true, false)
	request := func(email string) testutil.Response {
		return app.NewClient().Post("/api/auth/password-reset", map[string]string{"email": email})
	}
	confirm := func(token, password string) testutil.Response {
		return app.NewClient().Post("/api/auth/password-reset/confirm", map[string]string{"token": token, "password": password})
	}

	t.Run("answers the same whether or not the email has an account", func(t *testing.T) {
		user := app.NewUser("same")
		known := request(username(t, user) + "@example.com")
		unknown := request("nobody-here@example.com")
		if known.Code() != http.StatusAccepted || unknown.Code() != http.StatusAccepted {
			t.Fatalf("statuses %d and %d, want 202", known.Code(), unknown.Code())
		}
		if known.Body.String() != unknown.Body.String() {
			t.Errorf("bodies differ: %s vs %s", known.Body.String(), unknown.Body.String())
		}
		mails := waitForMail(t, fake.Mail, 1)
		time.Sleep(100 * time.Millisecond) // give a wrong second email the chance to show up
		if got := len(fake.Mail.Sent()); got != 1 || !strings.HasPrefix(mails[0].To, username(t, user)) {
			t.Errorf("want exactly one email, to the account: %+v", fake.Mail.Sent())
		}
	})

	t.Run("rejects a malformed email", func(t *testing.T) {
		if got := request("not an email").Code(); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})

	t.Run("the link sets a new password once and signs out everywhere", func(t *testing.T) {
		user := app.NewUser("rst")
		name := username(t, user)
		other := app.NewClient()
		if got := login(other, name, "correct-horse-battery").Code(); got != http.StatusOK {
			t.Fatalf("second device login: %d", got)
		}
		before := len(fake.Mail.Sent())
		request(name + "@example.com")
		token := resetToken(t, waitForMail(t, fake.Mail, before+1)[before])

		if got := confirm(token, "short").Code(); got != http.StatusBadRequest {
			t.Errorf("weak password: status = %d, want 400", got)
		}
		resp := confirm(token, "a-brand-new-password")
		if resp.Code() != http.StatusOK {
			t.Fatalf("confirm: status = %d %s", resp.Code(), resp.Body.String())
		}
		for i, c := range []*testutil.Client{user, other} {
			if c.Get("/api/auth/me").JSON()["user"] != nil {
				t.Errorf("session %d survived the reset", i)
			}
		}
		if got := login(app.NewClient(), name, "correct-horse-battery").Code(); got != http.StatusUnauthorized {
			t.Errorf("old password: status = %d, want 401", got)
		}
		if got := login(app.NewClient(), name, "a-brand-new-password").Code(); got != http.StatusOK {
			t.Errorf("new password: status = %d, want 200", got)
		}
		if got := confirm(token, "yet-another-password").Code(); got != http.StatusBadRequest {
			t.Errorf("reusing the link: status = %d, want 400", got)
		}
		changed := waitForMail(t, fake.Mail, before+2)[before+1]
		if !strings.Contains(changed.Subject, "password was changed") {
			t.Errorf("no password-changed email: %+v", changed)
		}
	})

	t.Run("only the hash of the token is stored", func(t *testing.T) {
		user := app.NewUser("hash")
		before := len(fake.Mail.Sent())
		request(username(t, user) + "@example.com")
		token := resetToken(t, waitForMail(t, fake.Mail, before+1)[before])
		var raw, hashed int
		app.Pool.QueryRow(t.Context(), "SELECT count(*) FROM password_reset_tokens WHERE token_hash = convert_to($1, 'UTF8')", token).Scan(&raw)
		app.Pool.QueryRow(t.Context(), "SELECT count(*) FROM password_reset_tokens WHERE token_hash = sha256(convert_to($1, 'UTF8'))", token).Scan(&hashed)
		if raw != 0 || hashed != 1 {
			t.Errorf("rows with the raw token: %d (want 0), with its SHA-256: %d (want 1)", raw, hashed)
		}
	})

	t.Run("a newer link replaces nothing, but using one voids the others", func(t *testing.T) {
		user := app.NewUser("two")
		email := username(t, user) + "@example.com"
		before := len(fake.Mail.Sent())
		request(email)
		request(email)
		mails := waitForMail(t, fake.Mail, before+2)
		first, second := resetToken(t, mails[before]), resetToken(t, mails[before+1])
		if got := confirm(second, "second-new-password").Code(); got != http.StatusOK {
			t.Fatalf("second link: status = %d", got)
		}
		if got := confirm(first, "first-new-password").Code(); got != http.StatusBadRequest {
			t.Errorf("first link after the second was used: status = %d, want 400", got)
		}
	})

	t.Run("an expired link doesn't work", func(t *testing.T) {
		user := app.NewUser("old")
		before := len(fake.Mail.Sent())
		request(username(t, user) + "@example.com")
		token := resetToken(t, waitForMail(t, fake.Mail, before+1)[before])
		app.Pool.Exec(t.Context(), "UPDATE password_reset_tokens SET expires_at = now() - interval '1 second' WHERE user_id = $1", user.UserID)
		if got := confirm(token, "too-late-password").Code(); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})

	t.Run("made-up tokens are refused", func(t *testing.T) {
		for _, token := range []string{"", "short", strings.Repeat("A", 43), strings.Repeat("A", 500)} {
			if got := confirm(token, "some-new-password").Code(); got != http.StatusBadRequest {
				t.Errorf("token %q: status = %d, want 400", token[:min(len(token), 10)], got)
			}
		}
	})

	t.Run("resetting lifts a lockout", func(t *testing.T) {
		user := app.NewUser("unlock")
		name := username(t, user)
		before := len(fake.Mail.Sent())
		failLogins(t, app, name, 10)
		waitForMail(t, fake.Mail, before+1) // the lockout email
		request(name + "@example.com")
		token := resetToken(t, waitForMail(t, fake.Mail, before+2)[before+1])
		if got := confirm(token, "unlocked-password").Code(); got != http.StatusOK {
			t.Fatalf("confirm: %d", got)
		}
		if got := login(app.NewClient(), name, "unlocked-password").Code(); got != http.StatusOK {
			t.Errorf("login after reset: status = %d, want 200", got)
		}
	})

	t.Run("banned accounts get no link", func(t *testing.T) {
		user := app.NewUser("banned")
		email := username(t, user) + "@example.com"
		app.SetFlag(user.UserID, "is_banned", true)
		before := len(fake.Mail.Sent())
		if got := request(email).Code(); got != http.StatusAccepted {
			t.Errorf("status = %d, want 202", got)
		}
		time.Sleep(200 * time.Millisecond)
		if got := len(fake.Mail.Sent()); got != before {
			t.Errorf("a banned account was sent %d emails", got-before)
		}
	})

	t.Run("one inbox gets at most three links an hour", func(t *testing.T) {
		user := app.NewUser("flood")
		email := username(t, user) + "@example.com"
		before := len(fake.Mail.Sent())
		for i := 0; i < 5; i++ {
			if got := request(email).Code(); got != http.StatusAccepted {
				t.Fatalf("request %d: status = %d, want 202", i, got)
			}
		}
		waitForMail(t, fake.Mail, before+3)
		time.Sleep(200 * time.Millisecond)
		if got := len(fake.Mail.Sent()) - before; got != 3 {
			t.Errorf("sent %d emails, want 3", got)
		}
	})

	t.Run("requests are rate limited per address", func(t *testing.T) {
		c := app.NewClient()
		for i := 0; i < 5; i++ {
			c.Post("/api/auth/password-reset", map[string]string{"email": fmt.Sprintf("x%d@example.com", i)})
		}
		if got := c.Post("/api/auth/password-reset", map[string]string{"email": "y@example.com"}).Code(); got != http.StatusTooManyRequests {
			t.Errorf("6th request: status = %d, want 429", got)
		}
	})
}

func TestAdminAuditLog(t *testing.T) {
	app := testutil.NewApp(t)
	admin := app.NewAdmin()
	adminName := username(t, admin)

	entries := func(query string) []map[string]any {
		t.Helper()
		resp := admin.Get("/api/admin/audit" + query)
		if resp.Code() != http.StatusOK {
			t.Fatalf("audit log: status = %d %s", resp.Code(), resp.Body.String())
		}
		var out []map[string]any
		for _, e := range resp.JSONArray() {
			out = append(out, e.(map[string]any))
		}
		return out
	}
	latest := func() map[string]any {
		t.Helper()
		list := entries("?limit=1")
		if len(list) == 0 {
			t.Fatal("audit log is empty")
		}
		return list[0]
	}
	expect := func(e map[string]any, action, targetType, targetID string) {
		t.Helper()
		if e["action"] != action || e["target_type"] != targetType || e["target_id"] != targetID {
			t.Errorf("entry = %v %v %v, want %s %s %s", e["action"], e["target_type"], e["target_id"], action, targetType, targetID)
		}
		if e["admin_username"] != adminName || e["admin_id"] != float64(admin.UserID) {
			t.Errorf("entry admin = %v/%v, want %s/%d", e["admin_username"], e["admin_id"], adminName, admin.UserID)
		}
		if e["ip"] == "" {
			t.Error("entry has no IP")
		}
	}

	t.Run("is admin-only", func(t *testing.T) {
		if got := app.NewUser("plain").Get("/api/admin/audit").Code(); got != http.StatusForbidden {
			t.Errorf("non-admin: status = %d, want 403", got)
		}
		if got := app.NewClient().Get("/api/admin/audit").Code(); got != http.StatusUnauthorized {
			t.Errorf("logged out: status = %d, want 401", got)
		}
	})

	t.Run("records bans and submission blocks", func(t *testing.T) {
		user := app.NewUser("mod")
		id := fmt.Sprint(user.UserID)
		name := username(t, user)
		admin.Put("/api/admin/users/"+id+"/moderation", map[string]bool{"is_banned": true})
		e := latest()
		expect(e, "user.ban", "user", id)
		if e["details"].(map[string]any)["username"] != name {
			t.Errorf("ban details = %v", e["details"])
		}
		admin.Put("/api/admin/users/"+id+"/moderation", map[string]bool{"is_banned": false, "submissions_blocked": true})
		list := entries("?limit=2")
		expect(list[0], "user.block_submissions", "user", id)
		expect(list[1], "user.unban", "user", id)
	})

	t.Run("a ban ends the user's sessions", func(t *testing.T) {
		user := app.NewUser("gone")
		admin.Put(fmt.Sprintf("/api/admin/users/%d/moderation", user.UserID), map[string]bool{"is_banned": true})
		var n int
		for _, key := range app.Mini.Keys() {
			if strings.HasPrefix(key, fmt.Sprintf("user_sessions:%d", user.UserID)) {
				n++
			}
		}
		if n != 0 {
			t.Error("the banned user's session index is still there")
		}
	})

	t.Run("records deleting a user, keeping who it was", func(t *testing.T) {
		user := app.NewUser("del")
		name := username(t, user)
		if got := admin.Delete(fmt.Sprintf("/api/admin/users/%d", user.UserID)).Code(); got != http.StatusNoContent {
			t.Fatalf("delete: %d", got)
		}
		e := latest()
		expect(e, "user.delete", "user", fmt.Sprint(user.UserID))
		if d := e["details"].(map[string]any); d["username"] != name || d["email"] != name+"@example.com" {
			t.Errorf("delete details = %v", d)
		}
	})

	t.Run("failed actions are not recorded", func(t *testing.T) {
		before := len(entries(""))
		admin.Delete("/api/admin/users/987654321")
		admin.Delete(fmt.Sprintf("/api/admin/users/%d", admin.UserID))
		admin.Delete("/api/admin/blacklist/987654321")
		if got := len(entries("")); got != before {
			t.Errorf("%d entries were added for actions that failed", got-before)
		}
	})

	t.Run("records blacklist rules being added, used and removed", func(t *testing.T) {
		app.SeedGame(880050, "Spammy Game", []string{"Spam Corp"}, []string{"Spam Corp"})
		resp := admin.Post("/api/admin/blacklist", map[string]string{"field": "developer", "pattern": "^spam corp", "note": "spam"})
		if resp.Code() != http.StatusCreated {
			t.Fatalf("add rule: %d %s", resp.Code(), resp.Body.String())
		}
		ruleID := fmt.Sprint(resp.JSON()["rule_id"])
		e := latest()
		expect(e, "blacklist.add", "blacklist_rule", ruleID)
		if d := e["details"].(map[string]any); d["pattern"] != "^spam corp" || d["field"] != "developer" {
			t.Errorf("add details = %v", d)
		}

		admin.Post("/api/admin/blacklist/"+ruleID+"/purge", nil)
		e = latest()
		expect(e, "blacklist.purge", "blacklist_rule", ruleID)
		d := e["details"].(map[string]any)
		games, _ := d["games"].([]any)
		if d["removed"] != float64(1) || len(games) != 1 || games[0].(map[string]any)["name"] != "Spammy Game" {
			t.Errorf("purge details = %v", d)
		}

		admin.Delete("/api/admin/blacklist/" + ruleID)
		e = latest()
		expect(e, "blacklist.delete", "blacklist_rule", ruleID)
		if e["details"].(map[string]any)["pattern"] != "^spam corp" {
			t.Errorf("delete details = %v", e["details"])
		}
	})

	t.Run("records deleting, approving and rejecting games and bundles", func(t *testing.T) {
		app.SeedGame(880060, "Doomed Game", nil, nil)
		if got := admin.Delete("/api/admin/items/app/880060").Code(); got != http.StatusNoContent {
			t.Fatalf("delete game: %d", got)
		}
		e := latest()
		expect(e, "item.delete", "game", "880060")
		if e["details"].(map[string]any)["name"] != "Doomed Game" {
			t.Errorf("delete details = %v", e["details"])
		}

		user := app.NewUser("sub")
		user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/app/880061"})
		user.Post("/api/submissions", map[string]string{"url": "https://store.steampowered.com/bundle/8861"})
		if got := admin.Post("/api/admin/items/app/880061/approve", nil).Code(); got != http.StatusNoContent {
			t.Fatalf("approve: %d", got)
		}
		e = latest()
		expect(e, "item.approve", "game", "880061")
		if e["details"].(map[string]any)["submitted_by"] != float64(user.UserID) {
			t.Errorf("approve details = %v", e["details"])
		}
		if got := admin.Post("/api/admin/items/bundle/8861/reject", nil).Code(); got != http.StatusNoContent {
			t.Fatalf("reject: %d", got)
		}
		expect(latest(), "item.reject", "bundle", "8861")
	})

	t.Run("filters by action and pages back", func(t *testing.T) {
		for _, e := range entries("?action=user.ban") {
			if e["action"] != "user.ban" {
				t.Errorf("filter let through %v", e["action"])
			}
		}
		all := entries("")
		if len(all) < 3 {
			t.Fatalf("only %d entries", len(all))
		}
		for i := 1; i < len(all); i++ {
			if all[i]["audit_id"].(float64) >= all[i-1]["audit_id"].(float64) {
				t.Fatal("entries are not newest first")
			}
		}
		page := entries(fmt.Sprintf("?before=%v&limit=2", all[0]["audit_id"]))
		if len(page) != 2 || page[0]["audit_id"] != all[1]["audit_id"] {
			t.Errorf("paging: got %v", page)
		}
	})

	t.Run("rejects bad parameters", func(t *testing.T) {
		for _, q := range []string{"?action=drop.table", "?before=abc", "?before=-1", "?limit=0", "?limit=500"} {
			if got := admin.Get("/api/admin/audit" + q).Code(); got != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", q, got)
			}
		}
	})

	t.Run("an entry survives its admin being removed", func(t *testing.T) {
		other := app.NewAdmin()
		target := app.NewUser("victim")
		other.Put(fmt.Sprintf("/api/admin/users/%d/moderation", target.UserID), map[string]bool{"is_banned": true})
		otherName := username(t, other)
		app.Pool.Exec(t.Context(), "DELETE FROM users WHERE user_id = $1", other.UserID)
		e := entries("?action=user.ban")[0]
		if e["admin_username"] != otherName || e["admin_id"] != nil {
			t.Errorf("entry after admin deletion = %v / %v", e["admin_username"], e["admin_id"])
		}
	})
}
