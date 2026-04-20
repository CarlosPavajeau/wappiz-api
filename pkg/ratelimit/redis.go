package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// slidingWindowScript implements an atomic sliding window using a Redis sorted
// set. Each member is a unique string "<ms>:<seq>" so two requests in the same
// millisecond can coexist. The TTL mirrors the window so keys self-expire.
//
// KEYS[1] – rate limit key (e.g. "rl:tenant:abc123")
// ARGV[1] – current Unix time in milliseconds
// ARGV[2] – window size in milliseconds
// ARGV[3] – request limit
//
// Returns {allowed (0|1), remaining, resetAtMs}
var slidingWindowScript = redis.NewScript(`
local key      = KEYS[1]
local now      = tonumber(ARGV[1])
local windowMs = tonumber(ARGV[2])
local limit    = tonumber(ARGV[3])

redis.call('ZREMRANGEBYSCORE', key, '-inf', now - windowMs)

local count = tonumber(redis.call('ZCARD', key))
if count < limit then
    local seq = redis.call('INCR', key .. ':seq')
    redis.call('ZADD', key, now, now .. ':' .. seq)
    redis.call('PEXPIRE', key, windowMs)
    return {1, limit - count - 1, now + windowMs}
end

local oldest = tonumber(redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')[2])
local resetAt = oldest and (oldest + windowMs) or (now + windowMs)
return {0, 0, resetAt}
`)

// Redis is a distributed sliding window rate limiter backed by Redis.
// It uses a single-roundtrip Lua script for atomicity — no MULTI/EXEC needed.
//
// Construct via [NewRedis]; the zero value is not usable.
type Redis struct {
	cfg    Config
	client *redis.Client
	once   sync.Once
}

// NewRedis creates a Redis-backed sliding window limiter.
// The caller owns client and is responsible for closing it after [Redis.Close].
func NewRedis(cfg Config, client *redis.Client) *Redis {
	return &Redis{
		cfg:    cfg,
		client: client,
	}
}

// Allow implements [Limiter]. It is safe for concurrent use and works correctly
// across multiple application instances sharing the same Redis instance.
func (r *Redis) Allow(ctx context.Context, key string) (Result, error) {
	nowMs := r.now().UnixMilli()
	windowMs := r.cfg.Window.Milliseconds()

	vals, err := slidingWindowScript.Run(
		ctx, r.client,
		[]string{fmt.Sprintf("rl:%s", key)},
		nowMs, windowMs, r.cfg.Limit,
	).Int64Slice()
	if err != nil {
		return Result{}, fmt.Errorf("ratelimit redis: %w", err)
	}
	if len(vals) != 3 {
		return Result{}, fmt.Errorf("ratelimit redis: unexpected script result length %d", len(vals))
	}

	allowed := vals[0] == 1
	remaining := int(vals[1])
	resetAt := time.UnixMilli(vals[2])

	return Result{
		Allowed:   allowed,
		Remaining: remaining,
		ResetAt:   resetAt,
	}, nil
}

// Close is a no-op for the Redis limiter — the caller owns the client lifetime.
// It exists solely to satisfy the [Limiter] interface.
func (r *Redis) Close() error {
	return nil
}

func (r *Redis) now() time.Time {
	return time.Now()
}
