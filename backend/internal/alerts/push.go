package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
)

// WebPusher sends web push messages signed with the server's VAPID keys.
type WebPusher struct{ cfg config.AlertsConfig }

func NewWebPusher(c config.AlertsConfig) *WebPusher { return &WebPusher{cfg: c} }

func (p *WebPusher) Send(ctx context.Context, sub database.PushSubscription, payload []byte) (bool, error) {
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
	}, &webpush.Options{
		Subscriber:      p.cfg.VAPIDSubject,
		VAPIDPublicKey:  p.cfg.VAPIDPublicKey,
		VAPIDPrivateKey: p.cfg.VAPIDPrivateKey,
		TTL:             24 * 60 * 60,
		Urgency:         webpush.UrgencyHigh,
	})
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		return true, nil // the browser unsubscribed, or the subscription expired
	}
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return false, fmt.Errorf("push service answered %d: %s", resp.StatusCode, b)
	}
	return false, nil
}

// pushPayload is what the service worker receives.
type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// pushOne sends to one browser, forgetting the subscription when the browser has gone.
func (n *Notifier) pushOne(ctx context.Context, userID int64, sub database.PushSubscription, m Message) error {
	payload, err := json.Marshal(pushPayload{Title: m.Title, Body: m.Body, URL: m.URL, Tag: "target-price"})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	gone, err := n.Push.Send(ctx, sub, payload)
	if gone {
		if derr := n.DB.DeletePushSubscriptionByEndpoint(ctx, sub.Endpoint); derr != nil {
			log.Printf("alerts: could not forget push subscription for user %d: %v", userID, derr)
		}
		return fmt.Errorf("that browser has unsubscribed")
	}
	return err
}

func (n *Notifier) sendPush(ctx context.Context, userID int64, sub database.PushSubscription, m Message) {
	if err := n.pushOne(ctx, userID, sub, m); err != nil {
		log.Printf("alerts: push for user %d failed: %v", userID, err)
	}
}
