package database

import (
	"context"
	"fmt"
)

// ExistingGameIDs returns which of appIDs are games we have data for.
func (db *DB) ExistingGameIDs(ctx context.Context, appIDs []int) (map[int]bool, error) {
	found := map[int]bool{}
	if len(appIDs) == 0 {
		return found, nil
	}
	rows, err := db.Pool.Query(ctx, `SELECT app_id FROM games WHERE app_id = ANY($1)`, appIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to look up games: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		found[id] = true
	}
	return found, rows.Err()
}

// WatchGames starts watching every one of appIDs that is a known game and that
// the user does not already watch, and returns how many it added. Games already
// watched are left exactly as they were (pin and target price included).
func (db *DB) WatchGames(ctx context.Context, userID int64, appIDs []int) (int, error) {
	if len(appIDs) == 0 {
		return 0, nil
	}
	tag, err := db.Pool.Exec(ctx, `
		INSERT INTO watched_games (user_id, app_id)
		SELECT $1, app_id FROM games WHERE app_id = ANY($2)
		ON CONFLICT (user_id, app_id) DO NOTHING`, userID, appIDs)
	if err != nil {
		return 0, fmt.Errorf("failed to watch games: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
