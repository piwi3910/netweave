package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/middleware"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestExtractResourceType(t *testing.T) {
	tests := []struct {
		path     string
		expected middleware.ResourceType
	}{
		{"/o2ims/v1/deploymentManagers", middleware.ResourceTypeDeploymentManagers},
		{"/o2ims/v1/deploymentManagers/dm-123", middleware.ResourceTypeDeploymentManagers},
		{"/o2ims/v1/resourcePools", middleware.ResourceTypeResourcePools},
		{"/o2ims/v1/resourcePools/pool-123", middleware.ResourceTypeResourcePools},
		{"/o2ims/v1/resources", middleware.ResourceTypeResources},
		{"/o2ims/v1/resources/res-123", middleware.ResourceTypeResources},
		{"/o2ims/v1/resourceTypes", middleware.ResourceTypeResourceTypes},
		{"/o2ims/v1/resourceTypes/type-123", middleware.ResourceTypeResourceTypes},
		{"/o2ims/v1/subscriptions", middleware.ResourceTypeSubscriptions},
		{"/o2ims/v1/subscriptions/sub-123", middleware.ResourceTypeSubscriptions},
		{"/o2ims-dms/v1/deploymentManagers", middleware.ResourceTypeDeploymentManagers},
		{"/o2ims/v1/dm-123/resourcePools", middleware.ResourceTypeResourcePools},
		{"/o2ims/v1/dm-123/resources", middleware.ResourceTypeResources},
		{"/o2ims/v1/unknown-endpoint", middleware.ResourceTypeUnknown},
		{"/health", middleware.ResourceTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := middleware.ExtractResourceType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractOperation(t *testing.T) {
	tests := []struct {
		method   string
		path     string
		expected middleware.OperationType
	}{
		{http.MethodGet, "/o2ims/v1/deploymentManagers", middleware.OperationList},
		{http.MethodGet, "/o2ims/v1/deploymentManagers/dm-123", middleware.OperationRead},
		{http.MethodGet, "/o2ims/v1/resourcePools", middleware.OperationList},
		{http.MethodGet, "/o2ims/v1/resourcePools/pool-123", middleware.OperationRead},
		{http.MethodGet, "/o2ims/v1/subscriptions", middleware.OperationList},
		{http.MethodGet, "/o2ims/v1/subscriptions/sub-123", middleware.OperationRead},
		{http.MethodPost, "/o2ims/v1/subscriptions", middleware.OperationWrite},
		{http.MethodPut, "/o2ims/v1/subscriptions/sub-123", middleware.OperationWrite},
		{http.MethodPatch, "/o2ims/v1/resources/res-123", middleware.OperationWrite},
		{http.MethodDelete, "/o2ims/v1/subscriptions/sub-123", middleware.OperationDelete},
	}

	for _, tt := range tests {
		t.Run(tt.method+"_"+tt.path, func(t *testing.T) {
			result := middleware.ExtractOperation(tt.method, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCollectionPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/o2ims/v1/deploymentManagers", true},
		{"/o2ims/v1/deploymentManagers/dm-123", false},
		{"/o2ims/v1/resourcePools", true},
		{"/o2ims/v1/resourcePools/pool-123", false},
		{"/o2ims/v1/resources", true},
		{"/o2ims/v1/resources/res-123", false},
		{"/o2ims/v1/subscriptions", true},
		{"/o2ims/v1/subscriptions/sub-123", false},
		{"/health", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := middleware.IsCollectionPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultResourceRateLimitConfig(t *testing.T) {
	config := middleware.DefaultResourceRateLimitConfig()

	assert.True(t, config.Enabled)

	// DeploymentManagers limits
	assert.Equal(t, 100, config.DeploymentManagers.ReadsPerMinute)
	assert.Equal(t, 10, config.DeploymentManagers.WritesPerMinute)
	assert.Equal(t, 100, config.DeploymentManagers.ListPageSizeMax)

	// ResourcePools limits
	assert.Equal(t, 500, config.ResourcePools.ReadsPerMinute)
	assert.Equal(t, 50, config.ResourcePools.WritesPerMinute)
	assert.Equal(t, 100, config.ResourcePools.ListPageSizeMax)

	// Resources limits
	assert.Equal(t, 1000, config.Resources.ReadsPerMinute)
	assert.Equal(t, 100, config.Resources.WritesPerMinute)
	assert.Equal(t, 100, config.Resources.ListPageSizeMax)

	// Subscriptions limits
	assert.Equal(t, 100, config.Subscriptions.CreatesPerHour)
	assert.Equal(t, 50, config.Subscriptions.MaxActive)
	assert.Equal(t, 200, config.Subscriptions.ReadsPerMinute)
}

func TestGetResourceTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		setupContext  func(*gin.Context)
		expectedID    string
		shouldContain string
	}{
		{
			name: "tenant from context",
			setupContext: func(c *gin.Context) {
				c.Set("tenant_id", "tenant-123")
			},
			expectedID: "tenant-123",
		},
		{
			name: "tenant from header",
			setupContext: func(c *gin.Context) {
				c.Request.Header.Set("X-Tenant-ID", "tenant-456")
			},
			expectedID: "tenant-456",
		},
		{
			name: "fallback to client IP",
			setupContext: func(_ *gin.Context) {
				// No tenant set
			},
			shouldContain: ".", // IP address contains dots
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

			tt.setupContext(c)

			result := middleware.GetResourceTenantID(c)

			if tt.expectedID != "" {
				assert.Equal(t, tt.expectedID, result)
			}
			if tt.shouldContain != "" {
				assert.Contains(t, result, tt.shouldContain)
			}
		})
	}
}

func TestResourceRateLimiter_GetMaxPageSize(t *testing.T) {
	config := middleware.DefaultResourceRateLimitConfig()
	rl := &middleware.ResourceRateLimiter{Config: config}

	tests := []struct {
		resourceType middleware.ResourceType
		expected     int
	}{
		{middleware.ResourceTypeDeploymentManagers, 100},
		{middleware.ResourceTypeResourcePools, 100},
		{middleware.ResourceTypeResources, 100},
		{middleware.ResourceTypeResourceTypes, 100},
		{middleware.ResourceTypeSubscriptions, 100},
		{middleware.ResourceTypeUnknown, 100},
	}

	for _, tt := range tests {
		t.Run(string(tt.resourceType), func(t *testing.T) {
			result := rl.GetMaxPageSize(tt.resourceType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResourceRateLimiter_CheckPageSize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := middleware.DefaultResourceRateLimitConfig()
	rl := &middleware.ResourceRateLimiter{Config: config}

	tests := []struct {
		name         string
		query        string
		resourceType middleware.ResourceType
		expected     bool
	}{
		{
			name:         "no limit param",
			query:        "",
			resourceType: middleware.ResourceTypeResources,
			expected:     true,
		},
		{
			name:         "limit within max",
			query:        "limit=50",
			resourceType: middleware.ResourceTypeResources,
			expected:     true,
		},
		{
			name:         "limit at max",
			query:        "limit=100",
			resourceType: middleware.ResourceTypeResources,
			expected:     true,
		},
		{
			name:         "limit exceeds max",
			query:        "limit=200",
			resourceType: middleware.ResourceTypeResources,
			expected:     false,
		},
		{
			name:         "invalid limit",
			query:        "limit=abc",
			resourceType: middleware.ResourceTypeResources,
			expected:     true, // Let other validation handle
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/o2ims/v1/resources"
			if tt.query != "" {
				url += "?" + tt.query
			}
			c.Request = httptest.NewRequest(http.MethodGet, url, nil)

			result := rl.CheckPageSize(c, tt.resourceType)
			assert.Equal(t, tt.expected, result)

			if !tt.expected {
				assert.Equal(t, http.StatusBadRequest, w.Code)
			}
		})
	}
}

// Redis integration tests

func TestNewResourceRateLimiter(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { _ = redisClient.Close() }()

	logger := zap.NewNop()

	t.Run("valid creation", func(t *testing.T) {
		config := middleware.DefaultResourceRateLimitConfig()
		config.RedisClient = redisClient

		rl, err := middleware.NewResourceRateLimiter(config, logger)
		require.NoError(t, err)
		assert.NotNil(t, rl)
	})

	t.Run("nil config", func(t *testing.T) {
		rl, err := middleware.NewResourceRateLimiter(nil, logger)
		assert.Error(t, err)
		assert.Nil(t, rl)
		assert.Contains(t, err.Error(), "config cannot be nil")
	})

	t.Run("nil redis client", func(t *testing.T) {
		config := middleware.DefaultResourceRateLimitConfig()
		config.RedisClient = nil

		rl, err := middleware.NewResourceRateLimiter(config, logger)
		assert.Error(t, err)
		assert.Nil(t, rl)
		assert.Contains(t, err.Error(), "redis client cannot be nil")
	})

	t.Run("nil logger", func(t *testing.T) {
		config := middleware.DefaultResourceRateLimitConfig()
		config.RedisClient = redisClient

		rl, err := middleware.NewResourceRateLimiter(config, nil)
		assert.Error(t, err)
		assert.Nil(t, rl)
		assert.Contains(t, err.Error(), "logger cannot be nil")
	})

	t.Run("negative rate limit values", func(t *testing.T) {
		config := middleware.DefaultResourceRateLimitConfig()
		config.RedisClient = redisClient
		config.DeploymentManagers.ReadsPerMinute = -1

		rl, err := middleware.NewResourceRateLimiter(config, logger)
		assert.Error(t, err)
		assert.Nil(t, rl)
		assert.Contains(t, err.Error(), "cannot be negative")
	})
}

func TestResourceRateLimiter_SlidingWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { _ = redisClient.Close() }()

	logger := zap.NewNop()

	config := middleware.DefaultResourceRateLimitConfig()
	config.RedisClient = redisClient
	// Set a very low limit for testing
	config.DeploymentManagers.ReadsPerMinute = 3

	rl, err := middleware.NewResourceRateLimiter(config, logger)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)

	// Helper to make requests
	makeRequest := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/o2ims/v1/deploymentManagers/dm-123", nil)
		c.Set("tenant_id", "test-tenant")

		mw := rl.Middleware()
		mw(c)
		return w
	}

	// First 3 requests should succeed
	for i := 0; i < 3; i++ {
		w := makeRequest()
		assert.NotEqual(t, http.StatusTooManyRequests, w.Code, "Request %d should succeed", i+1)
	}

	// 4th request should be rate limited
	w := makeRequest()
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "Request 4 should be rate limited")
	assert.Contains(t, w.Body.String(), "resource rate limit exceeded")
}

func TestResourceRateLimiter_RateLimitHeaders(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { _ = redisClient.Close() }()

	logger := zap.NewNop()

	config := middleware.DefaultResourceRateLimitConfig()
	config.RedisClient = redisClient
	config.Resources.ReadsPerMinute = 10

	rl, err := middleware.NewResourceRateLimiter(config, logger)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/o2ims/v1/resources/res-123", nil)
	c.Set("tenant_id", "test-tenant")

	mw := rl.Middleware()
	mw(c)

	// Check rate limit headers are set
	assert.Equal(t, "10", w.Header().Get("X-RateLimit-Limit"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
	assert.Equal(t, "resources", w.Header().Get("X-RateLimit-Resource"))
}

func TestResourceRateLimiter_FailOpen(t *testing.T) {
	// Create a miniredis instance and then close it to simulate failure
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close()

	// Create client pointing to closed server
	redisClient := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	defer func() { _ = redisClient.Close() }()

	logger := zap.NewNop()

	// Manually create the rate limiter without the connection check
	config := middleware.DefaultResourceRateLimitConfig()
	config.RedisClient = redisClient

	rl := &middleware.ResourceRateLimiter{
		Client: redisClient,
		Logger: logger,
		Config: config,
		Metrics: &middleware.ResourceRateLimitMetrics{
			Hits:     middleware.ResourceRateLimitHits,
			FailOpen: middleware.ResourceRateLimitFailOpen,
		},
	}

	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/o2ims/v1/resources/res-123", nil)
	c.Set("tenant_id", "test-tenant")

	mw := rl.Middleware()
	mw(c)

	// Request should succeed (fail-open behavior)
	assert.NotEqual(t, http.StatusTooManyRequests, w.Code, "Should fail open when Redis is unavailable")
}

func TestResourceRateLimiter_Disabled(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { _ = redisClient.Close() }()

	logger := zap.NewNop()

	config := middleware.DefaultResourceRateLimitConfig()
	config.RedisClient = redisClient
	config.Enabled = false

	rl, err := middleware.NewResourceRateLimiter(config, logger)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)

	// Make many requests - none should be rate limited
	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/o2ims/v1/resources/res-123", nil)
		c.Set("tenant_id", "test-tenant")

		mw := rl.Middleware()
		mw(c)

		assert.NotEqual(t, http.StatusTooManyRequests, w.Code, "Request %d should not be rate limited when disabled", i+1)
	}
}
