package itad

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestBuildDailySeriesSkipsDaysBeforeFirstEvent(t *testing.T) {
	events := []HistoryEvent{
		{Timestamp: day(2026, 9, 2), Price: 7.99, Regular: 7.99},
		{Timestamp: day(2026, 9, 4), Price: 5.99, Regular: 7.99, Cut: 25},
	}
	series := BuildDailySeries(events, day(2026, 8, 30), day(2026, 9, 5))

	if len(series) != 4 {
		t.Fatalf("got %d points, want 4 (Sep 2-5)", len(series))
	}
	if !series[0].Date.Equal(day(2026, 9, 2)) {
		t.Errorf("series starts %v, want 2026-09-02", series[0].Date)
	}
	if series[1].Price != 7.99 || series[2].Price != 5.99 || series[3].Price != 5.99 {
		t.Errorf("unexpected carry-forward prices: %+v", series)
	}
}

func TestBuildDailySeriesEmpty(t *testing.T) {
	if got := BuildDailySeries(nil, day(2026, 1, 1), day(2026, 1, 5)); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
