package itad

import (
	"sort"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

// BuildDailySeries turns a sparse log of price-change events into a daily
// price series covering [startDate, endDate], by carrying the most recent
// known price forward across every day where nothing changed. Days before
// the first recorded event are left out: we have no data for them, and for a
// recent release inventing a price would draw a history that never existed.
func BuildDailySeries(events []HistoryEvent, startDate, endDate time.Time) []models.PricePoint {
	if len(events) == 0 {
		return nil
	}

	sorted := make([]HistoryEvent, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp.Before(sorted[j].Timestamp) })

	startDate = truncateToDay(startDate)
	endDate = truncateToDay(endDate)

	points := make([]models.PricePoint, 0, int(endDate.Sub(startDate).Hours()/24)+1)
	eventIdx := 0
	var current HistoryEvent

	for day := startDate; !day.After(endDate); day = day.AddDate(0, 0, 1) {
		for eventIdx < len(sorted) && !sorted[eventIdx].Timestamp.After(day) {
			current = sorted[eventIdx]
			eventIdx++
		}
		if eventIdx == 0 {
			continue // no price known yet on this day
		}
		points = append(points, models.PricePoint{
			Date:               day,
			Price:              current.Price,
			OriginalPrice:      current.Regular,
			DiscountPercentage: current.Cut,
		})
	}
	return points
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
