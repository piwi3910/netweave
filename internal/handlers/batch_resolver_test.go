package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/handlers"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"github.com/piwi3910/netweave/internal/storage"
)

// TestBatchHandler_NilAdapter_NoResolver_Returns503 is the regression test
// for issue #480 (H7). Previously cmd/gateway/main.go passed a nil adapter
// into server.New which propagated into NewBatchHandler. Invoking any
// batch endpoint then NPEd on the first call. After the fix, a batch
// request made with no adapter and no resolver must return 503 with a
// structured error instead of panicking.
func TestBatchHandler_NilAdapter_NoResolver_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestMetrics()

	mr := miniredis.RunT(t)
	defer mr.Close()
	store := storage.NewRedisStore(&storage.RedisConfig{Addr: mr.Addr()})

	// Explicitly do not configure an adapter or a resolver — exactly the
	// production configuration described in issue #480.
	h := handlers.NewBatchHandler(nil, store, zap.NewNop(), nil)

	endpoints := []struct {
		name   string
		method string
		path   string
		body   []byte
		route  func(*gin.Engine)
	}{
		{
			name:   "create resource pools",
			method: http.MethodPost,
			path:   "/batch/resourcePools",
			body:   mustJSON(t, handlers.BatchResourcePoolCreate{ResourcePools: []models.ResourcePool{{ResourcePoolID: "p"}}}),
			route:  func(r *gin.Engine) { r.POST("/batch/resourcePools", h.BatchCreateResourcePools) },
		},
		{
			name:   "delete resource pools",
			method: http.MethodPost,
			path:   "/batch/resourcePools/delete",
			body:   mustJSON(t, handlers.BatchResourcePoolDelete{ResourcePoolIDs: []string{"p"}}),
			route:  func(r *gin.Engine) { r.POST("/batch/resourcePools/delete", h.BatchDeleteResourcePools) },
		},
		{
			name:   "update resource pools",
			method: http.MethodPost,
			path:   "/batch/resourcePools/update",
			body: mustJSON(t, handlers.BatchResourcePoolUpdate{Updates: []handlers.ResourcePoolUpdateItem{
				{ResourcePoolID: "p", Update: models.ResourcePool{Name: "new"}},
			}}),
			route: func(r *gin.Engine) { r.POST("/batch/resourcePools/update", h.BatchUpdateResourcePools) },
		},
		{
			name:   "create resources",
			method: http.MethodPost,
			path:   "/batch/resources",
			body:   mustJSON(t, handlers.BatchResourceCreate{Resources: []models.Resource{{ResourceID: "r"}}}),
			route:  func(r *gin.Engine) { r.POST("/batch/resources", h.BatchCreateResources) },
		},
		{
			name:   "delete resources",
			method: http.MethodPost,
			path:   "/batch/resources/delete",
			body:   mustJSON(t, handlers.BatchResourceDelete{ResourceIDs: []string{"r"}}),
			route:  func(r *gin.Engine) { r.POST("/batch/resources/delete", h.BatchDeleteResources) },
		},
		{
			name:   "update resources",
			method: http.MethodPost,
			path:   "/batch/resources/update",
			body: mustJSON(t, handlers.BatchResourceUpdate{Updates: []handlers.ResourceUpdateItem{
				{ResourceID: "r", Update: models.Resource{Description: "d"}},
			}}),
			route: func(r *gin.Engine) { r.POST("/batch/resources/update", h.BatchUpdateResources) },
		},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			router := gin.New()
			ep.route(router)

			req := httptest.NewRequest(ep.method, ep.path, bytes.NewReader(ep.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// The critical assertion: this must not panic.
			require.NotPanics(t, func() { router.ServeHTTP(w, req) })

			assert.Equal(t, http.StatusServiceUnavailable, w.Code,
				"nil adapter with no resolver must return 503 on %s %s, got body=%s",
				ep.method, ep.path, w.Body.String())

			var resp models.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "ServiceUnavailable", resp.Error)
		})
	}
}

// TestBatchHandler_AdapterResolver_PerTenantRouting verifies that when an
// AdapterResolver is configured, each batch request routes to the
// per-tenant adapter returned by the resolver. This is the core security
// property required by issue #480: in multi-tenant deployments a batch
// operation issued by tenant A must never land on tenant B's backend.
func TestBatchHandler_AdapterResolver_PerTenantRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestMetrics()

	mr := miniredis.RunT(t)
	defer mr.Close()
	store := storage.NewRedisStore(&storage.RedisConfig{Addr: mr.Addr()})

	// Two adapters standing in for two tenants' backends.
	tenantAAdapter := &mockBatchAdapter{}
	tenantBAdapter := &mockBatchAdapter{}

	tenantAdapters := map[string]adapter.Adapter{
		"tenant-a": tenantAAdapter,
		"tenant-b": tenantBAdapter,
	}

	h := handlers.NewBatchHandler(nil, store, zap.NewNop(), nil)
	h.SetAdapterResolver(func(c *gin.Context) adapter.Adapter {
		tenantID := c.GetHeader("X-Test-Tenant")
		adp, ok := tenantAdapters[tenantID]
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, models.ErrorResponse{
				Error:   "Forbidden",
				Message: "no backend for tenant",
				Code:    http.StatusForbidden,
			})
			return nil
		}
		return adp
	})

	router := gin.New()
	router.POST("/batch/resourcePools", h.BatchCreateResourcePools)

	// Tenant A fires a create — only tenant A's adapter should see the call.
	bodyA := mustJSON(t, handlers.BatchResourcePoolCreate{
		ResourcePools: []models.ResourcePool{{ResourcePoolID: "pool-a-1", Name: "A"}},
	})
	reqA := httptest.NewRequest(http.MethodPost, "/batch/resourcePools", bytes.NewReader(bodyA))
	reqA.Header.Set("Content-Type", "application/json")
	reqA.Header.Set("X-Test-Tenant", "tenant-a")
	wA := httptest.NewRecorder()
	router.ServeHTTP(wA, reqA)

	require.Equal(t, http.StatusOK, wA.Code, "tenant A response body=%s", wA.Body.String())
	assert.Equal(t, 1, tenantAAdapter.createPoolCount, "tenant A backend must have received the create call")
	assert.Equal(t, 0, tenantBAdapter.createPoolCount, "tenant B backend must be untouched by tenant A request")

	// Tenant B fires a create — only tenant B's adapter should see the call.
	bodyB := mustJSON(t, handlers.BatchResourcePoolCreate{
		ResourcePools: []models.ResourcePool{{ResourcePoolID: "pool-b-1", Name: "B"}},
	})
	reqB := httptest.NewRequest(http.MethodPost, "/batch/resourcePools", bytes.NewReader(bodyB))
	reqB.Header.Set("Content-Type", "application/json")
	reqB.Header.Set("X-Test-Tenant", "tenant-b")
	wB := httptest.NewRecorder()
	router.ServeHTTP(wB, reqB)

	require.Equal(t, http.StatusOK, wB.Code, "tenant B response body=%s", wB.Body.String())
	assert.Equal(t, 1, tenantAAdapter.createPoolCount, "tenant A backend must not be mutated by tenant B request")
	assert.Equal(t, 1, tenantBAdapter.createPoolCount, "tenant B backend must have received its own create call")

	// An unknown tenant is rejected by the resolver with 403 and no
	// backend is touched.
	bodyX := mustJSON(t, handlers.BatchResourcePoolCreate{
		ResourcePools: []models.ResourcePool{{ResourcePoolID: "pool-x", Name: "X"}},
	})
	reqX := httptest.NewRequest(http.MethodPost, "/batch/resourcePools", bytes.NewReader(bodyX))
	reqX.Header.Set("Content-Type", "application/json")
	reqX.Header.Set("X-Test-Tenant", "tenant-unknown")
	wX := httptest.NewRecorder()
	router.ServeHTTP(wX, reqX)

	assert.Equal(t, http.StatusForbidden, wX.Code)
	assert.Equal(t, 1, tenantAAdapter.createPoolCount, "resolver rejection must not touch any backend")
	assert.Equal(t, 1, tenantBAdapter.createPoolCount, "resolver rejection must not touch any backend")
}

// mustJSON marshals v into a JSON byte slice, failing the test on error.
func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// Ensure the package-level context import is retained even if test
// construction changes — some test helpers in this suite reference
// context.Context.
var _ = context.Background
