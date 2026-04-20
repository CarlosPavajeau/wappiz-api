package ratelimit

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Memory is a thread-safe, in-process sliding window rate limiter.
// It stores per-key timestamp rings in a sync.Map so operations on distinct
// keys never contend. A background goroutine evicts idle entries to bound
// memory usage.
//
// Construct via [New]; the zero value is not usable.
type Memory struct {
	cfg     Config
	entries sync.Map // map[string]*memEntry
	now     func() time.Time
	done    chan struct{}
	once    sync.Once
}

// memEntry holds the sliding window state for a single rate-limit key.
type memEntry struct {
	mu         sync.Mutex
	timestamps []int64 // sorted unix nanoseconds, oldest first
}

// New creates an in-memory sliding window limiter and starts a background
// cleanup goroutine that evicts stale entries every cfg.Window.
func New(cfg Config) *Memory {
	m := &Memory{
		cfg:  cfg,
		now:  time.Now,
		done: make(chan struct{}),
	}
	go m.cleanup()
	return m
}

// Allow implements [Limiter]. It is safe for concurrent use.
func (m *Memory) Allow(_ context.Context, key string) (Result, error) {
	e := m.loadOrStore(key)

	now := m.now()
	cutoff := now.Add(-m.cfg.Window).UnixNano()
	nowNano := now.UnixNano()

	e.mu.Lock()
	defer e.mu.Unlock()

	// Remove timestamps outside the current window using binary search.
	idx := sort.Search(len(e.timestamps), func(i int) bool {
		return e.timestamps[i] > cutoff
	})
	e.timestamps = e.timestamps[idx:]

	count := len(e.timestamps)
	if count < m.cfg.Limit {
		e.timestamps = append(e.timestamps, nowNano)
		return Result{
			Allowed:   true,
			Remaining: m.cfg.Limit - count - 1,
			ResetAt:   now.Add(m.cfg.Window),
		}, nil
	}

	// ResetAt is when the oldest in-window timestamp expires.
	var resetAt time.Time
	if len(e.timestamps) > 0 {
		resetAt = time.Unix(0, e.timestamps[0]).Add(m.cfg.Window)
	} else {
		resetAt = now.Add(m.cfg.Window)
	}

	return Result{
		Allowed:   false,
		Remaining: 0,
		ResetAt:   resetAt,
	}, nil
}

// Close stops the background cleanup goroutine. Safe to call multiple times.
func (m *Memory) Close() error {
	m.once.Do(func() { close(m.done) })
	return nil
}

func (m *Memory) loadOrStore(key string) *memEntry {
	val, _ := m.entries.LoadOrStore(key, &memEntry{})
	return val.(*memEntry) //nolint:forcetypeassert
}

// cleanup runs every window and evicts entries whose newest timestamp has
// already slid out of the window, preventing unbounded memory growth.
func (m *Memory) cleanup() {
	ticker := time.NewTicker(m.cfg.Window)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			cutoff := m.now().Add(-m.cfg.Window).UnixNano()
			m.entries.Range(func(k, v any) bool {
				e := v.(*memEntry) //nolint:forcetypeassert
				e.mu.Lock()
				if len(e.timestamps) == 0 || e.timestamps[len(e.timestamps)-1] <= cutoff {
					m.entries.Delete(k)
				}
				e.mu.Unlock()
				return true
			})
		}
	}
}
