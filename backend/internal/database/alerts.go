package database

import (
	"context"
	"fmt"
)

// PushSubscription is one browser a user allowed push notifications in.
type PushSubscription struct {
	ID       int64
	Endpoint string
	P256dh   string
	Auth     string
}

// AlertChannels is where one user wants target-price alerts sent.
type AlertChannels struct {
	EmailAddress  string
	Email         bool
	Discord       bool
	DiscordURL    string
	Push          bool
	Subscriptions []PushSubscription
}

// GetAlertChannels loads the alert settings (and push subscriptions) for the
// given users. Banned users are left out: they get no outside alerts.
func (db *DB) GetAlertChannels(ctx context.Context, userIDs []int64) (map[int64]AlertChannels, error) {
	out := map[int64]AlertChannels{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT u.user_id, u.email, p.alert_email, p.alert_discord, p.discord_webhook_url, p.alert_push
		FROM users u JOIN user_preferences p ON p.user_id = u.user_id
		WHERE u.user_id = ANY($1) AND NOT u.is_banned`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load alert settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var c AlertChannels
		if err := rows.Scan(&id, &c.EmailAddress, &c.Email, &c.Discord, &c.DiscordURL, &c.Push); err != nil {
			return nil, err
		}
		out[id] = c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	subs, err := db.Pool.Query(ctx, `
		SELECT user_id, subscription_id, endpoint, p256dh, auth FROM push_subscriptions
		WHERE user_id = ANY($1) ORDER BY subscription_id`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load push subscriptions: %w", err)
	}
	defer subs.Close()
	for subs.Next() {
		var uid int64
		var s PushSubscription
		if err := subs.Scan(&uid, &s.ID, &s.Endpoint, &s.P256dh, &s.Auth); err != nil {
			return nil, err
		}
		if c, ok := out[uid]; ok {
			c.Subscriptions = append(c.Subscriptions, s)
			out[uid] = c
		}
	}
	return out, subs.Err()
}

// SavePushSubscription remembers a browser for the user. The same browser
// (endpoint) signing in as someone else moves to that user.
func (db *DB) SavePushSubscription(ctx context.Context, userID int64, endpoint, p256dh, auth string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth) VALUES ($1, $2, $3, $4)
		ON CONFLICT (endpoint) DO UPDATE SET user_id = EXCLUDED.user_id, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth`,
		userID, endpoint, p256dh, auth)
	if err != nil {
		return fmt.Errorf("failed to save push subscription: %w", err)
	}
	return nil
}

// DeletePushSubscription forgets one of the user's browsers. It reports
// whether there was one to forget.
func (db *DB) DeletePushSubscription(ctx context.Context, userID int64, endpoint string) (bool, error) {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`, userID, endpoint)
	if err != nil {
		return false, fmt.Errorf("failed to delete push subscription: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DeletePushSubscriptionByEndpoint forgets a browser that has gone away.
func (db *DB) DeletePushSubscriptionByEndpoint(ctx context.Context, endpoint string) error {
	if _, err := db.Pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint = $1`, endpoint); err != nil {
		return fmt.Errorf("failed to delete push subscription: %w", err)
	}
	return nil
}

// CountPushSubscriptions is how many browsers the user has subscribed.
func (db *DB) CountPushSubscriptions(ctx context.Context, userID int64) (int, error) {
	var n int
	err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions WHERE user_id = $1`, userID).Scan(&n)
	return n, err
}

// DisableDiscordAlerts turns Discord alerts off for a user whose webhook
// stopped working.
func (db *DB) DisableDiscordAlerts(ctx context.Context, userID int64) error {
	if _, err := db.Pool.Exec(ctx, `UPDATE user_preferences SET alert_discord = false WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("failed to turn off discord alerts: %w", err)
	}
	return nil
}
