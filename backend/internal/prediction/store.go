package prediction

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// DefaultMaxEntries is how many forecasts the cache keeps.
	DefaultMaxEntries = 5
	// DefaultTTL is how long a cached forecast is served at most.
	DefaultTTL = 12 * time.Hour

	indexKey  = "predictions:recent"
	keyPrefix = "prediction:"
)

// Store caches the most recent forecasts in Redis: each game's predicted
// prices and score, for at most MaxEntries games. Storing one more evicts the
// least recently stored. A forecast is only served while the price history it
// was built from is unchanged (DataVersion), so a fresh daily scrape never
// shows stale numbers.
type Store struct {
	rdb *redis.Client
	max int
	ttl time.Duration
}

func NewStore(rdb *redis.Client, maxEntries int, ttl time.Duration) *Store {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{rdb: rdb, max: maxEntries, ttl: ttl}
}

// putScript stores the forecast, drops index entries older than the TTL, and
// evicts the oldest entries (and their data) beyond the limit, atomically.
//
// ARGV: 1 payload, 2 now (ms), 3 ttl (s), 4 app id, 5 max entries, 6 key prefix.
var putScript = redis.NewScript(`
redis.call('SET', ARGV[6] .. ARGV[4], ARGV[1], 'EX', ARGV[3])
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[4])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', tonumber(ARGV[2]) - tonumber(ARGV[3]) * 1000)
local n = redis.call('ZCARD', KEYS[1])
local max = tonumber(ARGV[5])
if n > max then
  local old = redis.call('ZRANGE', KEYS[1], 0, n - max - 1)
  for _, id in ipairs(old) do
    redis.call('DEL', ARGV[6] .. id)
    redis.call('ZREM', KEYS[1], id)
  end
end
redis.call('EXPIRE', KEYS[1], ARGV[3])
return redis.call('ZCARD', KEYS[1])
`)

// Key is the cache key of a forecast: the app id for a game, and the negated
// bundle id for a bundle, since both are plain integers that could collide.
func (f Forecast) Key() int {
	if f.BundleID > 0 {
		return BundleKey(f.BundleID)
	}
	return f.AppID
}

// BundleKey is the cache key of a bundle's forecast.
func BundleKey(bundleID int) int { return -bundleID }

// Get returns the cached forecast stored under key (see Forecast.Key) if one
// exists and was built from the history identified by dataVersion.
func (s *Store) Get(ctx context.Context, key int, dataVersion string) (*Forecast, bool) {
	raw, err := s.rdb.Get(ctx, keyPrefix+strconv.Itoa(key)).Bytes()
	if err != nil {
		return nil, false
	}
	var f Forecast
	if json.Unmarshal(raw, &f) != nil || f.DataVersion != dataVersion {
		return nil, false
	}
	f.Cached = true
	return &f, true
}

// Put stores a forecast, evicting the oldest ones beyond the limit.
func (s *Store) Put(ctx context.Context, f Forecast) error {
	f.Cached = false
	payload, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return putScript.Run(ctx, s.rdb, []string{indexKey},
		string(payload), time.Now().UnixMilli(), int(s.ttl.Seconds()), f.Key(), s.max, keyPrefix,
	).Err()
}

// Cached lists the keys currently held (see Forecast.Key), oldest first.
func (s *Store) Cached(ctx context.Context) []int {
	ids, err := s.rdb.ZRange(ctx, indexKey, 0, -1).Result()
	if err != nil {
		return nil
	}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if n, err := strconv.Atoi(id); err == nil {
			out = append(out, n)
		}
	}
	return out
}
