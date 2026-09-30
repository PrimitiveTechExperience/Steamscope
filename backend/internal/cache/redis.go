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

// Allow is a fixed-window rate limiter: at most `limit` calls per `window`
// for a given key. Fails open if Redis errors, so a Redis blip doesn't lock
// everyone out of logging in.
func Allow(ctx context.Context, rdb *redis.Client, key string, limit int64, window time.Duration) bool {
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return true
	}
	if count == 1 {
		rdb.Expire(ctx, key, window)
	}
	return count <= limit
}
