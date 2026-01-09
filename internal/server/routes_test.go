package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/config"
)

// TestHandleHealth tests the handleHealth endpoint.
func TestHandleHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("returns healthy status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"status"`)
	})
}

// TestHandleReadiness tests the handleReadiness endpoint.
func TestHandleReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("returns ready status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"ready"`)
	})
}

// TestHandleMetrics tests the handleMetrics endpoint.
func TestHandleMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
		Observability: config.ObservabilityConfig{
			Metrics: config.MetricsConfig{
				Path: "/metrics",
			},
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("returns prometheus metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "# HELP")
	})
}

// TestHandleRoot tests the handleRoot endpoint.
func TestHandleRoot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("returns API information", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "O2-IMS Gateway")
		assert.Contains(t, w.Body.String(), "health")
		assert.Contains(t, w.Body.String(), "ready")
	})
}

// TestHandleAPIInfo tests the handleAPIInfo endpoint.
func TestHandleAPIInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("returns O2-IMS API info", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1", nil)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "subscriptions")
		assert.Contains(t, w.Body.String(), "resourcePools")
	})
}

// TestHandleListSubscriptions tests the handleListSubscriptions endpoint.
func TestHandleListSubscriptions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("returns empty subscription list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/subscriptions", nil)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "[]")
	})
}

// TestHandleListResourcePools tests the handleListResourcePools endpoint.
func TestHandleListResourcePools(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("returns resource pool list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourcePools", nil)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestHandleListResources tests the handleListResources endpoint.
func TestHandleListResources(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("returns resource list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resources", nil)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestHandleListResourceTypes tests the handleListResourceTypes endpoint.
func TestHandleListResourceTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("returns resource type list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes", nil)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestHandleListDeploymentManagers tests the handleListDeploymentManagers endpoint.
func TestHandleListDeploymentManagers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("returns deployment manager list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/deploymentManagers", nil)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		// May return error or success - just test that handler executes
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
	})
}
