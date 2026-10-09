// Package alerts sends target-price alerts to the places a user asked for them:
// email, a Discord webhook and browser push notifications. The in-app
// notification is created separately; this is the extra delivery.
package alerts

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
)

// sendTimeout bounds every outgoing call, so one slow service cannot hold up a scrape.
const sendTimeout = 10 * time.Second

// Message is what gets delivered.
type Message struct {
	// Subject is the email subject; when empty one is made from the body.
	Subject string
	Title   string
	Body    string
	// URL is where the user should land: the game's page.
	URL string
}

// Mailer sends an email.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Pusher sends one web push message. gone is true when the browser has
// unsubscribed for good, so the subscription should be forgotten.
type Pusher interface {
	Send(ctx context.Context, sub database.PushSubscription, payload []byte) (gone bool, err error)
}

// Availability says which channels this server can send through.
type Availability struct {
	Email          bool   `json:"email"`
	Discord        bool   `json:"discord"`
	Push           bool   `json:"push"`
	VAPIDPublicKey string `json:"vapid_public_key"`
}

// Notifier delivers alerts. Its fields are exported so tests can swap parts.
type Notifier struct {
	DB          *database.DB
	FrontendURL string
	Mail        Mailer // nil when email is not configured
	Push        Pusher // nil when web push is not configured
	HTTP        *http.Client
	// DiscordHostOK decides which webhook hosts are acceptable, and
	// AllowInsecureDiscord permits plain http; both exist so tests can reach a local server.
	DiscordHostOK        func(host string) bool
	AllowInsecureDiscord bool

	vapidPublic string
}

// New builds a Notifier from the configuration. Channels that are not
// configured are left out.
func New(db *database.DB, cfg *config.Config) *Notifier {
	n := &Notifier{
		DB:            db,
		FrontendURL:   cfg.FrontendURL,
		HTTP:          &http.Client{Timeout: sendTimeout},
		DiscordHostOK: isDiscordHost,
	}
	if cfg.Alerts.EmailConfigured() {
		n.Mail = NewSMTPMailer(cfg.Alerts)
	}
	if cfg.Alerts.PushConfigured() {
		n.Push = NewWebPusher(cfg.Alerts)
		n.vapidPublic = cfg.Alerts.VAPIDPublicKey
	}
	return n
}

// Available reports which channels can be used.
func (n *Notifier) Available() Availability {
	return Availability{Email: n.Mail != nil, Discord: true, Push: n.Push != nil, VAPIDPublicKey: n.vapidPublic}
}

// SetVAPIDPublicKey lets a replaced Pusher still advertise a key.
func (n *Notifier) SetVAPIDPublicKey(key string) { n.vapidPublic = key }

// Dispatch sends each alert through every channel its user has turned on. A
// failure in one channel is logged and never stops the others, or the scrape.
func (n *Notifier) Dispatch(ctx context.Context, targets []database.TargetAlert) {
	if len(targets) == 0 {
		return
	}
	ids := make([]int64, 0, len(targets))
	for _, t := range targets {
		ids = append(ids, t.UserID)
	}
	channels, err := n.DB.GetAlertChannels(ctx, ids)
	if err != nil {
		log.Printf("alerts: could not load channels: %v", err)
		return
	}
	for _, t := range targets {
		c, ok := channels[t.UserID]
		if !ok {
			continue
		}
		n.deliver(ctx, t.UserID, c, n.messageFor(t))
	}
}

func (n *Notifier) messageFor(t database.TargetAlert) Message {
	return Message{
		Title: "Target price reached",
		Body:  t.Message,
		URL:   fmt.Sprintf("%s/games/%d", strings.TrimRight(n.FrontendURL, "/"), t.AppID),
	}
}

// deliver sends one message through whichever of the user's channels are on.
func (n *Notifier) deliver(ctx context.Context, userID int64, c database.AlertChannels, m Message) {
	if c.Email && n.Mail != nil && c.EmailAddress != "" {
		if err := n.sendEmail(ctx, c.EmailAddress, m); err != nil {
			log.Printf("alerts: email to user %d failed: %v", userID, err)
		}
	}
	if c.Discord && c.DiscordURL != "" {
		if err := n.sendDiscord(ctx, c.DiscordURL, m); err != nil {
			log.Printf("alerts: discord for user %d failed: %v", userID, err)
			if IsWebhookGone(err) {
				// Discord says the webhook no longer exists: stop trying, and let the
				// settings page show the box unticked.
				if derr := n.DB.DisableDiscordAlerts(ctx, userID); derr != nil {
					log.Printf("alerts: could not turn off discord for user %d: %v", userID, derr)
				}
			}
		}
	}
	if c.Push && n.Push != nil {
		for _, sub := range c.Subscriptions {
			n.sendPush(ctx, userID, sub, m)
		}
	}
}

// Test sends a sample alert through one channel for the user, so they can see
// it works. discordURL overrides the saved webhook (so an unsaved one can be tried).
func (n *Notifier) Test(ctx context.Context, userID int64, channel, discordURL string) error {
	channels, err := n.DB.GetAlertChannels(ctx, []int64{userID})
	if err != nil {
		return err
	}
	c := channels[userID]
	m := Message{
		Subject: "Steamscope test alert",
		Title:   "Test alert from Steamscope",
		Body:    "This is what a target-price alert looks like. You will get one when a game you watch reaches your target.",
		URL:     strings.TrimRight(n.FrontendURL, "/") + "/account",
	}
	switch channel {
	case "email":
		if n.Mail == nil {
			return ErrChannelUnavailable
		}
		if c.EmailAddress == "" {
			return fmt.Errorf("no email address on file")
		}
		return n.sendEmail(ctx, c.EmailAddress, m)
	case "discord":
		url := discordURL
		if url == "" {
			url = c.DiscordURL
		}
		if url == "" {
			return fmt.Errorf("no Discord webhook set")
		}
		return n.sendDiscord(ctx, url, m)
	case "push":
		if n.Push == nil {
			return ErrChannelUnavailable
		}
		if len(c.Subscriptions) == 0 {
			return fmt.Errorf("no browser is subscribed yet")
		}
		var sent int
		var lastErr error
		for _, sub := range c.Subscriptions {
			if err := n.pushOne(ctx, userID, sub, m); err != nil {
				lastErr = err
				continue
			}
			sent++
		}
		if sent == 0 {
			return lastErr
		}
		return nil
	}
	return fmt.Errorf("unknown channel %q", channel)
}
