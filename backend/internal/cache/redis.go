package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// New connects to Redis from a URL (redis:// or rediss:// - Upstash's TLS
// URLs work as-is) and verifies the connection before returning.
func New(ctx context.Context, url string) (*redis.Client, error) {
	if url == "" {
		return nil, fmt.Errorf("REDIS_URL is not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_URL: %w", err)
	}
	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}
	return client, nil
}

// countScript adds one to a counter and makes sure it expires, in one step.
//
// Doing INCR and then EXPIRE as two calls left a gap: if the process stopped or
// the second call failed between them, the counter had no expiry and kept
// counting forever, so that address was rate limited permanently (a real
// ::1 counter was found at 233 with no expiry). Checking the TTL here also
// repairs a counter left like that by the old code.
//
// KEYS[1] the counter; ARGV[1] the window in milliseconds.
var countScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 or redis.call('PTTL', KEYS[1]) < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

// Allow is a fixed-window rate limiter: at most `limit` calls per `window`
// for a given key. Fails open if Redis errors, so a Redis blip doesn't lock
// everyone out of logging in.
func Allow(ctx context.Context, rdb *redis.Client, key string, limit int64, window time.Duration) bool {
	count, err := countScript.Run(ctx, rdb, []string{key}, window.Milliseconds()).Int64()
	if err != nil {
		return true
	}
	return count <= limit
}
