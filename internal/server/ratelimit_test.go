package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
)

// mockRateLimitStore implements RateLimitStore for testing.
type mockRateLimitStore struct {
	allowed      bool
	currentCount int
	err          error
	callCount    int
}

func (m *mockRateLimitStore) IncrementAndCheck(_ context.Context, _ string, _ int, _ time.Duration) (bool, int, error) {
	m.callCount++
	return m.allowed, m.currentCount, m.err
}

// setTenantContext is a test helper that injects an AuthenticatedUser into the request context.
func setTenantContext(tenantID string, isPlatformAdmin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := &auth.AuthenticatedUser{
			UserID:          "user-1",
			TenantID:        tenantID,
			IsPlatformAdmin: isPlatformAdmin,
		}
		ctx := auth.ContextWithUser(c.Request.Context(), user)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// okHandler is a simple handler that returns 200 OK for testing middleware.
func okHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func TestRateLimiter_AllowsRequestUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &mockRateLimitStore{allowed: true, currentCount: 5}
	rl := NewRateLimiter(store, zap.NewNop(), nil)

	router := gin.New()
	router.Use(setTenantContext("tenant-1", false))
	router.Use(rl.Middleware())
	router.GET("/test", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "1000", w.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "995", w.Header().Get("X-RateLimit-Remaining"))
	assert.Equal(t, 1, store.callCount)
}

func TestRateLimiter_RejectsOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &mockRateLimitStore{allowed: false, currentCount: 1001}
	rl := NewRateLimiter(store, zap.NewNop(), nil)

	router := gin.New()
	router.Use(setTenantContext("tenant-1", false))
	router.Use(rl.Middleware())
	router.GET("/test", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

func TestRateLimiter_ExemptsPlatformAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &mockRateLimitStore{allowed: false, currentCount: 9999}
	rl := NewRateLimiter(store, zap.NewNop(), nil)

	router := gin.New()
	router.Use(setTenantContext("admin-tenant", true))
	router.Use(rl.Middleware())
	router.GET("/test", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, store.callCount)
}

func TestRateLimiter_SkipsWithoutTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &mockRateLimitStore{allowed: false, currentCount: 9999}
	rl := NewRateLimiter(store, zap.NewNop(), nil)

	router := gin.New()
	// No tenant context middleware.
	router.Use(rl.Middleware())
	router.GET("/test", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, store.callCount)
}

func TestRateLimiter_FailsOpenOnError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &mockRateLimitStore{err: errors.New("redis connection refused")}
	rl := NewRateLimiter(store, zap.NewNop(), nil)

	router := gin.New()
	router.Use(setTenantContext("tenant-1", false))
	router.Use(rl.Middleware())
	router.GET("/test", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimiter_UsesCustomLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &mockRateLimitStore{allowed: true, currentCount: 3}
	getLimit := func(_ context.Context, _ string) int { return 50 }
	rl := NewRateLimiter(store, zap.NewNop(), getLimit)

	router := gin.New()
	router.Use(setTenantContext("tenant-1", false))
	router.Use(rl.Middleware())
	router.GET("/test", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "50", w.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "47", w.Header().Get("X-RateLimit-Remaining"))
}

func TestRateLimiter_FallsBackOnZeroLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &mockRateLimitStore{allowed: true, currentCount: 10}
	getLimit := func(_ context.Context, _ string) int { return 0 }
	rl := NewRateLimiter(store, zap.NewNop(), getLimit)

	router := gin.New()
	router.Use(setTenantContext("tenant-1", false))
	router.Use(rl.Middleware())
	router.GET("/test", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "1000", w.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "990", w.Header().Get("X-RateLimit-Remaining"))
}

func TestRateLimiter_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		tenantID           string
		isPlatformAdmin    bool
		storeAllowed       bool
		storeCount         int
		storeErr           error
		customLimit        int
		wantStatus         int
		wantStoreCallCount int
		wantLimitHeader    string
		wantRemainingRange string
	}{
		{
			name:               "regular tenant under limit",
			tenantID:           "tenant-a",
			storeAllowed:       true,
			storeCount:         100,
			wantStatus:         http.StatusOK,
			wantStoreCallCount: 1,
			wantLimitHeader:    "1000",
		},
		{
			name:               "regular tenant over limit",
			tenantID:           "tenant-b",
			storeAllowed:       false,
			storeCount:         1500,
			wantStatus:         http.StatusTooManyRequests,
			wantStoreCallCount: 1,
			wantLimitHeader:    "1000",
		},
		{
			name:               "platform admin bypasses limit",
			tenantID:           "tenant-c",
			isPlatformAdmin:    true,
			storeAllowed:       false,
			storeCount:         9999,
			wantStatus:         http.StatusOK,
			wantStoreCallCount: 0,
		},
		{
			name:               "no tenant context passes through",
			tenantID:           "",
			storeAllowed:       false,
			storeCount:         9999,
			wantStatus:         http.StatusOK,
			wantStoreCallCount: 0,
		},
		{
			name:               "store error fails open",
			tenantID:           "tenant-d",
			storeErr:           errors.New("connection timeout"),
			wantStatus:         http.StatusOK,
			wantStoreCallCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockRateLimitStore{
				allowed:      tt.storeAllowed,
				currentCount: tt.storeCount,
				err:          tt.storeErr,
			}
			rl := NewRateLimiter(store, zap.NewNop(), nil)

			router := gin.New()
			if tt.tenantID != "" {
				router.Use(setTenantContext(tt.tenantID, tt.isPlatformAdmin))
			}
			router.Use(rl.Middleware())
			router.GET("/test", okHandler)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantStoreCallCount, store.callCount)

			if tt.wantLimitHeader != "" {
				assert.Equal(t, tt.wantLimitHeader, w.Header().Get("X-RateLimit-Limit"))
			}
		})
	}
}

func TestNewRateLimiter_NilGetLimit(t *testing.T) {
	store := &mockRateLimitStore{allowed: true, currentCount: 0}
	rl := NewRateLimiter(store, zap.NewNop(), nil)

	require.NotNil(t, rl)
	require.NotNil(t, rl.getLimit)

	// The default function should return defaultRateLimit.
	limit := rl.getLimit(context.Background(), "any-tenant")
	assert.Equal(t, defaultRateLimit, limit)
}

func TestRedisRateLimitStore_AtomicIncrementAndCheck(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisRateLimitStore(client)
	ctx := context.Background()

	t.Run("first request sets TTL atomically", func(t *testing.T) {
		allowed, count, err := store.IncrementAndCheck(ctx, "ratelimit:tenant-1", 10, time.Minute)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 1, count)

		// Verify TTL was set (miniredis tracks TTL).
		mr.FastForward(30 * time.Second)
		assert.True(t, mr.Exists("ratelimit:tenant-1"))
		mr.FastForward(31 * time.Second)
		assert.False(t, mr.Exists("ratelimit:tenant-1"))
	})

	t.Run("increments correctly and enforces limit", func(t *testing.T) {
		key := "ratelimit:tenant-2"
		limit := 3

		for i := 1; i <= limit; i++ {
			allowed, count, err := store.IncrementAndCheck(ctx, key, limit, time.Minute)
			require.NoError(t, err)
			assert.True(t, allowed, "request %d should be allowed", i)
			assert.Equal(t, i, count)
		}

		// Next request should be denied.
		allowed, count, err := store.IncrementAndCheck(ctx, key, limit, time.Minute)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 4, count)
	})

	t.Run("orphaned key without TTL cannot occur", func(t *testing.T) {
		// Verify that PEXPIRE is always set atomically with INCR.
		// If INCR creates the key (count == 1), the Lua script sets PEXPIRE in the
		// same atomic operation. Simulate by checking TTL is always present after
		// the first increment.
		key := "ratelimit:orphan-check"

		allowed, count, err := store.IncrementAndCheck(ctx, key, 10, 5*time.Second)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 1, count)

		ttl := mr.TTL(key)
		assert.True(t, ttl > 0, "key must have a TTL after first increment, got %v", ttl)

		// Second increment should NOT reset the TTL (only count==1 sets it).
		mr.FastForward(2 * time.Second)
		_, _, err = store.IncrementAndCheck(ctx, key, 10, 5*time.Second)
		require.NoError(t, err)

		ttlAfter := mr.TTL(key)
		assert.True(t, ttlAfter > 0, "key must still have a TTL after second increment")
		assert.True(t, ttlAfter < 4*time.Second, "TTL should not have been reset by second increment")
	})

	t.Run("key expires and resets counter", func(t *testing.T) {
		key := "ratelimit:tenant-3"
		limit := 2

		// Exhaust the limit.
		for i := 0; i < limit+1; i++ {
			_, _, err := store.IncrementAndCheck(ctx, key, limit, time.Minute)
			require.NoError(t, err)
		}

		// Fast-forward past the window.
		mr.FastForward(61 * time.Second)

		// Counter should have reset.
		allowed, count, err := store.IncrementAndCheck(ctx, key, limit, time.Minute)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 1, count)
	})
}
