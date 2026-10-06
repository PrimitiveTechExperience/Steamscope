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

// WatchedAmong returns which of appIDs the user already watches.
func (db *DB) WatchedAmong(ctx context.Context, userID int64, appIDs []int) (map[int]bool, error) {
	watched := map[int]bool{}
	if len(appIDs) == 0 {
		return watched, nil
	}
	rows, err := db.Pool.Query(ctx, `SELECT app_id FROM watched_games WHERE user_id = $1 AND app_id = ANY($2)`, userID, appIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to look up watched games: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		watched[id] = true
	}
	return watched, rows.Err()
}

// WishlistRequest is a wishlist game a user asked to have added, and whether
// they want it pinned to their feed once it is.
type WishlistRequest struct {
	AppID  int
	Pinned bool
}

// RecordWishlistRequests remembers what the user asked for. Asking again
// changes the pin choice.
func (db *DB) RecordWishlistRequests(ctx context.Context, userID int64, requests []WishlistRequest) error {
	for _, r := range requests {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO wishlist_requests (user_id, app_id, pinned) VALUES ($1, $2, $3)
			ON CONFLICT (user_id, app_id) DO UPDATE SET pinned = EXCLUDED.pinned`,
			userID, r.AppID, r.Pinned); err != nil {
			return fmt.Errorf("failed to record wishlist request: %w", err)
		}
	}
	return nil
}

// WishlistRequested returns which of appIDs the user has asked for.
func (db *DB) WishlistRequested(ctx context.Context, userID int64, appIDs []int) (map[int]bool, error) {
	requested := map[int]bool{}
	if len(appIDs) == 0 {
		return requested, nil
	}
	rows, err := db.Pool.Query(ctx, `SELECT app_id FROM wishlist_requests WHERE user_id = $1 AND app_id = ANY($2)`, userID, appIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to look up wishlist requests: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		requested[id] = true
	}
	return requested, rows.Err()
}

// ApplyWishlistRequests is called when a game has just become tracked: every
// user who asked for it starts watching it (pinned if they chose that), and
// the requests are cleared. It returns how many users were affected.
func (db *DB) ApplyWishlistRequests(ctx context.Context, appID int) (int, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		INSERT INTO watched_games (user_id, app_id, pinned)
		SELECT r.user_id, r.app_id, r.pinned FROM wishlist_requests r
		WHERE r.app_id = $1 AND EXISTS (SELECT 1 FROM games WHERE app_id = $1)
		ON CONFLICT (user_id, app_id) DO UPDATE SET pinned = watched_games.pinned OR EXCLUDED.pinned`, appID)
	if err != nil {
		return 0, fmt.Errorf("failed to watch requested games: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM wishlist_requests WHERE app_id = $1 AND EXISTS (SELECT 1 FROM games WHERE app_id = $1)`, appID); err != nil {
		return 0, fmt.Errorf("failed to clear wishlist requests: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// DeleteWishlistRequests drops everyone's requests for a game that will not be
// added (turned down, blocked, or it could not be found).
func (db *DB) DeleteWishlistRequests(ctx context.Context, appID int) error {
	if _, err := db.Pool.Exec(ctx, `DELETE FROM wishlist_requests WHERE app_id = $1`, appID); err != nil {
		return fmt.Errorf("failed to delete wishlist requests: %w", err)
	}
	return nil
}
