package alerts

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
)

func TestValidateDiscordWebhook(t *testing.T) {
	const token = "abcdefghijklmnopqrstuvwxyz-ABC_123"
	good := []string{
		"https://discord.com/api/webhooks/123456789012345678/" + token,
		"https://discordapp.com/api/webhooks/123456789012345678/" + token,
		"https://canary.discord.com/api/webhooks/123456789012345678/" + token,
		"https://DISCORD.com/api/webhooks/123456789012345678/" + token,
		"https://discord.com/api/v10/webhooks/123456789012345678/" + token,
		"https://discord.com:443/api/webhooks/123456789012345678/" + token,
	}
	for _, u := range good {
		if err := ValidateDiscordWebhook(u); err != nil {
			t.Errorf("%q rejected: %v", u, err)
		}
	}
	bad := map[string]string{
		"empty":             "",
		"not a url":         "hello",
		"plain http":        "http://discord.com/api/webhooks/123456789012345678/" + token,
		"other host":        "https://example.com/api/webhooks/123456789012345678/" + token,
		"lookalike suffix":  "https://discord.com.evil.example/api/webhooks/123456789012345678/" + token,
		"lookalike prefix":  "https://evildiscord.com/api/webhooks/123456789012345678/" + token,
		"userinfo trick":    "https://discord.com@evil.example/api/webhooks/123456789012345678/" + token,
		"internal address":  "https://127.0.0.1/api/webhooks/123456789012345678/" + token,
		"metadata address":  "https://169.254.169.254/api/webhooks/123456789012345678/" + token,
		"localhost":         "https://localhost/api/webhooks/123456789012345678/" + token,
		"odd port":          "https://discord.com:8443/api/webhooks/123456789012345678/" + token,
		"wrong path":        "https://discord.com/api/channels/123456789012345678/" + token,
		"missing token":     "https://discord.com/api/webhooks/123456789012345678",
		"short token":       "https://discord.com/api/webhooks/123456789012345678/abc",
		"non numeric id":    "https://discord.com/api/webhooks/abcdefghij/" + token,
		"path traversal":    "https://discord.com/api/webhooks/123456789012345678/../../x" + token,
		"query string":      "https://discord.com/api/webhooks/123456789012345678/" + token + "?x=1",
		"fragment":          "https://discord.com/api/webhooks/123456789012345678/" + token + "#x",
		"trailing path":     "https://discord.com/api/webhooks/123456789012345678/" + token + "/extra",
		"javascript scheme": "javascript:alert(1)",
		"file scheme":       "file:///etc/passwd",
		"too long":          "https://discord.com/api/webhooks/123456789012345678/" + strings.Repeat("a", 400),
	}
	for name, u := range bad {
		if err := ValidateDiscordWebhook(u); err == nil {
			t.Errorf("%s: %q was accepted", name, u)
		}
	}
}

func TestBuildMessageCannotBeInjectedInto(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	t.Run("a normal message", func(t *testing.T) {
		msg, sender, rcpt, err := buildMessage("Steamscope <alerts@example.com>", "ann@example.com", "Hades is now $9.99", "line one\nline two", now)
		if err != nil {
			t.Fatal(err)
		}
		s := string(msg)
		if sender != "alerts@example.com" || rcpt != "ann@example.com" {
			t.Errorf("envelope = %q -> %q", sender, rcpt)
		}
		for _, want := range []string{"From: \"Steamscope\" <alerts@example.com>\r\n", "To: <ann@example.com>\r\n", "Subject: Hades is now $9.99\r\n", "MIME-Version: 1.0\r\n", "Content-Type: text/plain; charset=UTF-8\r\n", "\r\n\r\nline one\r\nline two\r\n"} {
			if !strings.Contains(s, want) {
				t.Errorf("message lacks %q:\n%s", want, s)
			}
		}
	})

	t.Run("line breaks in the subject cannot add headers", func(t *testing.T) {
		msg, _, _, err := buildMessage("a@example.com", "b@example.com", "Hello\r\nBcc: attacker@evil.example\r\nX-Evil: 1", "body", now)
		if err != nil {
			t.Fatal(err)
		}
		head := strings.SplitN(string(msg), "\r\n\r\n", 2)[0]
		for _, line := range strings.Split(head, "\r\n") {
			if strings.HasPrefix(line, "Bcc:") || strings.HasPrefix(line, "X-Evil:") {
				t.Errorf("an injected header got through: %q", line)
			}
		}
	})

	t.Run("a recipient with extra addresses or line breaks is refused", func(t *testing.T) {
		for _, to := range []string{"b@example.com\r\nBcc: x@evil.example", "b@example.com, x@evil.example", "not an address", ""} {
			if _, _, _, err := buildMessage("a@example.com", to, "s", "b", now); err == nil {
				t.Errorf("recipient %q was accepted", to)
			}
		}
	})

	t.Run("a bad sender is refused", func(t *testing.T) {
		if _, _, _, err := buildMessage("nonsense", "b@example.com", "s", "b", now); err == nil {
			t.Error("a sender that is not an address was accepted")
		}
	})

	t.Run("non-ASCII subjects are encoded", func(t *testing.T) {
		msg, _, _, _ := buildMessage("a@example.com", "b@example.com", "Pokémon ist jetzt günstig", "b", now)
		if !strings.Contains(string(msg), "Subject: =?utf-8?q?") {
			t.Errorf("subject is not encoded:\n%s", msg)
		}
	})
}

// fakeSMTP is just enough of an SMTP server to take one message.
type fakeSMTP struct {
	ln       net.Listener
	mu       sync.Mutex
	from, to string
	data     string
	rejectTo bool
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *fakeSMTP) port() int { return s.ln.Addr().(*net.TCPAddr).Port }

func (s *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	say := func(line string) { io.WriteString(c, line+"\r\n") }
	say("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			say("250 fake")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			s.mu.Lock()
			s.from = strings.TrimSpace(line[len("MAIL FROM:"):])
			s.mu.Unlock()
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			if s.rejectTo {
				say("550 no such user")
				continue
			}
			s.mu.Lock()
			s.to = strings.TrimSpace(line[len("RCPT TO:"):])
			s.mu.Unlock()
			say("250 ok")
		case cmd == "DATA":
			say("354 go ahead")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			s.mu.Lock()
			s.data = b.String()
			s.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func TestSMTPMailerSendsAMessage(t *testing.T) {
	srv := newFakeSMTP(t)
	m := NewSMTPMailer(config.AlertsConfig{SMTPHost: "127.0.0.1", SMTPPort: srv.port(), SMTPFrom: "Steamscope <alerts@example.com>"})
	if err := m.Send(context.Background(), "ann@example.com", "Hades is now $9.99", "Go and see.\nSoon."); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(srv.from, "alerts@example.com") || !strings.Contains(srv.to, "ann@example.com") {
		t.Errorf("envelope from=%q to=%q", srv.from, srv.to)
	}
	if !strings.Contains(srv.data, "Subject: Hades is now $9.99") || !strings.Contains(srv.data, "Go and see.\r\nSoon.") {
		t.Errorf("message = %q", srv.data)
	}
}

func TestSMTPMailerReportsFailures(t *testing.T) {
	t.Run("a rejected recipient", func(t *testing.T) {
		srv := newFakeSMTP(t)
		srv.rejectTo = true
		m := NewSMTPMailer(config.AlertsConfig{SMTPHost: "127.0.0.1", SMTPPort: srv.port(), SMTPFrom: "a@example.com"})
		err := m.Send(context.Background(), "ghost@example.com", "s", "b")
		if err == nil || !strings.Contains(err.Error(), "recipient rejected") {
			t.Errorf("err = %v, want a rejected recipient", err)
		}
	})

	t.Run("no mail server", func(t *testing.T) {
		ln, _ := net.Listen("tcp", "127.0.0.1:0")
		port := ln.Addr().(*net.TCPAddr).Port
		ln.Close() // nothing listens there now
		m := NewSMTPMailer(config.AlertsConfig{SMTPHost: "127.0.0.1", SMTPPort: port, SMTPFrom: "a@example.com"})
		if err := m.Send(context.Background(), "b@example.com", "s", "b"); err == nil || !strings.Contains(err.Error(), "could not connect") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("an invalid recipient never reaches the server", func(t *testing.T) {
		srv := newFakeSMTP(t)
		m := NewSMTPMailer(config.AlertsConfig{SMTPHost: "127.0.0.1", SMTPPort: srv.port(), SMTPFrom: "a@example.com"})
		if err := m.Send(context.Background(), "b@example.com\r\nRCPT TO:<x@evil.example>", "s", "b"); err == nil {
			t.Error("a recipient with a line break was accepted")
		}
		srv.mu.Lock()
		defer srv.mu.Unlock()
		if srv.to != "" {
			t.Errorf("the server was given recipient %q", srv.to)
		}
	})
}

func TestChannelAvailabilityFollowsConfiguration(t *testing.T) {
	none := config.AlertsConfig{}
	if none.EmailConfigured() || none.PushConfigured() {
		t.Error("an empty configuration should have no channels")
	}
	if (config.AlertsConfig{SMTPHost: "smtp.example.com"}).EmailConfigured() {
		t.Error("email needs a sender address as well as a host")
	}
	if !(config.AlertsConfig{SMTPHost: "smtp.example.com", SMTPFrom: "a@example.com"}).EmailConfigured() {
		t.Error("host and sender should be enough for email")
	}
	if (config.AlertsConfig{VAPIDPublicKey: "x"}).PushConfigured() {
		t.Error("push needs both VAPID keys")
	}
	if !(config.AlertsConfig{VAPIDPublicKey: "x", VAPIDPrivateKey: "y"}).PushConfigured() {
		t.Error("both VAPID keys should be enough for push")
	}
}

func testNotifier(srv *httptest.Server) *Notifier {
	return &Notifier{
		HTTP:                 srv.Client(),
		DiscordHostOK:        func(h string) bool { return h == "127.0.0.1" },
		AllowInsecureDiscord: true,
	}
}

func webhookAt(srv *httptest.Server) string {
	return srv.URL + "/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz-ABC_123"
}

func TestSendDiscord(t *testing.T) {
	var got map[string]any
	var contentType string
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		json.NewDecoder(r.Body).Decode(&got)
		if status != http.StatusNoContent {
			http.Error(w, "unknown webhook", status)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	n := testNotifier(srv)

	t.Run("posts an embed with a link, and never pings anyone", func(t *testing.T) {
		m := Message{Title: "Target price reached", Body: "Hades is now $9.99 - @everyone", URL: "https://site.example/games/1145360"}
		if err := n.sendDiscord(context.Background(), webhookAt(srv), m); err != nil {
			t.Fatal(err)
		}
		if contentType != "application/json" {
			t.Errorf("Content-Type = %q", contentType)
		}
		embeds := got["embeds"].([]any)
		e := embeds[0].(map[string]any)
		if e["title"] != "Target price reached" || e["url"] != "https://site.example/games/1145360" || !strings.Contains(e["description"].(string), "Hades is now $9.99") {
			t.Errorf("embed = %v", e)
		}
		mentions := got["allowed_mentions"].(map[string]any)["parse"].([]any)
		if len(mentions) != 0 {
			t.Errorf("allowed_mentions.parse = %v, want none so @everyone does nothing", mentions)
		}
	})

	t.Run("a very long message is cut to fit", func(t *testing.T) {
		n.sendDiscord(context.Background(), webhookAt(srv), Message{Title: "t", Body: strings.Repeat("x", 5000), URL: "u"})
		d := got["embeds"].([]any)[0].(map[string]any)["description"].(string)
		if len([]rune(d)) > 1500 {
			t.Errorf("description has %d characters", len([]rune(d)))
		}
	})

	t.Run("a webhook Discord no longer knows is reported as gone", func(t *testing.T) {
		status = http.StatusNotFound
		err := n.sendDiscord(context.Background(), webhookAt(srv), Message{Title: "t"})
		var we *WebhookError
		if !errors.As(err, &we) || we.Status != 404 || !IsWebhookGone(err) {
			t.Errorf("err = %v, want a 404 webhook error that counts as gone", err)
		}
	})

	t.Run("other failures are not treated as gone", func(t *testing.T) {
		for _, s := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway} {
			status = s
			err := n.sendDiscord(context.Background(), webhookAt(srv), Message{Title: "t"})
			if err == nil || IsWebhookGone(err) {
				t.Errorf("status %d: err = %v, want a failure that is not 'gone'", s, err)
			}
		}
		if IsWebhookGone(errors.New("network down")) || IsWebhookGone(nil) {
			t.Error("an unrelated error counted as gone")
		}
	})

	t.Run("an unreachable Discord never puts the webhook token in the error", func(t *testing.T) {
		dead := httptest.NewServer(http.NotFoundHandler())
		url := webhookAt(dead)
		dead.Close()
		err := testNotifier(dead).sendDiscord(context.Background(), url, Message{Title: "t"})
		if err == nil {
			t.Fatal("expected a connection error")
		}
		if strings.Contains(err.Error(), "abcdefghijklmnopqrstuvwxyz") {
			t.Errorf("the token leaked into the error: %v", err)
		}
	})

	t.Run("refuses an address that is not a webhook before sending anything", func(t *testing.T) {
		calls := 0
		probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
		defer probe.Close()
		strict := &Notifier{HTTP: probe.Client()} // the real rules: Discord's https hosts only
		if err := strict.sendDiscord(context.Background(), probe.URL+"/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz", Message{Title: "t"}); err == nil {
			t.Error("a local address was accepted")
		}
		if calls != 0 {
			t.Errorf("%d requests reached a server that is not Discord", calls)
		}
	})
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncate("abcdefghij", 5); got != "abcd…" {
		t.Errorf("got %q", got)
	}
	if got := truncate("日本語のテキスト", 4); got != "日本語…" {
		t.Errorf("multibyte: got %q", got)
	}
}
