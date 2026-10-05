package itad

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
)

// ErrNoHistory means ITAD knows the game but has no usable price history
// for it (e.g. a game that has never been sold through a tracked shop).
var ErrNoHistory = errors.New("ITAD has no price history")

// BackfillGame writes a daily price_history series for one game covering
// [from, to], built from ITAD's price-change log. Safe to re-run: rows are
// upserted on (app_id, recorded_date).
func BackfillGame(ctx context.Context, db *database.DB, client *Client, appID int, from, to time.Time) (int, error) {
	itadID, err := client.LookupGameID(appID)
	if err != nil {
		return 0, err
	}
	events, err := client.GetHistory(itadID)
	if err != nil {
		return 0, err
	}
	series := BuildDailySeries(events, from, to)
	if len(series) == 0 {
		return 0, fmt.Errorf("appID %d: %w", appID, ErrNoHistory)
	}
	if err := db.UpsertPriceHistoryBatch(ctx, appID, series); err != nil {
		return 0, err
	}
	// Self-heal: drop rows before the first real event that an older version
	// of this backfill invented from that event's price.
	first := events[0]
	for _, e := range events {
		if e.Timestamp.Before(first.Timestamp) {
			first = e
		}
	}
	if _, err := db.DeleteFabricatedHistory(ctx, appID, truncateToDay(first.Timestamp), first.Price, first.Regular); err != nil {
		return len(series), err
	}
	return len(series), nil
}
