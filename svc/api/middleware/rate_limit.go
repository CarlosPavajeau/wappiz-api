package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
	"wappiz/pkg/jwt"
	"wappiz/pkg/logger"
	"wappiz/pkg/ratelimit"

	"github.com/gin-gonic/gin"
)

// KeyFunc extracts the rate-limit key from a Gin context.
// The returned string identifies the "bucket" for this request, e.g.
// "ip:1.2.3.4" or "tenant:abc-123".
type KeyFunc func(c *gin.Context) string

// KeyByIP partitions requests by client IP address.
func KeyByIP() KeyFunc {
	return func(c *gin.Context) string {
		return fmt.Sprintf("ip:%s", c.ClientIP())
	}
}

// KeyByTenantID partitions requests by the tenant ID extracted from the JWT.
// Falls back to the client IP when no tenant claim is present.
func KeyByTenantID() KeyFunc {
	return func(c *gin.Context) string {
		id, ok := jwt.TenantIDFromContextOK(c)
		if !ok {
			return fmt.Sprintf("ip:%s", c.ClientIP())
		}
		return fmt.Sprintf("tenant:%s", id)
	}
}

// KeyByUserID partitions requests by the user ID extracted from the JWT.
// Falls back to the client IP when no user claim is present.
func KeyByUserID() KeyFunc {
	return func(c *gin.Context) string {
		id := jwt.UserIDFromContext(c)
		if id == "" {
			return fmt.Sprintf("ip:%s", c.ClientIP())
		}
		return fmt.Sprintf("user:%s", id)
	}
}

// RateLimit returns a Gin middleware that enforces the given limiter using keyFn
// to derive the per-request bucket key.
//
// On infrastructure failure the middleware fails open (the request proceeds) and
// logs a warning so the issue is visible without causing an outage.
//
// Successful responses carry the standard rate-limit headers:
//
//	X-RateLimit-Limit     – configured request ceiling
//	X-RateLimit-Remaining – requests left in the current window
//	X-RateLimit-Reset     – Unix timestamp when the window resets
func RateLimit(limiter ratelimit.Limiter, keyFn KeyFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFn(c)

		res, err := limiter.Allow(c.Request.Context(), key)
		if err != nil {
			// Fail open: log and let the request through.
			logger.Warn("ratelimit: backend error, failing open", "key", key, "err", err)
			c.Next()
			return
		}

		limit := res.Remaining
		if !res.Allowed {
			limit = 0
		}

		c.Header("X-RateLimit-Remaining", strconv.Itoa(limit))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(res.ResetAt.Unix(), 10))

		if !res.Allowed {
			retryAfter := int(time.Until(res.ResetAt).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}

		c.Next()
	}
}
