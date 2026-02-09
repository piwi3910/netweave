package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
)

const (
	// defaultRateLimit is the default requests per minute if the tenant has no configured limit.
	defaultRateLimit = 1000

	// rateLimitWindow is the sliding window duration for rate limiting.
	rateLimitWindow = 1 * time.Minute
)

// RateLimitStore defines the interface for rate limit state storage.
type RateLimitStore interface {
	// IncrementAndCheck atomically increments a counter and returns whether
	// the request should be allowed. Returns (allowed, currentCount, error).
	IncrementAndCheck(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error)
}

// RateLimitConfigFunc retrieves the rate limit for a given tenant.
// It returns the maximum number of requests allowed per window.
type RateLimitConfigFunc func(ctx context.Context, tenantID string) int

// RateLimiter provides per-tenant rate limiting using a sliding window counter.
type RateLimiter struct {
	store    RateLimitStore
	logger   *zap.Logger
	getLimit RateLimitConfigFunc
}

// NewRateLimiter creates a new rate limiter with the given store, logger, and limit function.
// If getLimit is nil, all tenants are subject to the defaultRateLimit.
func NewRateLimiter(store RateLimitStore, logger *zap.Logger, getLimit RateLimitConfigFunc) *RateLimiter {
	if getLimit == nil {
		getLimit = func(_ context.Context, _ string) int { return defaultRateLimit }
	}
	return &RateLimiter{
		store:    store,
		logger:   logger,
		getLimit: getLimit,
	}
}

// Middleware returns a Gin middleware that enforces per-tenant rate limits.
// Platform admins are exempt from rate limiting. Requests without a tenant
// context are allowed through without rate limiting. On storage errors the
// middleware fails open (allows the request) to avoid cascading failures.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Platform admins are exempt from rate limiting.
		if auth.IsPlatformAdminFromContext(ctx) {
			c.Next()
			return
		}

		tenantID := auth.TenantIDFromContext(ctx)
		if tenantID == "" {
			// No tenant context - skip rate limiting (unauthenticated or system request).
			c.Next()
			return
		}

		limit := rl.getLimit(ctx, tenantID)
		if limit <= 0 {
			limit = defaultRateLimit
		}

		key := fmt.Sprintf("ratelimit:%s", tenantID)
		allowed, currentCount, err := rl.store.IncrementAndCheck(ctx, key, limit, rateLimitWindow)
		if err != nil {
			// Fail open - allow the request if rate limiter storage is unavailable.
			rl.logger.Error("rate limiter error, failing open",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			c.Next()
			return
		}

		remaining := max(0, limit-currentCount)

		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if !allowed {
			retryAfter := int(rateLimitWindow.Seconds())
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Header("X-RateLimit-Remaining", "0")

			rl.logger.Warn("rate limit exceeded",
				zap.String("tenant_id", tenantID),
				zap.Int("limit", limit),
				zap.Int("current", currentCount))

			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "RateLimitExceeded",
				"message": "Rate limit exceeded. Try again later.",
				"code":    http.StatusTooManyRequests,
			})
			return
		}

		c.Next()
	}
}

// RedisRateLimitStore implements RateLimitStore using Redis sliding window counters.
type RedisRateLimitStore struct {
	client redis.UniversalClient
}

// NewRedisRateLimitStore creates a new Redis-backed rate limit store.
func NewRedisRateLimitStore(client redis.UniversalClient) *RedisRateLimitStore {
	return &RedisRateLimitStore{client: client}
}

// rateLimitScript is a Lua script that atomically increments a counter and sets
// TTL on first increment. This prevents a race condition where INCR succeeds but
// EXPIRE fails (e.g., due to crash or network error), which would leave the key
// without a TTL and permanently block the tenant.
var rateLimitScript = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
    redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return count
`)

// IncrementAndCheck atomically increments a counter for the given key and checks
// whether the current count is within the limit. Uses a Lua script to ensure the
// INCR and PEXPIRE are executed atomically within Redis.
func (r *RedisRateLimitStore) IncrementAndCheck(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	result, err := rateLimitScript.Run(ctx, r.client, []string{key}, window.Milliseconds()).Int64()
	if err != nil {
		return false, 0, err
	}

	count := int(result)
	return count <= limit, count, nil
}
