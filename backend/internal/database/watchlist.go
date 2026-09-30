package database

import (
	"context"
	"fmt"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

// GetWatchlist returns a user's watched games, pinned first, then most
// recently watched.
func (db *DB) GetWatchlist(ctx context.Context, userID int64) ([]models.WatchedGame, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+gameCardColumns+`, w.pinned, w.target_price, w.created_at
		FROM watched_games w
		JOIN games g ON g.app_id = w.app_id
		WHERE w.user_id = $1
		ORDER BY w.pinned DESC, w.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get watchlist: %w", err)
	}
	defer rows.Close()

	watchlist := []models.WatchedGame{}
	for rows.Next() {
		var w models.WatchedGame
		game, err := scanGameCard(rows, &w.Pinned, &w.TargetPrice, &w.WatchedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan watched game: %w", err)
		}
		w.Game = game
		watchlist = append(watchlist, w)
	}
	return watchlist, rows.Err()
}

// UpsertWatchedGame watches a game (or updates pin/target price if already watched).
func (db *DB) UpsertWatchedGame(ctx context.Context, userID int64, appID int, pinned bool, targetPrice *float64) error {
	tag, err := db.Pool.Exec(ctx, `
		INSERT INTO watched_games (user_id, app_id, pinned, target_price)
		SELECT $1, $2, $3, $4
		WHERE EXISTS (SELECT 1 FROM games WHERE app_id = $2)
		ON CONFLICT (user_id, app_id) DO UPDATE SET pinned = EXCLUDED.pinned, target_price = EXCLUDED.target_price`,
		userID, appID, pinned, targetPrice,
	)
	if err != nil {
		return fmt.Errorf("failed to watch game: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) DeleteWatchedGame(ctx context.Context, userID int64, appID int) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM watched_games WHERE user_id = $1 AND app_id = $2`, userID, appID)
	if err != nil {
		return fmt.Errorf("failed to unwatch game: %w", err)
	}
	return nil
}
