package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb, mini
}

func TestAllowLimitsCallsWithinTheWindow(t *testing.T) {
	rdb, _ := testRedis(t)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		if !Allow(ctx, rdb, "k", 3, time.Minute) {
			t.Fatalf("call %d was refused, the limit is 3", i)
		}
	}
	if Allow(ctx, rdb, "k", 3, time.Minute) {
		t.Error("the 4th call was allowed")
	}
	if Allow(ctx, rdb, "k", 3, time.Minute) {
		t.Error("the 5th call was allowed")
	}
}

func TestAllowCountsEachKeySeparately(t *testing.T) {
	rdb, _ := testRedis(t)
	ctx := context.Background()
	Allow(ctx, rdb, "a", 1, time.Minute)
	if Allow(ctx, rdb, "a", 1, time.Minute) {
		t.Error("a second call on key a was allowed")
	}
	if !Allow(ctx, rdb, "b", 1, time.Minute) {
		t.Error("key b was limited by key a's calls")
	}
}

func TestAllowStartsAFreshWindowOnceItExpires(t *testing.T) {
	rdb, mini := testRedis(t)
	ctx := context.Background()
	Allow(ctx, rdb, "k", 1, time.Minute)
	if Allow(ctx, rdb, "k", 1, time.Minute) {
		t.Fatal("the limit was not enforced")
	}
	mini.FastForward(61 * time.Second)
	if !Allow(ctx, rdb, "k", 1, time.Minute) {
		t.Error("still limited after the window passed")
	}
}

func TestAllowAlwaysGivesTheCounterAnExpiry(t *testing.T) {
	rdb, mini := testRedis(t)
	Allow(context.Background(), rdb, "k", 5, 90*time.Second)
	if ttl := mini.TTL("k"); ttl <= 0 || ttl > 90*time.Second {
		t.Errorf("ttl = %v, want up to the 90s window", ttl)
	}
}

func TestAllowRepairsACounterThatWasLeftWithoutAnExpiry(t *testing.T) {
	// This is how an address ended up permanently limited: the counter was
	// created but the expiry was never set.
	rdb, mini := testRedis(t)
	mini.Set("ratelimit:predict:::1", "233")
	if mini.TTL("ratelimit:predict:::1") != 0 {
		t.Fatal("setup: the key should have no expiry")
	}
	if Allow(context.Background(), rdb, "ratelimit:predict:::1", 30, time.Minute) {
		t.Error("a counter over the limit was allowed")
	}
	if ttl := mini.TTL("ratelimit:predict:::1"); ttl <= 0 {
		t.Fatalf("ttl = %v: the counter was left without an expiry", ttl)
	}
	mini.FastForward(61 * time.Second)
	if !Allow(context.Background(), rdb, "ratelimit:predict:::1", 30, time.Minute) {
		t.Error("the address is still locked out after the window, as it would have been forever before")
	}
}

func TestAllowDoesNotExtendTheWindowOnLaterCalls(t *testing.T) {
	rdb, mini := testRedis(t)
	ctx := context.Background()
	Allow(ctx, rdb, "k", 10, time.Minute)
	mini.FastForward(40 * time.Second)
	Allow(ctx, rdb, "k", 10, time.Minute)
	if ttl := mini.TTL("k"); ttl > 21*time.Second {
		t.Errorf("ttl = %v: a later call pushed the window out (it should end 60s after the first)", ttl)
	}
}

func TestAllowFailsOpenWhenRedisIsDown(t *testing.T) {
	rdb, mini := testRedis(t)
	mini.Close()
	if !Allow(context.Background(), rdb, "k", 1, time.Minute) {
		t.Error("a Redis outage locked everyone out")
	}
}

func TestAllowIsExactUnderConcurrency(t *testing.T) {
	rdb, _ := testRedis(t)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if Allow(context.Background(), rdb, "k", 10, time.Minute) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Errorf("%d of 50 simultaneous calls were allowed, want exactly 10", allowed.Load())
	}
}
