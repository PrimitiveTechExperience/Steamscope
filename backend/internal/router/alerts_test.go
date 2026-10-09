package router_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

// prefs returns a user's saved preferences.
func prefs(t *testing.T, c *testutil.Client) map[string]any {
	t.Helper()
	resp := c.Get("/api/me/preferences")
	if resp.Code() != http.StatusOK {
		t.Fatalf("get preferences: %d %s", resp.Code(), resp.Body.String())
	}
	return resp.JSON()
}

// savePrefs changes some preferences, keeping the rest as they are.
func savePrefs(c *testutil.Client, current map[string]any, change map[string]any) testutil.Response {
	next := map[string]any{}
	for k, v := range current {
		next[k] = v
	}
	for k, v := range change {
		next[k] = v
	}
	return c.Put("/api/me/preferences", next)
}

func subscribe(c *testutil.Client, endpoint string) testutil.Response {
	return c.Post("/api/me/push-subscription", map[string]any{
		"endpoint": endpoint,
		"keys":     map[string]any{"p256dh": "BPublicKeyValue", "auth": "AuthSecretValue"},
	})
}

func TestAlertChannelsAreOnlyOfferedWhenTheServerCanSendThem(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("chan")

	t.Run("needs a signed-in user", func(t *testing.T) {
		if got := app.NewClient().Get("/api/me/alert-channels").Code(); got != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})

	t.Run("a server with no mail or push settings offers only Discord", func(t *testing.T) {
		j := user.Get("/api/me/alert-channels").JSON()
		if j["email"] != false || j["push"] != false || j["discord"] != true || j["vapid_public_key"] != "" {
			t.Errorf("channels = %v", j)
		}
	})

	t.Run("a configured server offers them, with the key the browser needs", func(t *testing.T) {
		app.FakeAlerts(true, true)
		j := user.Get("/api/me/alert-channels").JSON()
		if j["email"] != true || j["push"] != true || j["discord"] != true || j["vapid_public_key"] != "test-vapid-public-key" {
			t.Errorf("channels = %v", j)
		}
	})
}

func TestAlertPreferences(t *testing.T) {
	app := testutil.NewApp(t)
	fake := app.FakeAlerts(true, true)
	user := app.NewUser("prefs")

	t.Run("everything is off by default", func(t *testing.T) {
		p := prefs(t, user)
		if p["alert_email"] != false || p["alert_discord"] != false || p["alert_push"] != false || p["discord_webhook_url"] != "" {
			t.Errorf("defaults = %v", p)
		}
	})

	t.Run("email can be switched on and off", func(t *testing.T) {
		if resp := savePrefs(user, prefs(t, user), map[string]any{"alert_email": true}); resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		if prefs(t, user)["alert_email"] != true {
			t.Error("the setting was not saved")
		}
		savePrefs(user, prefs(t, user), map[string]any{"alert_email": false})
		if prefs(t, user)["alert_email"] != false {
			t.Error("the setting was not cleared")
		}
	})

	t.Run("a Discord webhook is saved and returned to its owner", func(t *testing.T) {
		url := fake.Discord.WebhookURL()
		resp := savePrefs(user, prefs(t, user), map[string]any{"alert_discord": true, "discord_webhook_url": "  " + url + "  "})
		if resp.Code() != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
		}
		p := prefs(t, user)
		if p["alert_discord"] != true || p["discord_webhook_url"] != url {
			t.Errorf("saved = %v / %v, want the trimmed URL", p["alert_discord"], p["discord_webhook_url"])
		}
	})

	t.Run("one user's webhook is never shown to another", func(t *testing.T) {
		other := app.NewUser("other")
		if got := prefs(t, other)["discord_webhook_url"]; got != "" {
			t.Errorf("another user sees %v", got)
		}
	})

	t.Run("a URL that is not a Discord webhook is refused", func(t *testing.T) {
		for _, bad := range []string{"https://example.com/hook", "http://discord.com/api/webhooks/123456789012345678/abcdefghijklmnop", "https://169.254.169.254/latest", "not a url"} {
			resp := savePrefs(user, prefs(t, user), map[string]any{"discord_webhook_url": bad})
			if resp.Code() != http.StatusBadRequest || resp.Error() == "" {
				t.Errorf("%q: status = %d body = %s, want a JSON 400", bad, resp.Code(), resp.Body.String())
			}
		}
	})

	t.Run("Discord cannot be switched on without a webhook", func(t *testing.T) {
		fresh := app.NewUser("nohook")
		resp := savePrefs(fresh, prefs(t, fresh), map[string]any{"alert_discord": true})
		if resp.Code() != http.StatusBadRequest || !strings.Contains(resp.Error(), "webhook") {
			t.Errorf("status = %d body = %s", resp.Code(), resp.Body.String())
		}
	})

	t.Run("push cannot be switched on before a browser has subscribed", func(t *testing.T) {
		fresh := app.NewUser("nopush")
		resp := savePrefs(fresh, prefs(t, fresh), map[string]any{"alert_push": true})
		if resp.Code() != http.StatusBadRequest || !strings.Contains(resp.Error(), "browser") {
			t.Errorf("status = %d body = %s", resp.Code(), resp.Body.String())
		}
		if subscribe(fresh, "https://push.example/send/abc").Code() != http.StatusNoContent {
			t.Fatal("subscribe failed")
		}
		if resp := savePrefs(fresh, prefs(t, fresh), map[string]any{"alert_push": true}); resp.Code() != http.StatusOK {
			t.Errorf("after subscribing: status = %d body = %s", resp.Code(), resp.Body.String())
		}
	})

	t.Run("an unconfigured channel cannot be switched on", func(t *testing.T) {
		bare := testutil.NewApp(t) // no mail or push settings
		u := bare.NewUser("bare")
		resp := savePrefs(u, prefs(t, u), map[string]any{"alert_email": true})
		if resp.Code() != http.StatusBadRequest || !strings.Contains(resp.Error(), "not set up") {
			t.Errorf("email: status = %d body = %s", resp.Code(), resp.Body.String())
		}
		resp = savePrefs(u, prefs(t, u), map[string]any{"alert_push": true})
		if resp.Code() != http.StatusBadRequest || !strings.Contains(resp.Error(), "not set up") {
			t.Errorf("push: status = %d body = %s", resp.Code(), resp.Body.String())
		}
	})

	t.Run("a channel already on does not block saving something else if the server later loses it", func(t *testing.T) {
		u := app.NewUser("later")
		savePrefs(u, prefs(t, u), map[string]any{"alert_email": true})
		app.H.Alerts = testutil.NewApp(t).H.Alerts // a notifier with no mail configured
		resp := savePrefs(u, prefs(t, u), map[string]any{"theme": "light"})
		if resp.Code() != http.StatusOK {
			t.Errorf("status = %d body = %s, want the unrelated change saved", resp.Code(), resp.Body.String())
		}
		app.FakeAlerts(true, true)
	})

	t.Run("the other preferences still validate", func(t *testing.T) {
		resp := savePrefs(user, prefs(t, user), map[string]any{"theme": "purple"})
		if resp.Code() != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.Code())
		}
	})
}

func TestPushSubscriptions(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("sub")

	t.Run("refused when the server has no push keys", func(t *testing.T) {
		if got := subscribe(user, "https://push.example/send/1").Code(); got != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", got)
		}
	})

	app.FakeAlerts(false, true)
	count := func() int {
		var n int
		app.Pool.QueryRow(context.Background(), "SELECT count(*) FROM push_subscriptions WHERE user_id = $1", user.UserID).Scan(&n)
		return n
	}

	t.Run("needs a signed-in user", func(t *testing.T) {
		if got := subscribe(app.NewClient(), "https://push.example/send/1").Code(); got != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})

	t.Run("saves a browser, once, however often it asks", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			if got := subscribe(user, "https://push.example/send/1").Code(); got != http.StatusNoContent {
				t.Fatalf("status = %d", got)
			}
		}
		if count() != 1 {
			t.Errorf("%d subscriptions saved, want 1", count())
		}
	})

	t.Run("rejects subscriptions that are not valid", func(t *testing.T) {
		for _, endpoint := range []string{"", "http://push.example/insecure", "not a url", "https://" + strings.Repeat("a", 900)} {
			if got := subscribe(user, endpoint).Code(); got != http.StatusBadRequest {
				t.Errorf("endpoint %.30q: status = %d, want 400", endpoint, got)
			}
		}
		noKeys := user.Post("/api/me/push-subscription", map[string]any{"endpoint": "https://push.example/send/2", "keys": map[string]any{}})
		if noKeys.Code() != http.StatusBadRequest {
			t.Errorf("without keys: status = %d, want 400", noKeys.Code())
		}
	})

	t.Run("limits how many browsers one user can add", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			subscribe(user, fmt.Sprintf("https://push.example/many/%d", i))
		}
		if count() != 10 {
			t.Errorf("%d subscriptions, want the limit of 10", count())
		}
		if got := subscribe(user, "https://push.example/one-too-many").Code(); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})

	t.Run("a browser can be removed, and removing twice is fine", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			resp := user.Do(http.MethodDelete, "/api/me/push-subscription", map[string]any{"endpoint": "https://push.example/send/1"})
			if resp.Code() != http.StatusNoContent {
				t.Errorf("delete %d: status = %d", i, resp.Code())
			}
		}
		var n int
		app.Pool.QueryRow(context.Background(), "SELECT count(*) FROM push_subscriptions WHERE endpoint = 'https://push.example/send/1'").Scan(&n)
		if n != 0 {
			t.Error("the subscription is still there")
		}
	})

	t.Run("one user cannot remove another user's browser", func(t *testing.T) {
		owner, other := app.NewUser("owner"), app.NewUser("intruder")
		subscribe(owner, "https://push.example/owners")
		other.Do(http.MethodDelete, "/api/me/push-subscription", map[string]any{"endpoint": "https://push.example/owners"})
		var n int
		app.Pool.QueryRow(context.Background(), "SELECT count(*) FROM push_subscriptions WHERE endpoint = 'https://push.example/owners'").Scan(&n)
		if n != 1 {
			t.Error("another user was able to remove it")
		}
	})

	t.Run("a browser that signs in as someone else moves to them", func(t *testing.T) {
		first, second := app.NewUser("first"), app.NewUser("second")
		subscribe(first, "https://push.example/shared-browser")
		subscribe(second, "https://push.example/shared-browser")
		var owner int64
		app.Pool.QueryRow(context.Background(), "SELECT user_id FROM push_subscriptions WHERE endpoint = 'https://push.example/shared-browser'").Scan(&owner)
		if owner != second.UserID {
			t.Errorf("owner = %d, want the user who subscribed last (%d), so alerts do not go to the previous person", owner, second.UserID)
		}
	})
}

func TestSendTestAlert(t *testing.T) {
	app := testutil.NewApp(t)
	fake := app.FakeAlerts(true, true)
	user := app.NewUser("tester")
	send := func(body map[string]any) testutil.Response { return user.Post("/api/me/alerts/test", body) }

	t.Run("needs a signed-in user", func(t *testing.T) {
		if got := app.NewClient().Post("/api/me/alerts/test", map[string]any{"channel": "email"}).Code(); got != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})

	t.Run("rejects an unknown channel", func(t *testing.T) {
		if got := send(map[string]any{"channel": "fax"}).Code(); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})

	t.Run("email goes to the address on the account", func(t *testing.T) {
		if got := send(map[string]any{"channel": "email"}).Code(); got != http.StatusNoContent {
			t.Fatalf("status = %d", got)
		}
		mails := fake.Mail.Sent()
		if len(mails) != 1 {
			t.Fatalf("%d emails, want 1", len(mails))
		}
		if !strings.HasPrefix(mails[0].To, "tester") || !strings.HasSuffix(mails[0].To, "@example.com") {
			t.Errorf("sent to %q, want the account's own address", mails[0].To)
		}
		if mails[0].Subject != "Steamscope test alert" {
			t.Errorf("subject = %q", mails[0].Subject)
		}
	})

	t.Run("Discord can try a webhook that has not been saved yet", func(t *testing.T) {
		if got := send(map[string]any{"channel": "discord", "discord_webhook_url": fake.Discord.WebhookURL()}).Code(); got != http.StatusNoContent {
			t.Fatalf("status = %d", got)
		}
		if posts := fake.Discord.Posts(); len(posts) != 1 || !strings.Contains(posts[0], "Test alert") {
			t.Errorf("posts = %v", posts)
		}
	})

	t.Run("Discord with nothing to send to says so", func(t *testing.T) {
		resp := send(map[string]any{"channel": "discord"})
		if resp.Code() != http.StatusBadGateway || !strings.Contains(resp.Error(), "Nothing to send to") {
			t.Errorf("status = %d body = %s", resp.Code(), resp.Body.String())
		}
	})

	t.Run("a webhook Discord rejects is reported with its status", func(t *testing.T) {
		fake.Discord.SetStatus(http.StatusNotFound)
		defer fake.Discord.SetStatus(http.StatusNoContent)
		resp := send(map[string]any{"channel": "discord", "discord_webhook_url": fake.Discord.WebhookURL()})
		if resp.Code() != http.StatusBadGateway || !strings.Contains(resp.Error(), "404") {
			t.Errorf("status = %d body = %s", resp.Code(), resp.Body.String())
		}
	})

	t.Run("a webhook that is not Discord's is refused before anything is sent", func(t *testing.T) {
		resp := send(map[string]any{"channel": "discord", "discord_webhook_url": "https://example.com/x"})
		if resp.Code() != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.Code())
		}
	})

	t.Run("push goes to every subscribed browser", func(t *testing.T) {
		subscribe(user, "https://push.example/a")
		subscribe(user, "https://push.example/b")
		if got := send(map[string]any{"channel": "push"}).Code(); got != http.StatusNoContent {
			t.Fatalf("status = %d", got)
		}
		if n := len(fake.Push.Sent()); n != 2 {
			t.Errorf("%d push messages, want one per browser", n)
		}
	})

	t.Run("push with no browser subscribed says so", func(t *testing.T) {
		lone := app.NewUser("lone")
		resp := lone.Post("/api/me/alerts/test", map[string]any{"channel": "push"})
		if resp.Code() != http.StatusBadGateway || !strings.Contains(resp.Error(), "Nothing to send to") {
			t.Errorf("status = %d body = %s", resp.Code(), resp.Body.String())
		}
	})

	t.Run("a failing mail server is a 502 with a plain message and no internals", func(t *testing.T) {
		fake.Mail.Err = fmt.Errorf("dial tcp smtp.internal.example:587: connection refused (password=hunter2)")
		defer func() { fake.Mail.Err = nil }()
		resp := send(map[string]any{"channel": "email"})
		if resp.Code() != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", resp.Code())
		}
		if strings.Contains(resp.Body.String(), "hunter2") || strings.Contains(resp.Body.String(), "smtp.internal") {
			t.Errorf("response leaks server details: %s", resp.Body.String())
		}
		if resp.JSON()["request_id"] == "" || resp.JSON()["request_id"] == nil {
			t.Error("the response should carry a request ID for the log")
		}
	})

	t.Run("unavailable channels answer 503", func(t *testing.T) {
		bare := testutil.NewApp(t)
		bare.FakeAlerts(false, false)
		u := bare.NewUser("nochan")
		for _, ch := range []string{"email", "push"} {
			if got := u.Post("/api/me/alerts/test", map[string]any{"channel": ch}).Code(); got != http.StatusServiceUnavailable {
				t.Errorf("%s: status = %d, want 503", ch, got)
			}
		}
	})

	t.Run("sending tests is rate limited", func(t *testing.T) {
		spammer := app.NewUser("spam")
		var last int
		for i := 0; i < 12; i++ {
			last = spammer.Post("/api/me/alerts/test", map[string]any{"channel": "email"}).Code()
		}
		if last != http.StatusTooManyRequests {
			t.Errorf("12th test: status = %d, want 429", last)
		}
	})
}

// watchWithTarget makes the user watch a game with a target price, and gives
// the game a higher price yesterday so that dropping to `now` crosses it.
func watchWithTarget(t *testing.T, app *testutil.App, user *testutil.Client, appID int, name string, target float64) {
	t.Helper()
	app.SeedGame(appID, name, []string{"d"}, []string{"p"})
	yesterday := time.Now().AddDate(0, 0, -1)
	app.SeedPriceHistory(appID, []models.PricePoint{{Date: yesterday, Price: target + 10, OriginalPrice: target + 10}})
	if got := user.Put(fmt.Sprintf("/api/me/watchlist/%d", appID), map[string]any{"target_price": target}).Code(); got != http.StatusNoContent {
		t.Fatalf("watch: status = %d", got)
	}
}

// reachTarget records the game's price falling to price today and sends the
// alerts, the way a scrape does.
func reachTarget(t *testing.T, app *testutil.App, fake *testutil.Alerts, appID int, price float64) int {
	t.Helper()
	targets, err := app.DB.CreatePriceDropNotifications(context.Background(), appID, price, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	fake.Notifier.Dispatch(context.Background(), targets)
	return len(targets)
}

func TestTargetPriceAlertsGoOutThroughTheChosenChannels(t *testing.T) {
	app := testutil.NewApp(t)
	fake := app.FakeAlerts(true, true)

	everything := app.NewUser("everything")
	emailOnly := app.NewUser("emailonly")
	discordOnly := app.NewUser("discordonly")
	pushOnly := app.NewUser("pushonly")
	inAppOnly := app.NewUser("inapponly")

	hook := fake.Discord.WebhookURL()
	subscribe(everything, "https://push.example/everything-1")
	subscribe(everything, "https://push.example/everything-2")
	subscribe(pushOnly, "https://push.example/pushonly")
	for u, change := range map[*testutil.Client]map[string]any{
		everything:  {"alert_email": true, "alert_discord": true, "discord_webhook_url": hook, "alert_push": true},
		emailOnly:   {"alert_email": true},
		discordOnly: {"alert_discord": true, "discord_webhook_url": hook},
		pushOnly:    {"alert_push": true},
		inAppOnly:   {},
	} {
		if resp := savePrefs(u, prefs(t, u), change); resp.Code() != http.StatusOK {
			t.Fatalf("save prefs: %d %s", resp.Code(), resp.Body.String())
		}
	}
	const id = 880001
	for _, u := range []*testutil.Client{everything, emailOnly, discordOnly, pushOnly, inAppOnly} {
		watchWithTarget(t, app, u, id, "Target Game", 10)
	}

	if n := reachTarget(t, app, fake, id, 8); n != 5 {
		t.Fatalf("%d notifications created, want one per watcher", n)
	}

	t.Run("email goes only to those who chose it, at their own address", func(t *testing.T) {
		mails := fake.Mail.Sent()
		if len(mails) != 2 {
			t.Fatalf("%d emails, want 2: %+v", len(mails), mails)
		}
		var emails []string
		for _, m := range mails {
			emails = append(emails, m.To)
			if !strings.Contains(m.Subject, "Target Game is now $8.00") {
				t.Errorf("subject = %q", m.Subject)
			}
			if !strings.Contains(m.Body, "/games/880001") || !strings.Contains(m.Body, "/account") {
				t.Errorf("body lacks the game link or the way to turn it off:\n%s", m.Body)
			}
		}
		if !testutil.Contains(emails, "everything") || !testutil.Contains(emails, "emailonly") {
			t.Errorf("sent to %v", emails)
		}
	})

	t.Run("Discord gets a post for each user who chose it", func(t *testing.T) {
		posts := fake.Discord.Posts()
		if len(posts) != 2 {
			t.Fatalf("%d Discord posts, want 2", len(posts))
		}
		for _, p := range posts {
			if !strings.Contains(p, "Target Game is now $8.00") || !strings.Contains(p, "/games/880001") {
				t.Errorf("post = %s", p)
			}
		}
	})

	t.Run("push reaches every browser of each user who chose it", func(t *testing.T) {
		var endpoints []string
		for _, p := range fake.Push.Sent() {
			endpoints = append(endpoints, p.Endpoint)
			if !strings.Contains(p.Payload, `"title":"Target price reached"`) || !strings.Contains(p.Payload, "/games/880001") {
				t.Errorf("payload = %s", p.Payload)
			}
		}
		if len(endpoints) != 3 || !testutil.Contains(endpoints, "everything-1") || !testutil.Contains(endpoints, "everything-2") || !testutil.Contains(endpoints, "pushonly") {
			t.Errorf("push endpoints = %v, want all three browsers", endpoints)
		}
	})

	t.Run("everyone still gets the in-app notification", func(t *testing.T) {
		var n int
		app.Pool.QueryRow(context.Background(), "SELECT count(*) FROM notifications WHERE app_id = $1 AND kind = 'target_price'", id).Scan(&n)
		if n != 5 {
			t.Errorf("%d in-app notifications, want 5", n)
		}
	})

	t.Run("recording the same price again does not alert twice", func(t *testing.T) {
		before := len(fake.Mail.Sent()) + len(fake.Discord.Posts()) + len(fake.Push.Sent())
		if n := reachTarget(t, app, fake, id, 8); n != 0 {
			t.Errorf("%d new notifications on a repeat, want 0", n)
		}
		if after := len(fake.Mail.Sent()) + len(fake.Discord.Posts()) + len(fake.Push.Sent()); after != before {
			t.Errorf("%d more messages were sent on a repeat", after-before)
		}
	})
}

func TestNoAlertsGoOutBeforeTheTargetIsReached(t *testing.T) {
	app := testutil.NewApp(t)
	fake := app.FakeAlerts(true, true)
	user := app.NewUser("early")
	savePrefs(user, prefs(t, user), map[string]any{"alert_email": true})
	watchWithTarget(t, app, user, 880101, "Still Dear", 10)

	if n := reachTarget(t, app, fake, 880101, 12); n != 0 {
		t.Errorf("%d notifications at $12 against a $10 target", n)
	}
	if len(fake.Mail.Sent()) != 0 {
		t.Error("an email went out before the target was reached")
	}
	if n := reachTarget(t, app, fake, 880101, 10); n != 1 {
		t.Errorf("%d notifications exactly at the target, want 1 (at or below counts)", n)
	}
	if len(fake.Mail.Sent()) != 1 {
		t.Errorf("%d emails at the target, want 1", len(fake.Mail.Sent()))
	}
}

func TestAlertsSkipUsersWhoAreOffOrBanned(t *testing.T) {
	app := testutil.NewApp(t)
	fake := app.FakeAlerts(true, true)
	banned := app.NewUser("banned")
	savePrefs(banned, prefs(t, banned), map[string]any{"alert_email": true})
	watchWithTarget(t, app, banned, 880201, "Banned Watch", 10)
	app.SetFlag(banned.UserID, "is_banned", true)

	reachTarget(t, app, fake, 880201, 5)
	if len(fake.Mail.Sent()) != 0 {
		t.Error("a banned user was emailed")
	}
}

func TestAChannelFailingDoesNotStopTheOthers(t *testing.T) {
	app := testutil.NewApp(t)
	fake := app.FakeAlerts(true, true)
	user := app.NewUser("resilient")
	subscribe(user, "https://push.example/resilient")
	savePrefs(user, prefs(t, user), map[string]any{
		"alert_email": true, "alert_discord": true, "discord_webhook_url": fake.Discord.WebhookURL(), "alert_push": true,
	})
	watchWithTarget(t, app, user, 880301, "Resilient Game", 10)

	fake.Mail.Err = testutil.ErrFake
	fake.Discord.SetStatus(http.StatusInternalServerError)
	reachTarget(t, app, fake, 880301, 5)

	if len(fake.Push.Sent()) != 1 {
		t.Errorf("%d push messages, want 1 even though email and Discord failed", len(fake.Push.Sent()))
	}
	// A temporary Discord failure must not switch the channel off.
	if prefs(t, user)["alert_discord"] != true {
		t.Error("Discord alerts were turned off after a server error that may pass")
	}
}

func TestADeadDiscordWebhookTurnsItselfOff(t *testing.T) {
	app := testutil.NewApp(t)
	fake := app.FakeAlerts(true, true)
	user := app.NewUser("deadhook")
	savePrefs(user, prefs(t, user), map[string]any{"alert_discord": true, "discord_webhook_url": fake.Discord.WebhookURL()})
	watchWithTarget(t, app, user, 880401, "Dead Hook Game", 10)

	fake.Discord.SetStatus(http.StatusNotFound) // the webhook was deleted in Discord
	reachTarget(t, app, fake, 880401, 5)

	p := prefs(t, user)
	if p["alert_discord"] != false {
		t.Error("Discord alerts stayed on for a webhook that no longer exists")
	}
	if p["discord_webhook_url"] == "" {
		t.Error("the URL was cleared; it should stay so the user can see and fix it")
	}
}

func TestAPushSubscriptionThatHasGoneIsForgotten(t *testing.T) {
	app := testutil.NewApp(t)
	fake := app.FakeAlerts(true, true)
	user := app.NewUser("gone")
	subscribe(user, "https://push.example/alive")
	subscribe(user, "https://push.example/dead")
	savePrefs(user, prefs(t, user), map[string]any{"alert_push": true})
	watchWithTarget(t, app, user, 880501, "Gone Game", 10)
	fake.Push.Gone["https://push.example/dead"] = true

	reachTarget(t, app, fake, 880501, 5)

	sent := fake.Push.Sent()
	if len(sent) != 1 || sent[0].Endpoint != "https://push.example/alive" {
		t.Errorf("sent = %+v, want just the live browser", sent)
	}
	var endpoints []string
	rows, _ := app.Pool.Query(context.Background(), "SELECT endpoint FROM push_subscriptions WHERE user_id = $1", user.UserID)
	for rows.Next() {
		var e string
		rows.Scan(&e)
		endpoints = append(endpoints, e)
	}
	rows.Close()
	if len(endpoints) != 1 || endpoints[0] != "https://push.example/alive" {
		t.Errorf("subscriptions kept = %v, want only the live one", endpoints)
	}
}

func TestDeletingAUserRemovesTheirPushSubscriptions(t *testing.T) {
	app := testutil.NewApp(t)
	app.FakeAlerts(false, true)
	user := app.NewUser("leaving")
	subscribe(user, "https://push.example/leaving")
	if _, err := app.Pool.Exec(context.Background(), "DELETE FROM users WHERE user_id = $1", user.UserID); err != nil {
		t.Fatal(err)
	}
	var n int
	app.Pool.QueryRow(context.Background(), "SELECT count(*) FROM push_subscriptions WHERE endpoint = 'https://push.example/leaving'").Scan(&n)
	if n != 0 {
		t.Errorf("%d subscriptions left behind", n)
	}
}
