// Package ratelimit provides a sliding window rate limiter with two backends:
//
//   - [Memory]: in-process, zero-dependency, suitable for single-instance deployments.
//   - [Redis]: distributed, uses an atomic Lua script, suitable for multi-instance deployments.
//
// Both implement [Limiter] so callers can swap backends without changing middleware code.
//
// Usage:
//
//	limiter := ratelimit.New(ratelimit.Config{Limit: 100, Window: time.Minute})
//	defer limiter.Close()
//
//	result, err := limiter.Allow(ctx, "tenant:abc123")
//	if err != nil { /* infrastructure failure */ }
//	if !result.Allowed { /* return 429 */ }
package ratelimit

import (
	"context"
	"errors"
	"time"
)

// ErrRateLimited is returned as a sentinel when callers want to distinguish
// a rate-limit decision from an infrastructure error.
var ErrRateLimited = errors.New("rate limited")

// Config holds the parameters shared by all Limiter implementations.
type Config struct {
	// Limit is the maximum number of requests allowed per Window.
	Limit int
	// Window is the rolling duration over which Limit applies.
	Window time.Duration
}

// Result carries the outcome of an Allow call together with quota metadata
// that callers can surface as HTTP response headers.
type Result struct {
	// Allowed reports whether the request is permitted.
	Allowed bool
	// Remaining is the number of additional requests allowed in the current window.
	// Zero when Allowed is false.
	Remaining int
	// ResetAt is the earliest time at which the next rejected request may be retried.
	ResetAt time.Time
}

// Limiter is the common interface for all rate limiter backends.
type Limiter interface {
	// Allow checks whether one request identified by key should be permitted.
	// key should include both the resource scope and the client identifier,
	// e.g. "tenant:<uuid>" or "ip:1.2.3.4".
	//
	// An error is returned only on infrastructure failure (e.g. Redis unreachable).
	// A denied request is NOT an error; check Result.Allowed instead.
	Allow(ctx context.Context, key string) (Result, error)

	// Close releases background resources held by the limiter (goroutines,
	// network connections). It is safe to call Close multiple times.
	Close() error
}
