package handlers

import (
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/itad"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/prediction"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestLowestPaidIgnoresFreePromotions(t *testing.T) {
	events := []itad.HistoryEvent{
		{Timestamp: day(2026, 10, 1), Price: 4.99, Regular: 9.99, Cut: 50},
		{Timestamp: day(2020, 6, 1), Price: 0, Regular: 9.99, Cut: 100}, // free weekend
		{Timestamp: day(2016, 6, 1), Price: 0, Regular: 0},              // $0 with no regular price recorded
		{Timestamp: day(2018, 2, 1), Price: 3.99, Regular: 9.99, Cut: 60},
		{Timestamp: day(2013, 1, 1), Price: 9.99, Regular: 9.99},
	}
	if got := lowestPaid(events); got != 3.99 {
		t.Errorf("lowest paid = %v, want 3.99 (the $0 free weekend is not a price anyone paid)", got)
	}
	if got := lowestPaid(nil); got != 0 {
		t.Errorf("no events: %v, want 0 (unknown)", got)
	}
	free := []itad.HistoryEvent{{Timestamp: day(2020, 1, 1), Price: 0, Regular: 0}}
	if got := lowestPaid(free); got != 0 {
		t.Errorf("a free-to-play game: %v, want 0", got)
	}
}

func TestMergeHistoryOnlyFillsDatesBeforeTheRecordedData(t *testing.T) {
	now := day(2026, 10, 5)
	recorded := []prediction.Point{
		{Date: day(2026, 9, 1), Price: 20, Regular: 20},
		{Date: day(2026, 9, 2), Price: 10, Regular: 20},
	}
	events := []itad.HistoryEvent{
		{Timestamp: day(2026, 1, 1), Price: 20, Regular: 20},
		{Timestamp: day(2026, 3, 1), Price: 10, Regular: 20, Cut: 50},
		{Timestamp: day(2026, 3, 10), Price: 20, Regular: 20},
		// ITAD also has a (conflicting) record of September; recorded data must win.
		{Timestamp: day(2026, 9, 1), Price: 5, Regular: 20, Cut: 75},
	}
	merged := mergeHistory(recorded, events, now)

	byDate := map[time.Time]prediction.Point{}
	for i, p := range merged {
		byDate[p.Date] = p
		if i > 0 && !p.Date.After(merged[i-1].Date) {
			t.Fatalf("dates must be strictly increasing, %v then %v", merged[i-1].Date, p.Date)
		}
	}
	if p := byDate[day(2026, 9, 1)]; p.Price != 20 {
		t.Errorf("1 Sep price = %v, want the recorded $20 and not ITAD's $5", p.Price)
	}
	if p := byDate[day(2026, 3, 5)]; p.Price != 10 {
		t.Errorf("5 Mar price = %v, want ITAD's $10 sale", p.Price)
	}
	if merged[0].Date.After(day(2026, 1, 2)) {
		t.Errorf("history starts %v, want it to reach back to ITAD's first event", merged[0].Date)
	}
	if last := merged[len(merged)-1]; !last.Date.Equal(day(2026, 9, 2)) {
		t.Errorf("history ends %v, want the last recorded day", last.Date)
	}
}

func TestMergeHistoryWithNoEventsKeepsRecorded(t *testing.T) {
	recorded := []prediction.Point{{Date: day(2026, 9, 1), Price: 20, Regular: 20}}
	if got := mergeHistory(recorded, nil, day(2026, 10, 5)); len(got) != 1 {
		t.Errorf("got %d points, want the recorded one untouched", len(got))
	}
}
