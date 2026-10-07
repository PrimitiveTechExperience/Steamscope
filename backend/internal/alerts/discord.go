package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// ErrChannelUnavailable means the server has no configuration for the channel.
var ErrChannelUnavailable = errors.New("that channel is not set up on this server")

var webhookPath = regexp.MustCompile(`^/api/(?:v\d+/)?webhooks/\d{5,25}/[A-Za-z0-9_-]{10,200}$`)

func isDiscordHost(host string) bool {
	host = strings.ToLower(host)
	return host == "discord.com" || host == "discordapp.com" || host == "canary.discord.com" || host == "ptb.discord.com"
}

// webhookRules say what a webhook URL may look like. The defaults accept only
// Discord's own https addresses; tests loosen them to reach a local server.
type webhookRules struct {
	hostOK    func(host string) bool
	allowHTTP bool
}

var defaultWebhookRules = webhookRules{hostOK: isDiscordHost}

// ValidateDiscordWebhook accepts only real Discord webhook URLs. Users supply
// the address and the server sends requests to it, so anything else (an
// internal address, a lookalike host, a plain-http URL) must be refused.
func ValidateDiscordWebhook(raw string) error {
	return validateWebhook(raw, defaultWebhookRules)
}

func validateWebhook(raw string, rules webhookRules) error {
	if len(raw) > 300 {
		return errors.New("that webhook URL is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("that is not a valid URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && rules.allowHTTP) {
		return errors.New("a Discord webhook URL starts with https://")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("that does not look like a Discord webhook URL")
	}
	const hint = "that is not a Discord webhook URL (it should look like https://discord.com/api/webhooks/123/token)"
	if !rules.hostOK(u.Hostname()) {
		return errors.New(hint)
	}
	if !rules.allowHTTP && u.Port() != "" && u.Port() != "443" {
		return errors.New(hint)
	}
	if !webhookPath.MatchString(u.Path) {
		return errors.New(hint)
	}
	return nil
}

// WebhookError is a failed webhook call; Status is Discord's response code.
type WebhookError struct {
	Status int
	Body   string
}

func (e *WebhookError) Error() string {
	return fmt.Sprintf("discord answered %d: %s", e.Status, e.Body)
}

// IsWebhookGone is true when Discord says the webhook no longer exists or is
// not allowed, so sending again would never work.
func IsWebhookGone(err error) bool {
	var we *WebhookError
	return errors.As(err, &we) && (we.Status == http.StatusNotFound || we.Status == http.StatusUnauthorized || we.Status == http.StatusForbidden)
}

func (n *Notifier) sendDiscord(ctx context.Context, webhookURL string, m Message) error {
	if err := validateWebhook(webhookURL, n.webhookRules()); err != nil {
		return err
	}
	payload := map[string]any{
		"username": "Steamscope",
		// No @mentions, whatever the message contains.
		"allowed_mentions": map[string]any{"parse": []string{}},
		"embeds": []map[string]any{{
			"title":       m.Title,
			"description": truncate(m.Body, 1500),
			"url":         m.URL,
			"color":       0xFF3D1F,
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.HTTP.Do(req)
	if err != nil {
		// The URL holds the webhook token; report only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("could not reach Discord: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return &WebhookError{Status: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	return nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// ValidateWebhook checks a webhook URL against this notifier's rules.
func (n *Notifier) ValidateWebhook(raw string) error { return validateWebhook(raw, n.webhookRules()) }

func (n *Notifier) webhookRules() webhookRules {
	rules := defaultWebhookRules
	if n.DiscordHostOK != nil {
		rules.hostOK = n.DiscordHostOK
	}
	rules.allowHTTP = n.AllowInsecureDiscord
	return rules
}
