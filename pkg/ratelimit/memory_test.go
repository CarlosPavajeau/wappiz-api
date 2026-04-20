package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemory_Allow_UnderLimit(t *testing.T) {
	m := New(Config{Limit: 3, Window: time.Minute})
	defer m.Close()

	ctx := context.Background()
	for i := range 3 {
		res, err := m.Allow(ctx, "key")
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("request %d: expected allowed, got denied", i)
		}
	}
}

func TestMemory_Allow_BlocksAtLimit(t *testing.T) {
	m := New(Config{Limit: 2, Window: time.Minute})
	defer m.Close()

	ctx := context.Background()
	m.Allow(ctx, "key") //nolint:errcheck
	m.Allow(ctx, "key") //nolint:errcheck

	res, err := m.Allow(ctx, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Allowed {
		t.Fatal("expected denied on 3rd request beyond limit=2")
	}
	if res.Remaining != 0 {
		t.Errorf("expected Remaining=0, got %d", res.Remaining)
	}
	if res.ResetAt.IsZero() {
		t.Error("expected non-zero ResetAt")
	}
}

func TestMemory_Allow_WindowSlides(t *testing.T) {
	base := time.Now()
	tick := base

	m := New(Config{Limit: 2, Window: 10 * time.Second})
	m.now = func() time.Time { return tick }
	defer m.Close()

	ctx := context.Background()

	// Fill the window.
	m.Allow(ctx, "key") //nolint:errcheck
	m.Allow(ctx, "key") //nolint:errcheck

	// Advance time past the window — old timestamps expire.
	tick = base.Add(11 * time.Second)

	res, err := m.Allow(ctx, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed {
		t.Fatal("expected allowed after window slides")
	}
}

func TestMemory_Allow_RemainingDecreases(t *testing.T) {
	m := New(Config{Limit: 3, Window: time.Minute})
	defer m.Close()

	ctx := context.Background()

	res1, _ := m.Allow(ctx, "key")
	res2, _ := m.Allow(ctx, "key")
	res3, _ := m.Allow(ctx, "key")

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

func TestMemory_Allow_IndependentKeys(t *testing.T) {
	m := New(Config{Limit: 1, Window: time.Minute})
	defer m.Close()

	ctx := context.Background()

	m.Allow(ctx, "key-a") //nolint:errcheck

	res, err := m.Allow(ctx, "key-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed {
		t.Fatal("key-b should not be affected by key-a's limit")
	}
}

func TestMemory_Allow_Concurrent(t *testing.T) {
	const limit = 10
	const goroutines = 20

	m := New(Config{Limit: limit, Window: time.Minute})
	defer m.Close()

	ctx := context.Background()
	var allowed atomic.Int64
	var wg sync.WaitGroup

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := m.Allow(ctx, "shared")
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
