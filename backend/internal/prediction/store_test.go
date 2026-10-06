package prediction

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newStore(t *testing.T, max int, ttl time.Duration) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return NewStore(rdb, max, ttl), mini
}

func sample(appID int, version string) Forecast {
	return Forecast{
		AppID: appID, DataVersion: version, Model: ModelWeibull, CurrentPrice: 19.99, Score: 72,
		GeneratedAt: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		Curve:       []CurvePoint{{Date: "2026-10-06", DaysAhead: 1, ExpectedPrice: 19.5, POnSale: 0.1, PLowerBy: 0.05}},
		Horizons:    []Horizon{{Days: 30, PLower: 0.4, ExpectedPrice: 18, ExpectedLow: 15}},
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s, _ := newStore(t, 5, time.Hour)
	ctx := context.Background()

	if _, ok := s.Get(ctx, 10, "v1"); ok {
		t.Fatal("hit on an empty cache")
	}
	want := sample(10, "v1")
	if err := s.Put(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get(ctx, 10, "v1")
	if !ok {
		t.Fatal("miss right after Put")
	}
	if !got.Cached {
		t.Error("a forecast read from the cache must say so")
	}
	got.Cached = false
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("round trip changed the forecast:\n got %+v\nwant %+v", *got, want)
	}
}

func TestStoreIgnoresForecastsBuiltFromOldData(t *testing.T) {
	s, _ := newStore(t, 5, time.Hour)
	ctx := context.Background()
	s.Put(ctx, sample(10, "history-as-of-monday"))

	if _, ok := s.Get(ctx, 10, "history-as-of-tuesday"); ok {
		t.Error("served a forecast built from different price history")
	}
	if _, ok := s.Get(ctx, 10, "history-as-of-monday"); !ok {
		t.Error("lost the forecast for matching history")
	}
}

func TestStoreKeepsOnlyTheFiveMostRecent(t *testing.T) {
	s, mini := newStore(t, 5, time.Hour)
	ctx := context.Background()

	for id := 1; id <= 8; id++ {
		if err := s.Put(ctx, sample(id, "v")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // distinct timestamps: recency decides who is evicted
	}

	if got, want := s.Cached(ctx), []int{4, 5, 6, 7, 8}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cached = %v, want the five newest %v", got, want)
	}
	for id := 1; id <= 3; id++ {
		if _, ok := s.Get(ctx, id, "v"); ok {
			t.Errorf("game %d should have been evicted", id)
		}
		if mini.Exists(keyPrefix + string(rune('0'+id))) {
			t.Errorf("evicted game %d's data is still in Redis", id)
		}
	}
	for id := 4; id <= 8; id++ {
		if _, ok := s.Get(ctx, id, "v"); !ok {
			t.Errorf("game %d should still be cached", id)
		}
	}
	// Nothing but the five forecasts and the index is left behind.
	if n := len(mini.Keys()); n != 6 {
		t.Errorf("Redis holds %d keys %v, want 5 forecasts + 1 index", n, mini.Keys())
	}
}

func TestStoreRefreshingAGameDoesNotDuplicateOrEvictOthers(t *testing.T) {
	s, _ := newStore(t, 3, time.Hour)
	ctx := context.Background()
	for id := 1; id <= 3; id++ {
		s.Put(ctx, sample(id, "v1"))
		time.Sleep(2 * time.Millisecond)
	}
	s.Put(ctx, sample(1, "v2")) // game 1 recomputed from newer data

	if got := s.Cached(ctx); len(got) != 3 {
		t.Fatalf("cached = %v, want still 3 entries", got)
	}
	if _, ok := s.Get(ctx, 1, "v2"); !ok {
		t.Error("the refreshed forecast is missing")
	}
	if _, ok := s.Get(ctx, 1, "v1"); ok {
		t.Error("the superseded forecast is still served")
	}

	// Game 1 is now the newest, so the next new game evicts game 2.
	time.Sleep(2 * time.Millisecond)
	s.Put(ctx, sample(4, "v1"))
	if _, ok := s.Get(ctx, 2, "v1"); ok {
		t.Error("game 2 was the oldest and should have been evicted")
	}
	if _, ok := s.Get(ctx, 1, "v2"); !ok {
		t.Error("refreshing a game should make it the most recent, not the next to go")
	}
}

func TestStoreEntriesExpire(t *testing.T) {
	s, mini := newStore(t, 5, time.Hour)
	ctx := context.Background()
	s.Put(ctx, sample(1, "v"))

	mini.FastForward(61 * time.Minute)
	if _, ok := s.Get(ctx, 1, "v"); ok {
		t.Error("an entry past its TTL was served")
	}
}

func TestStoreCountsOnlyLiveEntriesTowardTheLimit(t *testing.T) {
	// Entries that expired must not take up room: after an hour the cache is
	// empty, so five new games fit without evicting each other.
	s, mini := newStore(t, 5, time.Hour)
	ctx := context.Background()
	for id := 1; id <= 5; id++ {
		s.Put(ctx, sample(id, "v"))
	}
	mini.FastForward(2 * time.Hour)

	for id := 11; id <= 15; id++ {
		s.Put(ctx, sample(id, "v"))
		time.Sleep(2 * time.Millisecond)
	}
	for id := 11; id <= 15; id++ {
		if _, ok := s.Get(ctx, id, "v"); !ok {
			t.Errorf("game %d was evicted although only stale entries preceded it", id)
		}
	}
}

func TestStoreDefaults(t *testing.T) {
	s, _ := newStore(t, 0, 0)
	if s.max != DefaultMaxEntries || s.ttl != DefaultTTL || DefaultMaxEntries != 5 {
		t.Errorf("max=%d ttl=%v, want 5 and %v", s.max, s.ttl, DefaultTTL)
	}
}
