package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const testRemoteAddr = "192.168.1.100:12345"

// TestNewRateLimiter tests rate limiter creation.
func TestNewRateLimiter(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { require.NoError(t, redisClient.Close()) }()

	logger := zap.NewNop()

	t.Run("valid creation", func(t *testing.T) {
		config := &RateLimitConfig{
			Enabled:     true,
			RedisClient: redisClient,
		}

		rl, err := NewRateLimiter(config, logger)
		require.NoError(t, err)
		assert.NotNil(t, rl)
		assert.Equal(t, redisClient, rl.client)
		assert.Equal(t, logger, rl.logger)
		assert.Equal(t, config, rl.config)
	})

	t.Run("nil config", func(t *testing.T) {
		rl, err := NewRateLimiter(nil, logger)
		assert.Error(t, err)
		assert.Nil(t, rl)
		assert.Contains(t, err.Error(), "config cannot be nil")
	})

	t.Run("nil redis client", func(t *testing.T) {
		config := &RateLimitConfig{
			Enabled: true,
		}

		rl, err := NewRateLimiter(config, logger)
		assert.Error(t, err)
		assert.Nil(t, rl)
		assert.Contains(t, err.Error(), "redis client cannot be nil")
	})

	t.Run("nil logger", func(t *testing.T) {
		config := &RateLimitConfig{
			Enabled:     true,
			RedisClient: redisClient,
		}

		rl, err := NewRateLimiter(config, nil)
		assert.Error(t, err)
		assert.Nil(t, rl)
		assert.Contains(t, err.Error(), "logger cannot be nil")
	})

	t.Run("redis connection failure", func(t *testing.T) {
		// Create a client with invalid address
		badClient := redis.NewClient(&redis.Options{
			Addr: "localhost:9999",
		})
		defer func() { require.NoError(t, badClient.Close()) }()

		config := &RateLimitConfig{
			Enabled:     true,
			RedisClient: badClient,
		}

		rl, err := NewRateLimiter(config, logger)
		assert.Error(t, err)
		assert.Nil(t, rl)
		assert.Contains(t, err.Error(), "redis connection failed")
	})
}

// TestMiddleware tests the rate limit middleware.
func TestMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { require.NoError(t, redisClient.Close()) }()

	logger := zap.NewNop()

	t.Run("disabled rate limiter allows all requests", func(t *testing.T) {
		config := &RateLimitConfig{
			Enabled:     false,
			RedisClient: redisClient,
		}

		rl, err := NewRateLimiter(config, logger)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		c, router := gin.CreateTestContext(w)
		router.Use(rl.Middleware())
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request = req
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("per-tenant rate limit", func(t *testing.T) {
		config := &RateLimitConfig{
			Enabled: true,
			PerTenant: TenantLimitConfig{
				RequestsPerSecond: 2,
				BurstSize:         2,
			},
			RedisClient: redisClient,
		}

		rl, err := NewRateLimiter(config, logger)
		require.NoError(t, err)

		router := gin.New()
		router.Use(rl.Middleware())
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		// First request should succeed
		w1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusOK, w1.Code)
		assert.Contains(t, w1.Header().Get("X-RateLimit-Limit"), "2")

		// Second request should succeed
		w2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusOK, w2.Code)
	})

	t.Run("endpoint-specific rate limit", func(t *testing.T) {
		config := &RateLimitConfig{
			Enabled: true,
			PerEndpoint: []EndpointLimitConfig{
				{
					Path:              "/limited",
					Method:            "GET",
					RequestsPerSecond: 1,
					BurstSize:         1,
				},
			},
			RedisClient: redisClient,
		}

		rl, err := NewRateLimiter(config, logger)
		require.NoError(t, err)

		router := gin.New()
		router.Use(rl.Middleware())
		router.GET("/limited", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		// First request should succeed
		w1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/limited", nil)
		router.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusOK, w1.Code)
	})

	t.Run("global rate limit", func(t *testing.T) {
		config := &RateLimitConfig{
			Enabled: true,
			Global: GlobalLimitConfig{
				RequestsPerSecond: 10,
			},
			RedisClient: redisClient,
		}

		rl, err := NewRateLimiter(config, logger)
		require.NoError(t, err)

		router := gin.New()
		router.Use(rl.Middleware())
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestGetEndpointLimit tests endpoint limit lookup.
func TestGetEndpointLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { require.NoError(t, redisClient.Close()) }()

	logger := zap.NewNop()

	config := &RateLimitConfig{
		Enabled: true,
		PerEndpoint: []EndpointLimitConfig{
			{
				Path:              "/api/v1/users",
				Method:            "GET",
				RequestsPerSecond: 10,
				BurstSize:         20,
			},
			{
				Path:              "/api/v1/users",
				Method:            "POST",
				RequestsPerSecond: 5,
				BurstSize:         10,
			},
		},
		RedisClient: redisClient,
	}

	rl, err := NewRateLimiter(config, logger)
	require.NoError(t, err)

	t.Run("finds matching endpoint", func(t *testing.T) {
		limit := rl.getEndpointLimit("GET", "/api/v1/users")
		require.NotNil(t, limit)
		assert.Equal(t, 10, limit.RequestsPerSecond)
		assert.Equal(t, 20, limit.BurstSize)
	})

	t.Run("finds different method", func(t *testing.T) {
		limit := rl.getEndpointLimit("POST", "/api/v1/users")
		require.NotNil(t, limit)
		assert.Equal(t, 5, limit.RequestsPerSecond)
		assert.Equal(t, 10, limit.BurstSize)
	})

	t.Run("returns nil for non-existent endpoint", func(t *testing.T) {
		limit := rl.getEndpointLimit("DELETE", "/api/v1/users")
		assert.Nil(t, limit)
	})

	t.Run("returns nil for non-existent path", func(t *testing.T) {
		limit := rl.getEndpointLimit("GET", "/api/v1/posts")
		assert.Nil(t, limit)
	})
}

// TestGetTenantID tests tenant ID extraction.
func TestGetTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("extracts tenant from context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("tenant_id", "tenant-123")

		tenantID := getTenantID(c)
		assert.Equal(t, "tenant-123", tenantID)
	})

	t.Run("falls back to client IP", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request.RemoteAddr = testRemoteAddr

		tenantID := getTenantID(c)
		assert.Contains(t, tenantID, "192.168.1.100")
	})

	t.Run("handles empty tenant ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("tenant_id", "")
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request.RemoteAddr = testRemoteAddr

		tenantID := getTenantID(c)
		assert.Contains(t, tenantID, "192.168.1.100")
	})

	t.Run("handles non-string tenant ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("tenant_id", 123)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request.RemoteAddr = testRemoteAddr

		tenantID := getTenantID(c)
		assert.Contains(t, tenantID, "192.168.1.100")
	})
}

// TestGlobalLimitConfig_BurstSize tests burst size calculation.
func TestGlobalLimitConfig_BurstSize(t *testing.T) {
	t.Run("returns double the rate", func(t *testing.T) {
		config := GlobalLimitConfig{
			RequestsPerSecond: 10,
		}
		assert.Equal(t, 20, config.BurstSize())
	})

	t.Run("returns 0 for 0 rate", func(t *testing.T) {
		config := GlobalLimitConfig{
			RequestsPerSecond: 0,
		}
		assert.Equal(t, 0, config.BurstSize())
	})

	t.Run("handles large rates", func(t *testing.T) {
		config := GlobalLimitConfig{
			RequestsPerSecond: 1000,
		}
		assert.Equal(t, 2000, config.BurstSize())
	})
}
