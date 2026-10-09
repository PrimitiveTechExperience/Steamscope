package testutil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/alerts"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
)

// SentMail is an email the fake mail server was asked to send.
type SentMail struct{ To, Subject, Body string }

// FakeMailer records emails instead of sending them.
type FakeMailer struct {
	mu   sync.Mutex
	sent []SentMail
	// Err, when set, makes every send fail.
	Err error
}

func (m *FakeMailer) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.sent = append(m.sent, SentMail{to, subject, body})
	return nil
}

// Sent returns the emails sent so far.
func (m *FakeMailer) Sent() []SentMail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]SentMail(nil), m.sent...)
}

// SentPush is a push message the fake push service received.
type SentPush struct {
	Endpoint string
	Payload  string
}

// FakePusher records push messages instead of sending them.
type FakePusher struct {
	mu   sync.Mutex
	sent []SentPush
	// Gone lists endpoints whose browser has unsubscribed.
	Gone map[string]bool
	// Err, when set, makes every send fail.
	Err error
}

func (p *FakePusher) Send(_ context.Context, sub database.PushSubscription, payload []byte) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Gone[sub.Endpoint] {
		return true, nil
	}
	if p.Err != nil {
		return false, p.Err
	}
	p.sent = append(p.sent, SentPush{sub.Endpoint, string(payload)})
	return false, nil
}

// Sent returns the push messages sent so far.
func (p *FakePusher) Sent() []SentPush {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]SentPush(nil), p.sent...)
}

// FakeDiscord is a local stand-in for a Discord webhook.
type FakeDiscord struct {
	srv    *httptest.Server
	mu     sync.Mutex
	posts  []string
	status int
}

// WebhookURL is a URL shaped like a real webhook that reaches this fake.
func (d *FakeDiscord) WebhookURL() string {
	return d.srv.URL + "/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz-ABC_123"
}

// Posts returns the request bodies received.
func (d *FakeDiscord) Posts() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.posts...)
}

// SetStatus makes the webhook answer with that status from now on.
func (d *FakeDiscord) SetStatus(status int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = status
}

// Alerts is the fake alert channels installed by FakeAlerts.
type Alerts struct {
	Mail    *FakeMailer
	Push    *FakePusher
	Discord *FakeDiscord
	// Notifier is what the handlers and the tests dispatch through.
	Notifier *alerts.Notifier
}

// FakeAlerts installs working fake alert channels: an email sender, a push
// service and a Discord webhook. Pass false to leave the email or push channel
// unconfigured, as it is on a server with no SMTP or VAPID settings.
func (a *App) FakeAlerts(email, push bool) *Alerts {
	a.T.Helper()
	d := &FakeDiscord{status: http.StatusNoContent}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		d.mu.Lock()
		d.posts = append(d.posts, string(body))
		status := d.status
		d.mu.Unlock()
		if status != http.StatusNoContent {
			http.Error(w, "nope", status)
			return
		}
		w.WriteHeader(status)
	}))
	a.T.Cleanup(d.srv.Close)

	f := &Alerts{Discord: d, Mail: &FakeMailer{}, Push: &FakePusher{Gone: map[string]bool{}}}
	n := &alerts.Notifier{
		DB:                   a.DB,
		FrontendURL:          FrontendURL,
		HTTP:                 d.srv.Client(),
		DiscordHostOK:        func(host string) bool { return host == "127.0.0.1" },
		AllowInsecureDiscord: true,
	}
	if email {
		n.Mail = f.Mail
	}
	if push {
		n.Push = f.Push
		n.SetVAPIDPublicKey("test-vapid-public-key")
	}
	f.Notifier = n
	a.H.Alerts = n
	return f
}

// ErrFake is a convenient failure for tests.
var ErrFake = errors.New("fake failure")

// Contains reports whether any of the strings contains sub.
func Contains(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
