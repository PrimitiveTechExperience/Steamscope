package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PriceHistorySummary identifies the state of a game's recorded price history,
// cheaply, so cached forecasts can be tied to the data they were built from.
type PriceHistorySummary struct {
	Rows      int
	LastDate  time.Time
	LastPrice float64
}

func (db *DB) GetPriceHistorySummary(ctx context.Context, appID int) (PriceHistorySummary, error) {
	var s PriceHistorySummary
	var last *time.Time
	var price *float64
	err := db.Pool.QueryRow(ctx, `
		SELECT count(*), max(recorded_date),
			(SELECT price FROM price_history WHERE app_id = $1 ORDER BY recorded_date DESC LIMIT 1)
		FROM price_history WHERE app_id = $1`, appID).Scan(&s.Rows, &last, &price)
	if err != nil {
		return s, fmt.Errorf("failed to summarise price history for app_id %d: %w", appID, err)
	}
	if last != nil {
		s.LastDate = *last
	}
	if price != nil {
		s.LastPrice = *price
	}
	return s, nil
}

// WatchInfo is how and since when a user has been watching a game.
type WatchInfo struct {
	WatchedAt   time.Time
	TargetPrice *float64
	Pinned      bool
}

// GetWatchInfo returns ErrNotFound if the user isn't watching the game.
func (db *DB) GetWatchInfo(ctx context.Context, userID int64, appID int) (*WatchInfo, error) {
	var w WatchInfo
	err := db.Pool.QueryRow(ctx,
		`SELECT created_at, target_price, pinned FROM watched_games WHERE user_id = $1 AND app_id = $2`,
		userID, appID).Scan(&w.WatchedAt, &w.TargetPrice, &w.Pinned)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get watch info: %w", err)
	}
	return &w, nil
}

// GetPriceOnOrBefore returns the recorded price on a date, or the most recent
// one before it. It returns nil when there is no record that early.
func (db *DB) GetPriceOnOrBefore(ctx context.Context, appID int, date time.Time) (*float64, error) {
	var price float64
	err := db.Pool.QueryRow(ctx, `
		SELECT price FROM price_history
		WHERE app_id = $1 AND recorded_date <= $2::date
		ORDER BY recorded_date DESC LIMIT 1`, appID, date.Format("2006-01-02")).Scan(&price)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get price for app_id %d: %w", appID, err)
	}
	return &price, nil
}
