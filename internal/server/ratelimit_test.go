package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
