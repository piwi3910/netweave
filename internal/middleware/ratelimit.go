// Package middleware provides HTTP middleware for rate limiting using Redis.
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// FailMode controls how the rate limiter behaves when the backing store (Redis)
// is unavailable or returns an error.
type FailMode string

const (
	// FailModeOpen allows the request when the rate limit check fails.
	// Use only for non-sensitive, read-only endpoints.
	FailModeOpen FailMode = "open"

	// FailModeClosed denies the request with 503 when the rate limit check fails.
	// This is the safe default for write endpoints in production.
	FailModeClosed FailMode = "closed"
)

// RateLimiterFailOpen tracks when rate limiting fails open due to Redis errors.
// Labels:
//   - reason: the failure classification (e.g. "redis_error", "invalid_result")
//   - endpoint: "<METHOD> <path>" of the request allowed by fail-open
var RateLimiterFailOpen = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "rate_limiter_fail_open_total",
		Help: "Total number of requests allowed because the rate limit check failed (fail-open)",
	},
	[]string{"reason", "endpoint"},
)

// RateLimiterFailClosed tracks when rate limiting fails closed due to Redis errors.
// Exposed for visibility into denials caused by backend outages.
var RateLimiterFailClosed = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "rate_limiter_fail_closed_total",
		Help: "Total number of requests denied because the rate limit check failed (fail-closed)",
	},
	[]string{"reason", "endpoint"},
)

// RateLimiter provides distributed rate limiting using Redis.
// It implements token bucket algorithm with sliding window for accurate limiting.
type RateLimiter struct {
	Client redis.UniversalClient // Exported for testing
	Logger *zap.Logger           // Exported for testing
	Config *RateLimitConfig      // Exported for testing
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

	// DefaultFailMode controls the behavior when the rate limit check fails
	// (e.g. Redis is down). If unset, defaults to FailModeClosed for write
	// methods (POST/PUT/PATCH/DELETE) and FailModeOpen for read methods,
	// matching the recommendation in issue #479.
	//
	// This default applies to any endpoint/tenant/global check that does
	// not specify its own FailMode via EndpointLimitConfig.
	DefaultFailMode FailMode
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
	// FailMode overrides the limiter's DefaultFailMode for this endpoint.
	// Leave empty to inherit the default.
	FailMode FailMode
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
		Client: config.RedisClient,
		Logger: logger,
		Config: config,
	}, nil
}

// Middleware returns a Gin middleware function for rate limiting.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.Config.Enabled {
			c.Next()
			return
		}

		ctx := c.Request.Context()

		// Extract tenant ID from context or use default
		tenantID := GetTenantID(c)

		// Check endpoint-specific limits first. Endpoint config may override
		// the default fail-mode (e.g. force fail-closed on write endpoints).
		if endpointLimit := rl.GetEndpointLimit(c.Request.Method, c.FullPath()); endpointLimit != nil {
			if !rl.checkLimit(ctx, c, fmt.Sprintf("endpoint:%s:%s:%s", tenantID, c.Request.Method, c.FullPath()),
				endpointLimit.RequestsPerSecond, endpointLimit.BurstSize, endpointLimit.FailMode) {
				return
			}
		}

		// Check per-tenant limits
		if rl.Config.PerTenant.RequestsPerSecond > 0 {
			if !rl.checkLimit(ctx, c, fmt.Sprintf("tenant:%s", tenantID),
				rl.Config.PerTenant.RequestsPerSecond, rl.Config.PerTenant.BurstSize, "") {
				return
			}
		}

		// Check global limits
		if rl.Config.Global.RequestsPerSecond > 0 {
			if !rl.checkLimit(ctx, c, "global",
				rl.Config.Global.RequestsPerSecond, rl.Config.Global.BurstSize(), "") {
				return
			}
		}

		c.Next()
	}
}

// resolveFailMode returns the effective fail mode for a request.
// Precedence: explicit override > DefaultFailMode > method-based default.
// Write methods (POST/PUT/PATCH/DELETE) default to fail-closed to satisfy
// the audit finding in issue #479; read methods default to fail-open.
func (rl *RateLimiter) resolveFailMode(method string, override FailMode) FailMode {
	if override == FailModeOpen || override == FailModeClosed {
		return override
	}
	if rl.Config.DefaultFailMode == FailModeOpen || rl.Config.DefaultFailMode == FailModeClosed {
		return rl.Config.DefaultFailMode
	}
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return FailModeClosed
	default:
		return FailModeOpen
	}
}

// endpointLabel returns a low-cardinality label safe for Prometheus.
// Uses FullPath (the matched route, e.g. "/tenants/:id") instead of
// the raw URL to avoid high-cardinality explosions from path params.
func endpointLabel(c *gin.Context) string {
	path := c.FullPath()
	if path == "" {
		path = "unmatched"
	}
	return c.Request.Method + " " + path
}

// handleCheckFailure decides whether to fail open (allow) or fail closed
// (deny with 503) when the rate limit backend cannot complete a check.
// It emits the rate_limiter_fail_open_total / rate_limiter_fail_closed_total
// metric with a low-cardinality endpoint label.
func (rl *RateLimiter) handleCheckFailure(c *gin.Context, failModeOverride FailMode, reason string) bool {
	mode := rl.resolveFailMode(c.Request.Method, failModeOverride)
	label := endpointLabel(c)

	if mode == FailModeClosed {
		RateLimiterFailClosed.WithLabelValues(reason, label).Inc()
		rl.Logger.Warn("rate limit check failed; failing closed",
			zap.String("reason", reason),
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.String("client_ip", c.ClientIP()),
		)
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"error":   "ServiceUnavailable",
			"message": "Rate limiting backend is unavailable; request denied (fail-closed).",
			"code":    http.StatusServiceUnavailable,
		})
		return false
	}

	// Fail-open path: emit the metric and allow the request through.
	RateLimiterFailOpen.WithLabelValues(reason, label).Inc()
	rl.Logger.Warn("rate limit check failed; failing open",
		zap.String("reason", reason),
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
		zap.String("client_ip", c.ClientIP()),
	)
	return true
}

// checkLimit checks if the request is within the rate limit using token bucket algorithm.
// Returns true if allowed, false if rate limit exceeded or the check failed closed.
// The failModeOverride parameter, when non-empty, forces the failure behavior
// for this specific check (e.g. an endpoint-level override). When empty, the
// limiter falls back to the configured default, which in turn defaults to
// fail-closed for write methods.
func (rl *RateLimiter) checkLimit(
	ctx context.Context, c *gin.Context, key string, requestsPerSecond, burstSize int, failModeOverride FailMode,
) bool {
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

	result, err := rl.Client.Eval(ctx, script, []string{key}, now, requestsPerSecond, burstSize, windowSize).Result()
	if err != nil {
		rl.Logger.Error("rate limit check failed",
			zap.String("key", key),
			zap.Error(err),
		)
		return rl.handleCheckFailure(c, failModeOverride, "redis_error")
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) < 3 {
		rl.Logger.Error("invalid rate limit result format",
			zap.String("key", key),
		)
		return rl.handleCheckFailure(c, failModeOverride, "invalid_result")
	}

	allowedVal, ok := resultSlice[0].(int64)
	if !ok {
		rl.Logger.Error("invalid rate limit allowed field type", zap.String("key", key))
		return rl.handleCheckFailure(c, failModeOverride, "invalid_result")
	}
	remainingVal, ok := resultSlice[1].(int64)
	if !ok {
		rl.Logger.Error("invalid rate limit remaining field type", zap.String("key", key))
		return rl.handleCheckFailure(c, failModeOverride, "invalid_result")
	}
	limitVal, ok := resultSlice[2].(int64)
	if !ok {
		rl.Logger.Error("invalid rate limit limit field type", zap.String("key", key))
		return rl.handleCheckFailure(c, failModeOverride, "invalid_result")
	}

	allowed := allowedVal == 1
	remaining := remainingVal
	limit := limitVal

	// Set rate limit headers
	c.Header("X-RateLimit-Limit", strconv.FormatInt(limit, 10))
	c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(now+windowSize, 10))

	if !allowed {
		c.Header("Retry-After", strconv.FormatInt(windowSize, 10))

		rl.Logger.Warn("rate limit exceeded",
			zap.String("key", key),
			zap.String("tenant", GetTenantID(c)),
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

// GetEndpointLimit returns the rate limit config for a specific endpoint if configured.
func (rl *RateLimiter) GetEndpointLimit(method, path string) *EndpointLimitConfig {
	for _, limit := range rl.Config.PerEndpoint {
		if limit.Method == method && limit.Path == path {
			return &limit
		}
	}
	return nil
}

// GetTenantID extracts the tenant ID from the Gin context.
// It first checks for a tenant ID in the context (set by auth middleware),
// then falls back to client IP as a default identifier.
func GetTenantID(c *gin.Context) string {
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
