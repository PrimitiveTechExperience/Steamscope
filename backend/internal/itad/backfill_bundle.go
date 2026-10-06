package itad

import (
	"context"
	"fmt"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
)

// BundleHistoryYears is how far back bundle histories are imported; it matches
// how much ITAD history the price forecast uses.
const BundleHistoryYears = 6

// BackfillBundle fills in a bundle's daily price history over [from, to] from
// ITAD's price log, so a bundle has years of history instead of starting on the
// day it was first scraped. Days we already recorded ourselves are left as
// they are (Steam's own number wins), so it is safe to re-run. It returns how
// many days it added.
func BackfillBundle(ctx context.Context, db *database.DB, client *Client, bundleID int, from, to time.Time) (int, error) {
	events, err := client.BundleHistory(ctx, bundleID)
	if err != nil {
		return 0, err
	}
	// A bundle is never free; a zero in the log means "no price", not a price.
	series := BuildDailySeries(events, from, to)
	paid := series[:0]
	for _, p := range series {
		if p.Price > 0 {
			paid = append(paid, p)
		}
	}
	series = paid
	if len(series) == 0 {
		return 0, fmt.Errorf("bundle %d: %w", bundleID, ErrNoHistory)
	}
	return db.InsertBundlePriceHistory(ctx, bundleID, series)
}
