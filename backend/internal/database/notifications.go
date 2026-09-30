package database

import (
	"context"
	"fmt"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

const maxNotifications = 50

func (db *DB) CreateNotification(ctx context.Context, userID int64, appID *int, kind, message string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO notifications (user_id, app_id, kind, message) VALUES ($1, $2, $3, $4)`,
		userID, appID, kind, message,
	)
	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}
	return nil
}

func (db *DB) GetNotifications(ctx context.Context, userID int64) ([]models.Notification, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT notification_id, app_id, kind, message, read_at, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2`, userID, maxNotifications)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}
	defer rows.Close()

	notifications := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.NotificationID, &n.AppID, &n.Kind, &n.Message, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

// MarkNotificationsRead marks the given notifications read, or all of the
// user's unread notifications if ids is empty.
func (db *DB) MarkNotificationsRead(ctx context.Context, userID int64, ids []int64) error {
	var err error
	if len(ids) == 0 {
		_, err = db.Pool.Exec(ctx,
			`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID)
	} else {
		_, err = db.Pool.Exec(ctx,
			`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND notification_id = ANY($2) AND read_at IS NULL`,
			userID, ids)
	}
	if err != nil {
		return fmt.Errorf("failed to mark notifications read: %w", err)
	}
	return nil
}

// CreatePriceDropNotifications notifies watchers of appID after a new price
// has been recorded for today:
//   - price_drop: today's price is at least the user's threshold % below the
//     previous recorded price.
//   - target_price: the price just crossed to at/below the user's target
//     (not re-sent every day it stays there).
//
// Both are limited to one per user/game/kind/day.
// recordedDate must be the same date the new price was stored under in
// price_history (UpsertPriceHistory), so "previous price" is computed on the
// scraper's calendar rather than the database's (Postgres CURRENT_DATE is
// UTC, which disagrees with the scraper's local date for part of each day).
func (db *DB) CreatePriceDropNotifications(ctx context.Context, appID int, newPrice float64, recordedDate time.Time) error {
	day := recordedDate.Format("2006-01-02")
	_, err := db.Pool.Exec(ctx, `
		WITH prev AS (
			SELECT price FROM price_history
			WHERE app_id = $1 AND recorded_date < $3::date
			ORDER BY recorded_date DESC
			LIMIT 1
		)
		INSERT INTO notifications (user_id, app_id, kind, message)
		SELECT w.user_id, $1, 'price_drop',
			format('%s dropped from $%s to $%s (-%s%%)',
				g.name, to_char(prev.price, 'FM999990.00'), to_char($2::numeric, 'FM999990.00'),
				round((1 - $2::numeric / prev.price) * 100))
		FROM watched_games w
		JOIN user_preferences p ON p.user_id = w.user_id
		JOIN games g ON g.app_id = w.app_id
		CROSS JOIN prev
		WHERE w.app_id = $1
			AND p.notify_price_drops
			AND prev.price > 0
			AND (1 - $2::numeric / prev.price) * 100 >= p.price_drop_threshold_percent
			AND NOT EXISTS (
				SELECT 1 FROM notifications n
				WHERE n.user_id = w.user_id AND n.app_id = $1 AND n.kind = 'price_drop'
					AND n.created_at > now() - interval '20 hours'
			)`,
		appID, newPrice, day,
	)
	if err != nil {
		return fmt.Errorf("failed to create price drop notifications: %w", err)
	}

	_, err = db.Pool.Exec(ctx, `
		WITH prev AS (
			SELECT price FROM price_history
			WHERE app_id = $1 AND recorded_date < $3::date
			ORDER BY recorded_date DESC
			LIMIT 1
		)
		INSERT INTO notifications (user_id, app_id, kind, message)
		SELECT w.user_id, $1, 'target_price',
			format('%s is now $%s - at or below your target of $%s',
				g.name, to_char($2::numeric, 'FM999990.00'), to_char(w.target_price, 'FM999990.00'))
		FROM watched_games w
		JOIN user_preferences p ON p.user_id = w.user_id
		JOIN games g ON g.app_id = w.app_id
		LEFT JOIN prev ON true
		WHERE w.app_id = $1
			AND p.notify_price_drops
			AND w.target_price IS NOT NULL
			AND $2::numeric <= w.target_price
			AND (prev.price IS NULL OR prev.price > w.target_price)
			AND NOT EXISTS (
				SELECT 1 FROM notifications n
				WHERE n.user_id = w.user_id AND n.app_id = $1 AND n.kind = 'target_price'
					AND n.created_at > now() - interval '20 hours'
			)`,
		appID, newPrice, day,
	)
	if err != nil {
		return fmt.Errorf("failed to create target price notifications: %w", err)
	}
	return nil
}
