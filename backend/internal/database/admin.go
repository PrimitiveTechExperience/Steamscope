package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// AdminUser is a user row as shown on the admin page.
type AdminUser struct {
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	SteamID   *string   `json:"steam_id"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

// AdminItem is a tracked game or bundle of any status.
type AdminItem struct {
	Kind        string    `json:"kind"` // "app" or "bundle"
	ID          int       `json:"id"`
	Name        *string   `json:"name"`
	Status      string    `json:"status"`
	SubmittedBy *string   `json:"submitted_by"`
	CreatedAt   time.Time `json:"created_at"`
}

func (db *DB) ListUsers(ctx context.Context) ([]AdminUser, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, username, email, steam_id, is_admin, created_at
		FROM users ORDER BY created_at DESC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()
	users := []AdminUser{}
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(&u.UserID, &u.Username, &u.Email, &u.SteamID, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// DeleteUser removes a user. Their preferences, watchlist and notifications
// go with them; games they submitted stay (submitted_by is set to null).
// Admins cannot be deleted this way. It returns ErrNotFound if no such
// non-admin user exists.
func (db *DB) DeleteUser(ctx context.Context, userID int64) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM users WHERE user_id = $1 AND NOT is_admin`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListItems returns every tracked game and bundle with its status, newest
// first.
func (db *DB) ListItems(ctx context.Context) ([]AdminItem, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT * FROM (
			SELECT 'app' AS kind, t.app_id AS id, g.name, t.status, u.username, t.created_at
			FROM tracked_games t
			LEFT JOIN games g ON g.app_id = t.app_id
			LEFT JOIN users u ON u.user_id = t.submitted_by
			UNION ALL
			SELECT 'bundle', b.bundle_id, NULLIF(b.name, ''), b.status, u.username, b.created_at
			FROM bundles b
			LEFT JOIN users u ON u.user_id = b.submitted_by
		) i
		ORDER BY created_at DESC
		LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}
	defer rows.Close()
	items := []AdminItem{}
	for rows.Next() {
		var i AdminItem
		if err := rows.Scan(&i.Kind, &i.ID, &i.Name, &i.Status, &i.SubmittedBy, &i.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

// DeleteItem stops tracking a game or bundle and deletes its stored data
// (price history, reviews and so on cascade from the games row).
func (db *DB) DeleteItem(ctx context.Context, kind string, id int) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var n int64
	if kind == "bundle" {
		tag, err := tx.Exec(ctx, `DELETE FROM bundles WHERE bundle_id = $1`, id)
		if err != nil {
			return fmt.Errorf("failed to delete bundle: %w", err)
		}
		n = tag.RowsAffected()
	} else {
		tag, err := tx.Exec(ctx, `DELETE FROM tracked_games WHERE app_id = $1`, id)
		if err != nil {
			return fmt.Errorf("failed to delete tracked game: %w", err)
		}
		n = tag.RowsAffected()
		if _, err := tx.Exec(ctx, `DELETE FROM games WHERE app_id = $1`, id); err != nil {
			return fmt.Errorf("failed to delete game: %w", err)
		}
	}
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// ReviewItem moves an awaiting_approval game or bundle to newStatus
// ("pending" to approve, "rejected" to reject) and returns who submitted it
// (nil if that user is gone). ErrNotFound if it isn't awaiting approval.
func (db *DB) ReviewItem(ctx context.Context, kind string, id int, newStatus string) (*int64, error) {
	query := `UPDATE tracked_games SET status = $2 WHERE app_id = $1 AND status = 'awaiting_approval' RETURNING submitted_by`
	if kind == "bundle" {
		query = `UPDATE bundles SET status = $2, updated_at = now() WHERE bundle_id = $1 AND status = 'awaiting_approval' RETURNING submitted_by`
	}
	var submitter *int64
	err := db.Pool.QueryRow(ctx, query, id, newStatus).Scan(&submitter)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to review item: %w", err)
	}
	return submitter, nil
}

// GetTrackStatuses returns the tracking status of each given app ID that has
// a tracked_games row; missing IDs mean "never submitted".
func (db *DB) GetTrackStatuses(ctx context.Context, appIDs []int) (map[int]string, error) {
	statuses := map[int]string{}
	if len(appIDs) == 0 {
		return statuses, nil
	}
	rows, err := db.Pool.Query(ctx, `SELECT app_id, status FROM tracked_games WHERE app_id = ANY($1)`, appIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get track statuses: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		statuses[id] = status
	}
	return statuses, rows.Err()
}
