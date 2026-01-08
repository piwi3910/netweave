// Package middleware provides HTTP middleware for rate limiting using Redis.
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RateLimiter provides distributed rate limiting using Redis.
// It implements token bucket algorithm with sliding window for accurate limiting.
type RateLimiter struct {
	client redis.UniversalClient
	logger *zap.Logger
	config *RateLimitConfig
}

// RateLimitConfig contains rate limiting configuration.
type RateLimitConfig struct {
	// Enabled controls whether rate limiting is active
	Enabled bool

	// PerTenant configures per-tenant rate limits
	PerTenant TenantLimitConfig

	// PerEndpoint configures per-endpoint rate limits
	PerEndpoint []EndpointLimitConfig

	// Global configures global rate limits
	Global GlobalLimitConfig

	// RedisClient is the Redis client for distributed limiting
	RedisClient redis.UniversalClient
}

// TenantLimitConfig configures per-tenant rate limits.
type TenantLimitConfig struct {
	RequestsPerSecond int
	BurstSize         int
}

// EndpointLimitConfig configures rate limits for specific endpoints.
type EndpointLimitConfig struct {
	Path              string
	Method            string
	RequestsPerSecond int
	BurstSize         int
}

// GlobalLimitConfig configures global rate limits.
type GlobalLimitConfig struct {
	RequestsPerSecond     int
	MaxConcurrentRequests int
}

// NewRateLimiter creates a new rate limiter with the given configuration.
func NewRateLimiter(config *RateLimitConfig, logger *zap.Logger) (*RateLimiter, error) {
	if config == nil {
		return nil, fmt.Errorf("rate limit config cannot be nil")
	}
	if config.RedisClient == nil {
		return nil, fmt.Errorf("redis client cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := config.RedisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RateLimiter{
		client: config.RedisClient,
		logger: logger,
		config: config,
	}, nil
}

// Middleware returns a Gin middleware function for rate limiting.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.config.Enabled {
			c.Next()
			return
		}

		ctx := c.Request.Context()

		// Extract tenant ID from context or use default
		tenantID := getTenantID(c)

		// Check endpoint-specific limits first
		if endpointLimit := rl.getEndpointLimit(c.Request.Method, c.FullPath()); endpointLimit != nil {
			if !rl.checkLimit(ctx, c, fmt.Sprintf("endpoint:%s:%s:%s", tenantID, c.Request.Method, c.FullPath()),
				endpointLimit.RequestsPerSecond, endpointLimit.BurstSize) {
				return
			}
		}

		// Check per-tenant limits
		if rl.config.PerTenant.RequestsPerSecond > 0 {
			if !rl.checkLimit(ctx, c, fmt.Sprintf("tenant:%s", tenantID),
				rl.config.PerTenant.RequestsPerSecond, rl.config.PerTenant.BurstSize) {
				return
			}
		}

		// Check global limits
		if rl.config.Global.RequestsPerSecond > 0 {
			if !rl.checkLimit(ctx, c, "global",
				rl.config.Global.RequestsPerSecond, rl.config.Global.BurstSize()) {
				return
			}
		}

		c.Next()
	}
}

// checkLimit checks if the request is within the rate limit using token bucket algorithm.
// Returns true if allowed, false if rate limit exceeded.
func (rl *RateLimiter) checkLimit(ctx context.Context, c *gin.Context, key string, requestsPerSecond, burstSize int) bool {
	now := time.Now().Unix()
	windowSize := int64(1) // 1 second window

	// Lua script for atomic token bucket check and update
	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local rate = tonumber(ARGV[2])
		local burst = tonumber(ARGV[3])
		local window = tonumber(ARGV[4])

		local tokens_key = key .. ":tokens"
		local timestamp_key = key .. ":ts"

		-- Get current tokens and last update time
		local tokens = tonumber(redis.call('GET', tokens_key) or burst)
		local last_update = tonumber(redis.call('GET', timestamp_key) or now)

		-- Calculate tokens to add based on time elapsed
		local elapsed = now - last_update
		local tokens_to_add = elapsed * rate
		tokens = math.min(burst, tokens + tokens_to_add)

		-- Check if we have tokens available
		if tokens >= 1 then
			tokens = tokens - 1
			redis.call('SET', tokens_key, tokens, 'EX', window * 2)
			redis.call('SET', timestamp_key, now, 'EX', window * 2)
			return {1, tokens, burst}
		else
			return {0, 0, burst}
		end
	`

	result, err := rl.client.Eval(ctx, script, []string{key}, now, requestsPerSecond, burstSize, windowSize).Result()
	if err != nil {
		rl.logger.Error("rate limit check failed",
			zap.String("key", key),
			zap.Error(err),
		)
		// Fail open: allow request if Redis fails
		return true
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) < 3 {
		rl.logger.Error("invalid rate limit result format")
		return true
	}

	allowed := resultSlice[0].(int64) == 1
	remaining := resultSlice[1].(int64)
	limit := resultSlice[2].(int64)

	// Set rate limit headers
	c.Header("X-RateLimit-Limit", strconv.FormatInt(limit, 10))
	c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(now+windowSize, 10))

	if !allowed {
		c.Header("Retry-After", strconv.FormatInt(windowSize, 10))

		rl.logger.Warn("rate limit exceeded",
			zap.String("key", key),
			zap.String("tenant", getTenantID(c)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.String("client_ip", c.ClientIP()),
		)

		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":       "rate limit exceeded",
			"retry_after": windowSize,
		})
		c.Abort()
		return false
	}

	return true
}

// getEndpointLimit returns the rate limit config for a specific endpoint if configured.
func (rl *RateLimiter) getEndpointLimit(method, path string) *EndpointLimitConfig {
	for _, limit := range rl.config.PerEndpoint {
		if limit.Method == method && limit.Path == path {
			return &limit
		}
	}
	return nil
}

// getTenantID extracts the tenant ID from the Gin context.
// It first checks for a tenant ID in the context (set by auth middleware),
// then falls back to client IP as a default identifier.
func getTenantID(c *gin.Context) string {
	// Try to get tenant from auth context
	if tenantID, exists := c.Get("tenant_id"); exists {
		if id, ok := tenantID.(string); ok && id != "" {
			return id
		}
	}

	// Fallback to client IP
	return c.ClientIP()
}

// BurstSize returns the burst size for global limits.
// If not explicitly set, it defaults to 2x the requests per second.
func (g *GlobalLimitConfig) BurstSize() int {
	if g.RequestsPerSecond == 0 {
		return 0
	}
	// Default burst size is 2x the rate
	return g.RequestsPerSecond * 2
}
