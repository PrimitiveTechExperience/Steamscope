package database

import (
	"context"
	"fmt"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

func (db *DB) GetPreferences(ctx context.Context, userID int64) (*models.Preferences, error) {
	var p models.Preferences
	err := db.Pool.QueryRow(ctx, `
		SELECT theme, notify_price_drops, price_drop_threshold_percent, preferred_genres,
			alert_email, alert_discord, discord_webhook_url, alert_push
		FROM user_preferences WHERE user_id = $1`, userID,
	).Scan(&p.Theme, &p.NotifyPriceDrops, &p.PriceDropThresholdPercent, &p.PreferredGenres,
		&p.AlertEmail, &p.AlertDiscord, &p.DiscordWebhookURL, &p.AlertPush)
	if err != nil {
		return nil, fmt.Errorf("failed to get preferences: %w", err)
	}
	return &p, nil
}

func (db *DB) UpdatePreferences(ctx context.Context, userID int64, p models.Preferences) error {
	if p.PreferredGenres == nil {
		p.PreferredGenres = []string{}
	}
	_, err := db.Pool.Exec(ctx, `
		UPDATE user_preferences
		SET theme = $2, notify_price_drops = $3, price_drop_threshold_percent = $4, preferred_genres = $5,
			alert_email = $6, alert_discord = $7, discord_webhook_url = $8, alert_push = $9
		WHERE user_id = $1`,
		userID, p.Theme, p.NotifyPriceDrops, p.PriceDropThresholdPercent, p.PreferredGenres,
		p.AlertEmail, p.AlertDiscord, p.DiscordWebhookURL, p.AlertPush,
	)
	if err != nil {
		return fmt.Errorf("failed to update preferences: %w", err)
	}
	return nil
}
