package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/alerts"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/cache"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

const (
	// A user rarely has more than a few browsers; this stops the table being filled.
	maxPushSubscriptions = 10
	// Test sends reach outside services, so they are limited.
	alertTestLimit  = 10
	alertTestWindow = time.Hour
)

// GetAlertChannels says which ways of sending an alert this server supports,
// so the settings page only offers what works.
func (h *Handler) GetAlertChannels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Alerts.Available())
}

type pushSubscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// SavePushSubscription remembers the browser the user just allowed push
// notifications in.
func (h *Handler) SavePushSubscription(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	if !h.Alerts.Available().Push {
		writeError(w, http.StatusServiceUnavailable, "push notifications are not set up on this server")
		return
	}
	var req pushSubscriptionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := url.Parse(req.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || len(req.Endpoint) > 800 {
		writeError(w, http.StatusBadRequest, "invalid push subscription")
		return
	}
	if req.Keys.P256dh == "" || req.Keys.Auth == "" || len(req.Keys.P256dh) > 200 || len(req.Keys.Auth) > 100 {
		writeError(w, http.StatusBadRequest, "invalid push subscription")
		return
	}
	n, err := h.DB.CountPushSubscriptions(r.Context(), user.UserID)
	if err != nil {
		serverError(w, r, "count push subscriptions", err, "failed to save the subscription")
		return
	}
	if n >= maxPushSubscriptions {
		writeError(w, http.StatusBadRequest, "you have too many browsers subscribed; remove one first")
		return
	}
	if err := h.DB.SavePushSubscription(r.Context(), user.UserID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth); err != nil {
		serverError(w, r, "save push subscription", err, "failed to save the subscription")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

// DeletePushSubscription forgets a browser. When it was the last one, push
// alerts are turned off, since there is nowhere left to send them.
func (h *Handler) DeletePushSubscription(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	var req pushUnsubscribeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if _, err := h.DB.DeletePushSubscription(r.Context(), user.UserID, req.Endpoint); err != nil {
		serverError(w, r, "delete push subscription", err, "failed to remove the subscription")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type alertTestRequest struct {
	Channel string `json:"channel"`
	// DiscordWebhookURL tries a webhook that has not been saved yet.
	DiscordWebhookURL string `json:"discord_webhook_url"`
}

// SendTestAlert sends a sample alert through one channel so the user can see
// it works before relying on it.
func (h *Handler) SendTestAlert(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	var req alertTestRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	switch req.Channel {
	case "email", "discord", "push":
	default:
		writeError(w, http.StatusBadRequest, "channel must be email, discord or push")
		return
	}
	if req.Channel == "discord" && req.DiscordWebhookURL != "" {
		if err := h.Alerts.ValidateWebhook(req.DiscordWebhookURL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if !cache.Allow(r.Context(), h.Redis, "ratelimit:alerttest:"+strconv.FormatInt(user.UserID, 10), alertTestLimit, alertTestWindow) {
		writeError(w, http.StatusTooManyRequests, "you have sent a lot of test alerts, try again in a while")
		return
	}

	err := h.Alerts.Test(r.Context(), user.UserID, req.Channel, req.DiscordWebhookURL)
	var webhookErr *alerts.WebhookError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, alerts.ErrChannelUnavailable):
		writeError(w, http.StatusServiceUnavailable, "that channel is not set up on this server")
	case errors.As(err, &webhookErr):
		writeError(w, http.StatusBadGateway, "Discord rejected the webhook (it answered "+strconv.Itoa(webhookErr.Status)+"). Check the URL.")
	default:
		// What failed goes to the log, with the request ID; the user gets a plain message.
		serverErrorWithStatus(w, r, http.StatusBadGateway, "send test alert "+req.Channel, err, testAlertMessage(req.Channel, err))
	}
}

// testAlertMessage says what went wrong in terms the user can act on.
func testAlertMessage(channel string, err error) string {
	switch err.Error() {
	case "no email address on file", "no Discord webhook set", "no browser is subscribed yet":
		return "Nothing to send to yet: " + err.Error() + "."
	}
	switch channel {
	case "email":
		return "The mail server could not send the email."
	case "push":
		return "The browser's push service could not deliver it."
	}
	return "The test alert could not be sent."
}

// validateAlertPreferences checks the alert part of a preferences update.
// Turning a channel on is only allowed when it can work; leaving one that is
// already on alone is always fine, so saving the theme never fails because an
// unrelated channel was later switched off on the server.
func (h *Handler) validateAlertPreferences(r *http.Request, userID int64, next models.Preferences, current *models.Preferences) (string, bool) {
	if next.DiscordWebhookURL != "" {
		if err := h.Alerts.ValidateWebhook(next.DiscordWebhookURL); err != nil {
			return err.Error(), false
		}
	}
	if next.AlertDiscord && next.DiscordWebhookURL == "" {
		return "add your Discord webhook URL first", false
	}
	available := h.Alerts.Available()
	if next.AlertEmail && !current.AlertEmail && !available.Email {
		return "email alerts are not set up on this server", false
	}
	if next.AlertPush && !current.AlertPush {
		if !available.Push {
			return "push notifications are not set up on this server", false
		}
		n, err := h.DB.CountPushSubscriptions(r.Context(), userID)
		if err != nil || n == 0 {
			return "allow notifications in this browser first", false
		}
	}
	return "", true
}
