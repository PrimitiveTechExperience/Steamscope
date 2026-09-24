package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

func (db *DB) UpsertPriceHistory(ctx context.Context, appID int, price, originalPrice float64, discountPercentage int, date time.Time) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO price_history (app_id, recorded_date, price, original_price, discount_percentage)
			VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (app_id, recorded_date) DO UPDATE SET
			price = EXCLUDED.price,
			original_price = EXCLUDED.original_price,
			discount_percentage = EXCLUDED.discount_percentage
		`,
		appID, date.Format("2006-01-02"), price, originalPrice, discountPercentage,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert price history for app_id %d: %w", appID, err)
	}
	return nil
}

// UpsertPriceHistoryBatch writes an entire series for one game in a single
// round trip, instead of one query per day. Backfilling ~2 years (~730 rows)
// per game one row at a time over a remote DB connection is slow enough to
// realistically get interrupted partway through; batching makes each game's
// backfill a single atomic statement.
func (db *DB) UpsertPriceHistoryBatch(ctx context.Context, appID int, points []models.PricePoint) error {
	if len(points) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("INSERT INTO price_history (app_id, recorded_date, price, original_price, discount_percentage) VALUES ")
	args := make([]any, 0, len(points)*5)
	for i, point := range points {
		if i > 0 {
			sb.WriteString(",")
		}
		base := len(args)
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5)
		args = append(args, appID, point.Date.Format("2006-01-02"), point.Price, point.OriginalPrice, point.DiscountPercentage)
	}
	sb.WriteString(`
		ON CONFLICT (app_id, recorded_date) DO UPDATE SET
			price = EXCLUDED.price,
			original_price = EXCLUDED.original_price,
			discount_percentage = EXCLUDED.discount_percentage
	`)

	if _, err := db.Pool.Exec(ctx, sb.String(), args...); err != nil {
		return fmt.Errorf("failed to batch upsert price history for app_id %d: %w", appID, err)
	}
	return nil
}

func (db *DB) DeleteOldPriceHistory(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM price_history WHERE recorded_date < $1`, cutoff.Format("2006-01-02"))
	if err != nil {
		return 0, fmt.Errorf("failed to delete old price history: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (db *DB) GetPriceHistory(ctx context.Context, appID int) ([]models.PricePoint, error) {
	rows, err := db.Pool.Query(
		ctx,
		`
		SELECT recorded_date, price, original_price, discount_percentage
		FROM price_history
		WHERE app_id = $1
		ORDER BY recorded_date ASC
		`,
		appID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get price history for app_id %d: %w", appID, err)
	}
	defer rows.Close()

	points := []models.PricePoint{}
	for rows.Next() {
		var point models.PricePoint
		if err := rows.Scan(&point.Date, &point.Price, &point.OriginalPrice, &point.DiscountPercentage); err != nil {
			return nil, fmt.Errorf("failed to scan price history for app_id %d: %w", appID, err)
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read price history for app_id %d: %w", appID, err)
	}
	return points, nil
}
