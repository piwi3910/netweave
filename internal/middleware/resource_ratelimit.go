// Package middleware provides HTTP middleware for the O2-IMS Gateway.
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ResourceType represents the type of O2-IMS resource.
type ResourceType string

const (
	// ResourceTypeDeploymentManagers represents deployment manager resources.
	ResourceTypeDeploymentManagers ResourceType = "deploymentManagers"
	// ResourceTypeResourcePools represents resource pool resources.
	ResourceTypeResourcePools ResourceType = "resourcePools"
	// ResourceTypeResources represents individual resources.
	ResourceTypeResources ResourceType = "resources"
	// ResourceTypeResourceTypes represents resource type definitions.
	ResourceTypeResourceTypes ResourceType = "resourceTypes"
	// ResourceTypeSubscriptions represents subscription resources.
	ResourceTypeSubscriptions ResourceType = "subscriptions"
	// ResourceTypeUnknown represents an unknown resource type.
	ResourceTypeUnknown ResourceType = "unknown"
)

// DefaultMaxPageSize is the default maximum page size for list operations.
const DefaultMaxPageSize = 100

// Pre-compiled regex patterns for resource type extraction.
// These are compiled once at package initialization for performance.
var resourceTypePatterns = []struct {
	pattern      *regexp.Regexp
	resourceType ResourceType
}{
	{regexp.MustCompile(`/o2ims(?:-dms)?/v1/deploymentManagers`), ResourceTypeDeploymentManagers},
	{regexp.MustCompile(`/o2ims(?:-dms)?/v1/resourcePools`), ResourceTypeResourcePools},
	{regexp.MustCompile(`/o2ims(?:-dms)?/v1/resources`), ResourceTypeResources},
	{regexp.MustCompile(`/o2ims(?:-dms)?/v1/resourceTypes`), ResourceTypeResourceTypes},
	{regexp.MustCompile(`/o2ims(?:-dms)?/v1/subscriptions`), ResourceTypeSubscriptions},
	{regexp.MustCompile(`/o2ims(?:-dms)?/v1/[^/]+/resourcePools`), ResourceTypeResourcePools},
	{regexp.MustCompile(`/o2ims(?:-dms)?/v1/[^/]+/resources`), ResourceTypeResources},
	{regexp.MustCompile(`/o2ims(?:-dms)?/v1/[^/]+/deploymentManagers`), ResourceTypeDeploymentManagers},
}

// Pre-compiled regex patterns for collection path detection.
var collectionPathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`/deploymentManagers$`),
	regexp.MustCompile(`/resourcePools$`),
	regexp.MustCompile(`/resources$`),
	regexp.MustCompile(`/resourceTypes$`),
	regexp.MustCompile(`/subscriptions$`),
}

// OperationType represents the type of operation being performed.
type OperationType string

const (
	// OperationRead represents a read operation (GET).
	OperationRead OperationType = "read"
	// OperationList represents a list operation (GET on collection).
	OperationList OperationType = "list"
	// OperationWrite represents a write operation (POST, PUT, PATCH).
	OperationWrite OperationType = "write"
	// OperationDelete represents a delete operation (DELETE).
	OperationDelete OperationType = "delete"
)

// ResourceRateLimitConfig contains configuration for resource-type rate limiting.
type ResourceRateLimitConfig struct {
	// Enabled controls whether resource rate limiting is active
	Enabled bool

	// RedisClient is the Redis client for distributed limiting
	RedisClient redis.UniversalClient

	// DeploymentManagers configures limits for deployment manager operations
	DeploymentManagers ResourceTypeLimits

	// ResourcePools configures limits for resource pool operations
	ResourcePools ResourceTypeLimits

	// Resources configures limits for individual resource operations
	Resources ResourceTypeLimits

	// ResourceTypes configures limits for resource type operations
	ResourceTypes ResourceTypeLimits

	// Subscriptions configures limits for subscription operations
	Subscriptions SubscriptionLimits

	// DefaultLimits provides fallback limits for unknown resource types
	DefaultLimits ResourceTypeLimits
}

// ResourceTypeLimits defines rate limits for a resource type.
type ResourceTypeLimits struct {
	// ReadsPerMinute limits read operations per minute
	ReadsPerMinute int

	// WritesPerMinute limits write operations per minute
	WritesPerMinute int

	// ListPageSizeMax limits the maximum page size for list operations
	ListPageSizeMax int
}

// SubscriptionLimits defines rate limits specific to subscriptions.
type SubscriptionLimits struct {
	// CreatesPerHour limits subscription creations per hour
	CreatesPerHour int

	// MaxActive limits the maximum number of active subscriptions per tenant
	MaxActive int

	// ReadsPerMinute limits read operations per minute
	ReadsPerMinute int
}

// ResourceRateLimiter provides resource-type-specific rate limiting.
type ResourceRateLimiter struct {
	client  redis.UniversalClient
	logger  *zap.Logger
	config  *ResourceRateLimitConfig
	metrics *resourceRateLimitMetrics
}

type resourceRateLimitMetrics struct {
	hits     *prometheus.CounterVec
	failOpen *prometheus.CounterVec
}

// Prometheus metrics for resource rate limiting.
var resourceRateLimitHits = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "o2ims_resource_rate_limit_hits_total",
		Help: "Total number of resource rate limit hits",
	},
	[]string{"resource_type", "operation", "tenant"},
)

// resourceRateLimitFailOpen tracks when rate limiting fails open due to Redis errors.
var resourceRateLimitFailOpen = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "o2ims_resource_rate_limit_fail_open_total",
		Help: "Total number of requests allowed due to rate limit check failures (fail-open behavior)",
	},
	[]string{"resource_type", "operation", "tenant"},
)

// DefaultResourceRateLimitConfig returns sensible defaults for resource rate limiting.
func DefaultResourceRateLimitConfig() *ResourceRateLimitConfig {
	return &ResourceRateLimitConfig{
		Enabled: true,
		DeploymentManagers: ResourceTypeLimits{
			ReadsPerMinute:  100,
			WritesPerMinute: 10,
			ListPageSizeMax: 100,
		},
		ResourcePools: ResourceTypeLimits{
			ReadsPerMinute:  500,
			WritesPerMinute: 50,
			ListPageSizeMax: 100,
		},
		Resources: ResourceTypeLimits{
			ReadsPerMinute:  1000,
			WritesPerMinute: 100,
			ListPageSizeMax: 100,
		},
		ResourceTypes: ResourceTypeLimits{
			ReadsPerMinute:  500,
			WritesPerMinute: 10,
			ListPageSizeMax: 100,
		},
		Subscriptions: SubscriptionLimits{
			CreatesPerHour: 100,
			MaxActive:      50,
			ReadsPerMinute: 200,
		},
		DefaultLimits: ResourceTypeLimits{
			ReadsPerMinute:  100,
			WritesPerMinute: 10,
			ListPageSizeMax: 100,
		},
	}
}

// NewResourceRateLimiter creates a new resource-type rate limiter.
func NewResourceRateLimiter(
	config *ResourceRateLimitConfig,
	logger *zap.Logger,
) (*ResourceRateLimiter, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.RedisClient == nil {
		return nil, fmt.Errorf("redis client cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Validate rate limit configuration values
	if err := validateResourceRateLimitConfig(config); err != nil {
		return nil, fmt.Errorf("invalid rate limit configuration: %w", err)
	}

	// Warn about zero rate limit values that effectively disable limiting
	warnZeroLimits(logger, "DeploymentManagers", config.DeploymentManagers)
	warnZeroLimits(logger, "ResourcePools", config.ResourcePools)
	warnZeroLimits(logger, "Resources", config.Resources)
	warnZeroLimits(logger, "ResourceTypes", config.ResourceTypes)
	warnZeroSubscriptionLimits(logger, config.Subscriptions)
	warnZeroLimits(logger, "DefaultLimits", config.DefaultLimits)

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := config.RedisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &ResourceRateLimiter{
		client: config.RedisClient,
		logger: logger,
		config: config,
		metrics: &resourceRateLimitMetrics{
			hits:     resourceRateLimitHits,
			failOpen: resourceRateLimitFailOpen,
		},
	}, nil
}

// validateResourceRateLimitConfig validates rate limit configuration values.
func validateResourceRateLimitConfig(config *ResourceRateLimitConfig) error {
	// Validate DeploymentManagers limits
	if err := validateResourceTypeLimits("DeploymentManagers", config.DeploymentManagers); err != nil {
		return err
	}

	// Validate ResourcePools limits
	if err := validateResourceTypeLimits("ResourcePools", config.ResourcePools); err != nil {
		return err
	}

	// Validate Resources limits
	if err := validateResourceTypeLimits("Resources", config.Resources); err != nil {
		return err
	}

	// Validate ResourceTypes limits
	if err := validateResourceTypeLimits("ResourceTypes", config.ResourceTypes); err != nil {
		return err
	}

	// Validate Subscriptions limits
	if config.Subscriptions.CreatesPerHour < 0 {
		return fmt.Errorf("Subscriptions.CreatesPerHour cannot be negative")
	}
	if config.Subscriptions.MaxActive < 0 {
		return fmt.Errorf("Subscriptions.MaxActive cannot be negative")
	}
	if config.Subscriptions.ReadsPerMinute < 0 {
		return fmt.Errorf("Subscriptions.ReadsPerMinute cannot be negative")
	}

	// Validate DefaultLimits
	if err := validateResourceTypeLimits("DefaultLimits", config.DefaultLimits); err != nil {
		return err
	}

	return nil
}

// validateResourceTypeLimits validates a ResourceTypeLimits struct.
func validateResourceTypeLimits(name string, limits ResourceTypeLimits) error {
	if limits.ReadsPerMinute < 0 {
		return fmt.Errorf("%s.ReadsPerMinute cannot be negative", name)
	}
	if limits.WritesPerMinute < 0 {
		return fmt.Errorf("%s.WritesPerMinute cannot be negative", name)
	}
	if limits.ListPageSizeMax < 0 {
		return fmt.Errorf("%s.ListPageSizeMax cannot be negative", name)
	}
	return nil
}

// warnZeroLimits logs warnings for zero rate limit values that effectively disable limiting.
func warnZeroLimits(logger *zap.Logger, name string, limits ResourceTypeLimits) {
	if limits.ReadsPerMinute == 0 {
		logger.Warn("rate limit effectively disabled",
			zap.String("resource_type", name),
			zap.String("limit_type", "ReadsPerMinute"),
			zap.String("recommendation", "set explicit value or use Enabled=false"),
		)
	}
	if limits.WritesPerMinute == 0 {
		logger.Warn("rate limit effectively disabled",
			zap.String("resource_type", name),
			zap.String("limit_type", "WritesPerMinute"),
			zap.String("recommendation", "set explicit value or use Enabled=false"),
		)
	}
}

// warnZeroSubscriptionLimits logs warnings for zero subscription rate limit values.
func warnZeroSubscriptionLimits(logger *zap.Logger, limits SubscriptionLimits) {
	if limits.ReadsPerMinute == 0 {
		logger.Warn("rate limit effectively disabled",
			zap.String("resource_type", "Subscriptions"),
			zap.String("limit_type", "ReadsPerMinute"),
			zap.String("recommendation", "set explicit value or use Enabled=false"),
		)
	}
	if limits.CreatesPerHour == 0 {
		logger.Warn("rate limit effectively disabled",
			zap.String("resource_type", "Subscriptions"),
			zap.String("limit_type", "CreatesPerHour"),
			zap.String("recommendation", "set explicit value or use Enabled=false"),
		)
	}
}

// Middleware returns a Gin middleware for resource-type rate limiting.
func (rl *ResourceRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.config.Enabled {
			c.Next()
			return
		}

		ctx := c.Request.Context()
		tenantID := getResourceTenantID(c)
		resourceType := extractResourceType(c.FullPath())
		operation := extractOperation(c.Request.Method, c.FullPath())

		// Check page size for list operations
		if operation == OperationList {
			if !rl.checkPageSize(c, resourceType) {
				return
			}
		}

		// Check rate limits based on resource type
		if !rl.checkResourceLimit(ctx, c, tenantID, resourceType, operation) {
			return
		}

		c.Next()
	}
}

// checkPageSize validates that the requested page size is within limits.
func (rl *ResourceRateLimiter) checkPageSize(c *gin.Context, resourceType ResourceType) bool {
	pageSizeStr := c.Query("limit")
	if pageSizeStr == "" {
		return true
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil {
		return true // Let other validation handle invalid values
	}

	maxSize := rl.getMaxPageSize(resourceType)
	if pageSize > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "page size exceeds maximum",
			"max_size": maxSize,
			"received": pageSize,
		})
		c.Abort()
		return false
	}

	return true
}

// getMaxPageSize returns the maximum page size for a resource type.
func (rl *ResourceRateLimiter) getMaxPageSize(resourceType ResourceType) int {
	switch resourceType {
	case ResourceTypeDeploymentManagers:
		if rl.config.DeploymentManagers.ListPageSizeMax > 0 {
			return rl.config.DeploymentManagers.ListPageSizeMax
		}
	case ResourceTypeResourcePools:
		if rl.config.ResourcePools.ListPageSizeMax > 0 {
			return rl.config.ResourcePools.ListPageSizeMax
		}
	case ResourceTypeResources:
		if rl.config.Resources.ListPageSizeMax > 0 {
			return rl.config.Resources.ListPageSizeMax
		}
	case ResourceTypeResourceTypes:
		if rl.config.ResourceTypes.ListPageSizeMax > 0 {
			return rl.config.ResourceTypes.ListPageSizeMax
		}
	case ResourceTypeSubscriptions:
		return DefaultMaxPageSize // Subscriptions don't have a separate page size config
	default:
		if rl.config.DefaultLimits.ListPageSizeMax > 0 {
			return rl.config.DefaultLimits.ListPageSizeMax
		}
	}
	return DefaultMaxPageSize // Default fallback
}

// checkResourceLimit checks if the request is within the resource-specific rate limit.
func (rl *ResourceRateLimiter) checkResourceLimit(
	ctx context.Context,
	c *gin.Context,
	tenantID string,
	resourceType ResourceType,
	operation OperationType,
) bool {
	limit, window := rl.getLimits(resourceType, operation)
	if limit == 0 {
		return true // No limit configured
	}

	key := fmt.Sprintf("rate:%s:%s:%s", tenantID, resourceType, operation)

	allowed, remaining, err := rl.checkRedisLimit(ctx, key, limit, window)
	if err != nil {
		rl.logger.Error("resource rate limit check failed",
			zap.String("key", key),
			zap.Error(err),
		)
		// Record fail-open metric for observability
		rl.metrics.failOpen.WithLabelValues(string(resourceType), string(operation), tenantID).Inc()
		// Fail open: allow request if Redis fails
		return true
	}

	// Set rate limit headers
	c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(window).Unix(), 10))
	c.Header("X-RateLimit-Resource", string(resourceType))

	if !allowed {
		retryAfter := int(window.Seconds())
		c.Header("Retry-After", strconv.Itoa(retryAfter))

		rl.logger.Warn("resource rate limit exceeded",
			zap.String("tenant", tenantID),
			zap.String("resourceType", string(resourceType)),
			zap.String("operation", string(operation)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.String("client_ip", c.ClientIP()),
		)

		// Record metric
		rl.metrics.hits.WithLabelValues(string(resourceType), string(operation), tenantID).Inc()

		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":         "resource rate limit exceeded",
			"resource_type": resourceType,
			"operation":     operation,
			"retry_after":   retryAfter,
		})
		c.Abort()
		return false
	}

	return true
}

// getLimits returns the rate limit and window for a resource type and operation.
func (rl *ResourceRateLimiter) getLimits(
	resourceType ResourceType,
	operation OperationType,
) (int, time.Duration) {
	switch resourceType {
	case ResourceTypeDeploymentManagers:
		return rl.getTypeLimits(rl.config.DeploymentManagers, operation)
	case ResourceTypeResourcePools:
		return rl.getTypeLimits(rl.config.ResourcePools, operation)
	case ResourceTypeResources:
		return rl.getTypeLimits(rl.config.Resources, operation)
	case ResourceTypeResourceTypes:
		return rl.getTypeLimits(rl.config.ResourceTypes, operation)
	case ResourceTypeSubscriptions:
		return rl.getSubscriptionLimits(operation)
	default:
		return rl.getTypeLimits(rl.config.DefaultLimits, operation)
	}
}

// getTypeLimits returns the limit and window for standard resource types.
func (rl *ResourceRateLimiter) getTypeLimits(
	limits ResourceTypeLimits,
	operation OperationType,
) (int, time.Duration) {
	switch operation {
	case OperationRead, OperationList:
		return limits.ReadsPerMinute, time.Minute
	case OperationWrite:
		return limits.WritesPerMinute, time.Minute
	case OperationDelete:
		return limits.WritesPerMinute, time.Minute
	default:
		return limits.ReadsPerMinute, time.Minute
	}
}

// getSubscriptionLimits returns limits specific to subscriptions.
func (rl *ResourceRateLimiter) getSubscriptionLimits(operation OperationType) (int, time.Duration) {
	switch operation {
	case OperationRead, OperationList:
		return rl.config.Subscriptions.ReadsPerMinute, time.Minute
	case OperationWrite:
		return rl.config.Subscriptions.CreatesPerHour, time.Hour
	case OperationDelete:
		return rl.config.Subscriptions.CreatesPerHour, time.Hour
	default:
		return rl.config.Subscriptions.ReadsPerMinute, time.Minute
	}
}

// checkRedisLimit performs the rate limit check using Redis.
func (rl *ResourceRateLimiter) checkRedisLimit(
	ctx context.Context,
	key string,
	limit int,
	window time.Duration,
) (bool, int, error) {
	windowSeconds := int64(window.Seconds())
	now := time.Now().Unix()

	// Lua script for sliding window rate limiting
	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local limit = tonumber(ARGV[2])
		local window = tonumber(ARGV[3])

		-- Remove old entries outside the window
		redis.call('ZREMRANGEBYSCORE', key, 0, now - window)

		-- Count current requests in window
		local current = redis.call('ZCARD', key)

		if current < limit then
			-- Add the current request
			redis.call('ZADD', key, now, now .. ':' .. math.random())
			redis.call('EXPIRE', key, window)
			return {1, limit - current - 1}
		else
			return {0, 0}
		end
	`

	result, err := rl.client.Eval(ctx, script, []string{key}, now, limit, windowSeconds).Result()
	if err != nil {
		return false, 0, fmt.Errorf("redis eval failed: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) < 2 {
		return false, 0, fmt.Errorf("invalid redis result format")
	}

	allowed := resultSlice[0].(int64) == 1
	remaining := int(resultSlice[1].(int64))

	return allowed, remaining, nil
}

// extractResourceType determines the resource type from the request path.
func extractResourceType(path string) ResourceType {
	// O2-IMS API paths follow the pattern: /o2ims/v1/{resourceType}/...
	// Uses pre-compiled regex patterns for performance.
	for _, p := range resourceTypePatterns {
		if p.pattern.MatchString(path) {
			return p.resourceType
		}
	}

	return ResourceTypeUnknown
}

// extractOperation determines the operation type from the HTTP method and path.
func extractOperation(method, path string) OperationType {
	switch method {
	case http.MethodGet:
		// Check if it's a list operation (collection endpoint)
		if isCollectionPath(path) {
			return OperationList
		}
		return OperationRead
	case http.MethodPost:
		return OperationWrite
	case http.MethodPut, http.MethodPatch:
		return OperationWrite
	case http.MethodDelete:
		return OperationDelete
	default:
		return OperationRead
	}
}

// isCollectionPath determines if a path is a collection endpoint.
func isCollectionPath(path string) bool {
	// Collection paths end with the resource type name, not an ID.
	// Uses pre-compiled regex patterns for performance.
	for _, pattern := range collectionPathPatterns {
		if pattern.MatchString(path) {
			return true
		}
	}

	return false
}

// getResourceTenantID extracts the tenant ID for resource rate limiting.
func getResourceTenantID(c *gin.Context) string {
	// Try to get tenant from auth context
	if tenantID, exists := c.Get("tenant_id"); exists {
		if id, ok := tenantID.(string); ok && id != "" {
			return id
		}
	}

	// Try to get from X-Tenant-ID header
	if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" {
		return tenantID
	}

	// Fallback to client IP
	return strings.ReplaceAll(c.ClientIP(), ":", "_")
}
