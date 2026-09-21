package database

import (
	"context"
	"fmt"
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
