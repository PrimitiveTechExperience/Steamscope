package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// UpsertBundle stores a scraped bundle (marking it tracked), replaces its
// game list, and records today's price, all in one transaction.
func (db *DB) UpsertBundle(ctx context.Context, b models.Bundle, recordedDate time.Time) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO bundles (bundle_id, name, url, header_image, price, original_price, discount_percentage, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'tracked')
		ON CONFLICT (bundle_id) DO UPDATE SET
			name = EXCLUDED.name,
			url = EXCLUDED.url,
			header_image = COALESCE(NULLIF(EXCLUDED.header_image, ''), bundles.header_image),
			price = EXCLUDED.price,
			original_price = EXCLUDED.original_price,
			discount_percentage = EXCLUDED.discount_percentage,
			status = 'tracked',
			updated_at = now()`,
		b.BundleID, b.Name, b.URL, b.HeaderImage, b.Price, b.OriginalPrice, b.DiscountPercentage,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert bundle %d: %w", b.BundleID, err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM bundle_games WHERE bundle_id = $1`, b.BundleID); err != nil {
		return fmt.Errorf("failed to clear bundle games: %w", err)
	}
	for _, g := range b.Games {
		if _, err := tx.Exec(ctx,
			`INSERT INTO bundle_games (bundle_id, app_id, name, price, regular_price) VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
			b.BundleID, g.AppID, g.Name, g.Price, g.RegularPrice); err != nil {
			return fmt.Errorf("failed to add bundle game: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO bundle_price_history (bundle_id, recorded_date, price, original_price, discount_percentage)
		VALUES ($1, $2::date, $3, $4, $5)
		ON CONFLICT (bundle_id, recorded_date) DO UPDATE SET
			price = EXCLUDED.price,
			original_price = EXCLUDED.original_price,
			discount_percentage = EXCLUDED.discount_percentage`,
		b.BundleID, recordedDate.Format("2006-01-02"), b.Price, b.OriginalPrice, b.DiscountPercentage,
	); err != nil {
		return fmt.Errorf("failed to record bundle price: %w", err)
	}
	return tx.Commit(ctx)
}

// SubmitBundle records a user's bundle submission with the given initial
// status (see SubmitTrackedGame). The returned bool is true for a new bundle
// or a retry of a failed one.
func (db *DB) SubmitBundle(ctx context.Context, bundleID int, userID int64, initial string) (status string, shouldScrape bool, err error) {
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO bundles (bundle_id, url, status, submitted_by)
		VALUES ($1, $2, $4, $3)
		ON CONFLICT (bundle_id) DO UPDATE
			SET status = EXCLUDED.status, submitted_by = EXCLUDED.submitted_by
			WHERE bundles.status = 'failed'
		RETURNING status`,
		bundleID, fmt.Sprintf("https://store.steampowered.com/bundle/%d", bundleID), userID, initial,
	).Scan(&status)
	if err == nil {
		return status, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("failed to submit bundle: %w", err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT status FROM bundles WHERE bundle_id = $1`, bundleID).Scan(&status); err != nil {
		return "", false, fmt.Errorf("failed to read bundle status: %w", err)
	}
	return status, false, nil
}

func (db *DB) SetBundleStatus(ctx context.Context, bundleID int, status string) error {
	_, err := db.Pool.Exec(ctx, `UPDATE bundles SET status = $2, updated_at = now() WHERE bundle_id = $1`, bundleID, status)
	if err != nil {
		return fmt.Errorf("failed to set bundle status: %w", err)
	}
	return nil
}

// GetTrackedBundleIDs returns every bundle the scheduler should re-scrape.
func (db *DB) GetTrackedBundleIDs(ctx context.Context) ([]int, error) {
	rows, err := db.Pool.Query(ctx, `SELECT bundle_id FROM bundles WHERE status = 'tracked' ORDER BY bundle_id`)
	if err != nil {
		return nil, fmt.Errorf("failed to get tracked bundles: %w", err)
	}
	defer rows.Close()

	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan bundle id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (db *DB) GetBundleName(ctx context.Context, bundleID int) (string, error) {
	var name string
	err := db.Pool.QueryRow(ctx, `SELECT name FROM bundles WHERE bundle_id = $1`, bundleID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

const bundleColumns = `b.bundle_id, b.name, b.url, b.header_image, b.price, b.original_price,
	b.discount_percentage, b.status, b.updated_at,
	(SELECT count(*) FROM bundle_games bg WHERE bg.bundle_id = b.bundle_id),
	-- At a record low: discounted, at the lowest price recorded, and the price has
	-- actually changed before (a bundle that never moves is not a "record").
	(b.discount_percentage > 0 AND b.price > 0
		AND b.price <= (SELECT min(h.price) FROM bundle_price_history h WHERE h.bundle_id = b.bundle_id) + 0.005
		AND (SELECT count(DISTINCT h.price) FROM bundle_price_history h WHERE h.bundle_id = b.bundle_id) >= 2)`

func scanBundle(row pgx.Row) (models.Bundle, error) {
	var b models.Bundle
	err := row.Scan(&b.BundleID, &b.Name, &b.URL, &b.HeaderImage, &b.Price, &b.OriginalPrice,
		&b.DiscountPercentage, &b.Status, &b.UpdatedAt, &b.GameCount, &b.AtRecordLow)
	b.Games = []models.BundleGame{}
	return b, err
}

// GetBundles lists tracked bundles, biggest discount first. With appID > 0
// it only returns bundles containing that game.
func (db *DB) GetBundles(ctx context.Context, appID int, limit int) ([]models.Bundle, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+bundleColumns+`
		FROM bundles b
		WHERE b.status = 'tracked'
			AND ($1 = 0 OR EXISTS (SELECT 1 FROM bundle_games bg WHERE bg.bundle_id = b.bundle_id AND bg.app_id = $1))
		ORDER BY b.discount_percentage DESC, b.name
		LIMIT $2`, appID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get bundles: %w", err)
	}
	defer rows.Close()

	bundles := []models.Bundle{}
	for rows.Next() {
		b, err := scanBundle(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan bundle: %w", err)
		}
		bundles = append(bundles, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := db.attachPreviewGames(ctx, bundles); err != nil {
		return nil, err
	}
	return bundles, nil
}

// previewGames is how many of a bundle's games list responses carry; enough
// for the frontend to build a cover collage when Steam has no header image.
const previewGames = 4

func (db *DB) attachPreviewGames(ctx context.Context, bundles []models.Bundle) error {
	if len(bundles) == 0 {
		return nil
	}
	ids := make([]int, len(bundles))
	index := make(map[int]int, len(bundles))
	for i, b := range bundles {
		ids[i] = b.BundleID
		index[b.BundleID] = i
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT bg.bundle_id, bg.app_id, COALESCE(NULLIF(g.name, ''), bg.name), (g.app_id IS NOT NULL)
		FROM bundle_games bg
		LEFT JOIN games g ON g.app_id = bg.app_id
		WHERE bg.bundle_id = ANY($1)
		ORDER BY bg.bundle_id, (g.app_id IS NOT NULL) DESC, bg.app_id`, ids)
	if err != nil {
		return fmt.Errorf("failed to get bundle preview games: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var bundleID int
		var g models.BundleGame
		if err := rows.Scan(&bundleID, &g.AppID, &g.Name, &g.Tracked); err != nil {
			return fmt.Errorf("failed to scan bundle preview game: %w", err)
		}
		b := &bundles[index[bundleID]]
		if len(b.Games) < previewGames {
			b.Games = append(b.Games, g)
		}
	}
	return rows.Err()
}

// GetBundle returns one bundle with its games and price history.
func (db *DB) GetBundle(ctx context.Context, bundleID int) (*models.BundleDetail, error) {
	b, err := scanBundle(db.Pool.QueryRow(ctx,
		`SELECT `+bundleColumns+` FROM bundles b WHERE b.bundle_id = $1 AND b.status = 'tracked'`, bundleID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bundle: %w", err)
	}

	gameRows, err := db.Pool.Query(ctx, `
		SELECT bg.app_id, COALESCE(NULLIF(g.name, ''), bg.name), (g.app_id IS NOT NULL),
			CASE WHEN g.app_id IS NULL THEN COALESCE(t.status, '') ELSE '' END,
			-- Our own scraped prices for tracked games, otherwise what the bundle page showed.
			CASE WHEN g.app_id IS NOT NULL THEN g.price ELSE bg.price END,
			CASE WHEN g.app_id IS NOT NULL THEN g.original_price ELSE bg.regular_price END
		FROM bundle_games bg
		LEFT JOIN games g ON g.app_id = bg.app_id
		LEFT JOIN tracked_games t ON t.app_id = bg.app_id
		WHERE bg.bundle_id = $1
		ORDER BY 3 DESC, 2`, bundleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bundle games: %w", err)
	}
	defer gameRows.Close()
	for gameRows.Next() {
		var g models.BundleGame
		if err := gameRows.Scan(&g.AppID, &g.Name, &g.Tracked, &g.TrackStatus, &g.Price, &g.RegularPrice); err != nil {
			return nil, fmt.Errorf("failed to scan bundle game: %w", err)
		}
		b.Games = append(b.Games, g)
	}
	if err := gameRows.Err(); err != nil {
		return nil, err
	}

	historyRows, err := db.Pool.Query(ctx, `
		SELECT recorded_date, price, original_price, discount_percentage
		FROM bundle_price_history WHERE bundle_id = $1 ORDER BY recorded_date`, bundleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bundle price history: %w", err)
	}
	defer historyRows.Close()
	history := []models.PricePoint{}
	for historyRows.Next() {
		var p models.PricePoint
		if err := historyRows.Scan(&p.Date, &p.Price, &p.OriginalPrice, &p.DiscountPercentage); err != nil {
			return nil, fmt.Errorf("failed to scan bundle price point: %w", err)
		}
		history = append(history, p)
	}
	if err := historyRows.Err(); err != nil {
		return nil, err
	}

	detail := &models.BundleDetail{Bundle: b, PriceHistory: history}
	if len(history) > 0 {
		detail.RecordLow = history[0].Price
		for _, p := range history {
			detail.RecordLow = min(detail.RecordLow, p.Price)
		}
		detail.HistoryDays = int(history[len(history)-1].Date.Sub(history[0].Date).Hours()/24) + 1
	}
	return detail, nil
}
