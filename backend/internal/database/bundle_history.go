package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

// InsertBundlePriceHistory adds days to a bundle's price history without
// touching days that already have a row, so a price we scraped from Steam
// itself is never replaced by an imported one. It returns how many days were
// added.
func (db *DB) InsertBundlePriceHistory(ctx context.Context, bundleID int, points []models.PricePoint) (int, error) {
	const batch = 2000 // 5 args each, well under Postgres' 65535 parameter limit
	added := 0
	for start := 0; start < len(points); start += batch {
		chunk := points[start:min(start+batch, len(points))]
		var sb strings.Builder
		sb.WriteString("INSERT INTO bundle_price_history (bundle_id, recorded_date, price, original_price, discount_percentage) VALUES ")
		args := make([]any, 0, len(chunk)*5)
		for i, p := range chunk {
			if i > 0 {
				sb.WriteString(",")
			}
			n := len(args)
			fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d)", n+1, n+2, n+3, n+4, n+5)
			args = append(args, bundleID, p.Date.Format("2006-01-02"), p.Price, p.OriginalPrice, p.DiscountPercentage)
		}
		sb.WriteString(" ON CONFLICT (bundle_id, recorded_date) DO NOTHING")
		tag, err := db.Pool.Exec(ctx, sb.String(), args...)
		if err != nil {
			return added, fmt.Errorf("failed to insert price history for bundle %d: %w", bundleID, err)
		}
		added += int(tag.RowsAffected())
	}
	return added, nil
}

// GetBundlePriceHistory returns a bundle's recorded daily prices, oldest first.
func (db *DB) GetBundlePriceHistory(ctx context.Context, bundleID int) ([]models.PricePoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT recorded_date, price, original_price, discount_percentage
		FROM bundle_price_history WHERE bundle_id = $1 ORDER BY recorded_date`, bundleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get price history for bundle %d: %w", bundleID, err)
	}
	defer rows.Close()
	points := []models.PricePoint{}
	for rows.Next() {
		var p models.PricePoint
		if err := rows.Scan(&p.Date, &p.Price, &p.OriginalPrice, &p.DiscountPercentage); err != nil {
			return nil, fmt.Errorf("failed to scan price point for bundle %d: %w", bundleID, err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetBundlePriceHistorySummary is the cheap fingerprint of a bundle's history
// that a cached forecast is checked against.
func (db *DB) GetBundlePriceHistorySummary(ctx context.Context, bundleID int) (PriceHistorySummary, error) {
	var s PriceHistorySummary
	var last *time.Time
	var price *float64
	err := db.Pool.QueryRow(ctx, `
		SELECT count(*), max(recorded_date),
			(SELECT price FROM bundle_price_history WHERE bundle_id = $1 ORDER BY recorded_date DESC LIMIT 1)
		FROM bundle_price_history WHERE bundle_id = $1`, bundleID).Scan(&s.Rows, &last, &price)
	if err != nil {
		return s, fmt.Errorf("failed to summarise price history for bundle %d: %w", bundleID, err)
	}
	if last != nil {
		s.LastDate = *last
	}
	if price != nil {
		s.LastPrice = *price
	}
	return s, nil
}

// GetBundleIDsNeedingBackfill lists tracked bundles with fewer than minRows
// days of history.
func (db *DB) GetBundleIDsNeedingBackfill(ctx context.Context, minRows int) ([]int, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT b.bundle_id FROM bundles b
		WHERE b.status = 'tracked'
		  AND (SELECT count(*) FROM bundle_price_history h WHERE h.bundle_id = b.bundle_id) < $1
		ORDER BY b.bundle_id`, minRows)
	if err != nil {
		return nil, fmt.Errorf("failed to find bundles needing a backfill: %w", err)
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
