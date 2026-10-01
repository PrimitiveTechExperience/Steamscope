package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// SeedTrackedGames marks app IDs (e.g. from TRACKED_APP_IDS) as tracked,
// leaving any existing rows alone.
func (db *DB) SeedTrackedGames(ctx context.Context, appIDs []int) error {
	if len(appIDs) == 0 {
		return nil
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO tracked_games (app_id, status)
		SELECT unnest($1::int[]), 'tracked'
		ON CONFLICT (app_id) DO NOTHING`, appIDs)
	if err != nil {
		return fmt.Errorf("failed to seed tracked games: %w", err)
	}
	return nil
}

// GetTrackedAppIDs returns every game the scheduler should re-scrape.
func (db *DB) GetTrackedAppIDs(ctx context.Context) ([]int, error) {
	rows, err := db.Pool.Query(ctx, `SELECT app_id FROM tracked_games WHERE status = 'tracked' ORDER BY app_id`)
	if err != nil {
		return nil, fmt.Errorf("failed to get tracked games: %w", err)
	}
	defer rows.Close()

	appIDs := []int{}
	for rows.Next() {
		var appID int
		if err := rows.Scan(&appID); err != nil {
			return nil, fmt.Errorf("failed to scan tracked game: %w", err)
		}
		appIDs = append(appIDs, appID)
	}
	return appIDs, rows.Err()
}

// SubmitTrackedGame records a user's submission with the given initial
// status: "awaiting_approval" for ordinary users, "pending" (scrape now)
// for admins. created is true for a brand new game or a retry of a failed
// one, false if it already exists in any other state.
func (db *DB) SubmitTrackedGame(ctx context.Context, appID int, userID int64, initial string) (status string, shouldScrape bool, err error) {
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO tracked_games (app_id, submitted_by, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (app_id) DO UPDATE
			SET status = EXCLUDED.status, submitted_by = EXCLUDED.submitted_by, created_at = now()
			WHERE tracked_games.status = 'failed'
		RETURNING status`, appID, userID, initial,
	).Scan(&status)
	if err == nil {
		return status, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("failed to submit game: %w", err)
	}

	// Conflict with a tracked/pending row: report its current status.
	if err := db.Pool.QueryRow(ctx, `SELECT status FROM tracked_games WHERE app_id = $1`, appID).Scan(&status); err != nil {
		return "", false, fmt.Errorf("failed to read game status: %w", err)
	}
	return status, false, nil
}

func (db *DB) SetTrackedGameStatus(ctx context.Context, appID int, status string) error {
	_, err := db.Pool.Exec(ctx, `UPDATE tracked_games SET status = $2 WHERE app_id = $1`, appID, status)
	if err != nil {
		return fmt.Errorf("failed to set tracked game status: %w", err)
	}
	return nil
}

// GetGameName returns the scraped name for appID, or ErrNotFound if the
// game has never been successfully scraped.
func (db *DB) GetGameName(ctx context.Context, appID int) (string, error) {
	var name string
	err := db.Pool.QueryRow(ctx, `SELECT name FROM games WHERE app_id = $1`, appID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

func (db *DB) GetSubmissions(ctx context.Context, userID int64) ([]models.Submission, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT * FROM (
			SELECT 'app' AS kind, t.app_id AS id, g.name, t.status, t.created_at
			FROM tracked_games t
			LEFT JOIN games g ON g.app_id = t.app_id
			WHERE t.submitted_by = $1
			UNION ALL
			SELECT 'bundle', b.bundle_id, NULLIF(b.name, ''), b.status, b.created_at
			FROM bundles b
			WHERE b.submitted_by = $1
		) s
		ORDER BY created_at DESC
		LIMIT 50`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get submissions: %w", err)
	}
	defer rows.Close()

	submissions := []models.Submission{}
	for rows.Next() {
		var s models.Submission
		if err := rows.Scan(&s.Kind, &s.ID, &s.Name, &s.Status, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan submission: %w", err)
		}
		submissions = append(submissions, s)
	}
	return submissions, rows.Err()
}
