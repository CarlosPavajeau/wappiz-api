package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*Redis, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return NewRedis(Config{Limit: 3, Window: time.Minute}, client), mr
}

func TestRedis_Allow_UnderLimit(t *testing.T) {
	r, _ := newTestRedis(t)

	ctx := context.Background()
	for i := range 3 {
		res, err := r.Allow(ctx, "key")
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("request %d: expected allowed, got denied", i)
		}
	}
}

func TestRedis_Allow_BlocksAtLimit(t *testing.T) {
	r, _ := newTestRedis(t)

	ctx := context.Background()
	r.Allow(ctx, "key") //nolint:errcheck
	r.Allow(ctx, "key") //nolint:errcheck
	r.Allow(ctx, "key") //nolint:errcheck

	res, err := r.Allow(ctx, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Allowed {
		t.Fatal("expected denied on request beyond limit=3")
	}
	if res.Remaining != 0 {
		t.Errorf("expected Remaining=0, got %d", res.Remaining)
	}
}

func TestRedis_Allow_WindowSlides(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	r := NewRedis(Config{Limit: 2, Window: 10 * time.Second}, client)

	ctx := context.Background()

	r.Allow(ctx, "key") //nolint:errcheck
	r.Allow(ctx, "key") //nolint:errcheck

	// Fast-forward miniredis clock past the window.
	mr.FastForward(11 * time.Second)

	res, err := r.Allow(ctx, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed {
		t.Fatal("expected allowed after window slides")
	}
}

func TestRedis_Allow_RemainingDecreases(t *testing.T) {
	r, _ := newTestRedis(t)

	ctx := context.Background()

	res1, _ := r.Allow(ctx, "key")
	res2, _ := r.Allow(ctx, "key")
	res3, _ := r.Allow(ctx, "key")

	if res1.Remaining != 2 {
		t.Errorf("expected Remaining=2, got %d", res1.Remaining)
	}
	if res2.Remaining != 1 {
		t.Errorf("expected Remaining=1, got %d", res2.Remaining)
	}
	if res3.Remaining != 0 {
		t.Errorf("expected Remaining=0, got %d", res3.Remaining)
	}
}

func TestRedis_Allow_IndependentKeys(t *testing.T) {
	r, _ := newTestRedis(t)

	ctx := context.Background()

	r.Allow(ctx, "key-a") //nolint:errcheck
	r.Allow(ctx, "key-a") //nolint:errcheck
	r.Allow(ctx, "key-a") //nolint:errcheck

	res, err := r.Allow(ctx, "key-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed {
		t.Fatal("key-b should not be affected by key-a's limit")
	}
}

func TestRedis_Allow_Concurrent(t *testing.T) {
	const limit = 10
	const goroutines = 20

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	r := NewRedis(Config{Limit: limit, Window: time.Minute}, client)

	ctx := context.Background()
	var allowed atomic.Int64
	var wg sync.WaitGroup

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := r.Allow(ctx, "shared")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if res.Allowed {
				allowed.Add(1)
			}
		}()
	}

	wg.Wait()

	if got := allowed.Load(); got != limit {
		t.Errorf("expected exactly %d allowed, got %d", limit, got)
	}
}

func TestRedis_Close_IsNoop(t *testing.T) {
	r, _ := newTestRedis(t)
	if err := r.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Errorf("second Close returned error: %v", err)
	}
}
