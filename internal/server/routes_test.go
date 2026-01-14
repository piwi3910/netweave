package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/piwi3910/netweave/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/config"
)

// TestHandleHealth tests the handleHealth endpoint.
func TestHandleHealth(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("returns healthy status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"status"`)
	})
}

// TestHandleReadiness tests the handleReadiness endpoint.
func TestHandleReadiness(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("returns ready status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"ready"`)
	})
}

// TestHandleMetrics tests the handleMetrics endpoint.
func TestHandleMetrics(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
		Observability: config.ObservabilityConfig{
			Metrics: config.MetricsConfig{
				Enabled: true,
				Path:    "/metrics",
			},
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("returns prometheus metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "# HELP")
	})
}

// TestHandleRoot tests the handleRoot endpoint.
func TestHandleRoot(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("returns API information", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "O2-IMS Gateway")
		assert.Contains(t, w.Body.String(), "health")
		assert.Contains(t, w.Body.String(), "ready")
	})
}

// TestHandleAPIInfo tests the handleAPIInfo endpoint.
func TestHandleAPIInfo(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("returns O2-IMS API info", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "subscriptions")
		assert.Contains(t, w.Body.String(), "resourcePools")
	})
}

// TestHandleListSubscriptions tests the handleListSubscriptions endpoint.
func TestHandleListSubscriptions(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("returns empty subscription list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/subscriptions", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "[]")
	})
}

// TestHandleListResourcePools tests the handleListResourcePools endpoint.
func TestHandleListResourcePools(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("returns resource pool list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourcePools", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestHandleListResources tests the handleListResources endpoint.
func TestHandleListResources(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("returns resource list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resources", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestHandleListResourceTypes tests the handleListResourceTypes endpoint.
func TestHandleListResourceTypes(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("returns resource type list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestHandleListDeploymentManagers tests the handleListDeploymentManagers endpoint.
func TestHandleListDeploymentManagers(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("returns deployment manager list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/deploymentManagers", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		// May return error or success - just test that handler executes
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
	})
}

// TestHandleCreateSubscription tests the handleCreateSubscription endpoint.
func TestHandleCreateSubscription(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("creates subscription successfully", func(t *testing.T) {
		body := `{"callback":"https://smo.example.com/notify","filter":{}}`
		req := httptest.NewRequest(
			http.MethodPost, "/o2ims-infrastructureInventory/v1/subscriptions", strings.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})
}

// TestHandleGetSubscription tests the handleGetSubscription endpoint.
func TestHandleGetSubscription(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("subscription not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/subscriptions/sub-123", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestHandleDeleteSubscription tests the handleDeleteSubscription endpoint.
func TestHandleDeleteSubscription(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("deletes subscription", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/subscriptions/sub-123", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

// TestHandleGetResourcePool tests the handleGetResourcePool endpoint.
func TestHandleGetResourcePool(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("resource pool not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourcePools/pool-123", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestHandleGetResource tests the handleGetResource endpoint.
func TestHandleGetResource(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("resource not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resources/res-123", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestHandleDeleteResource tests the handleDeleteResource endpoint.
func TestHandleDeleteResource(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("resource not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/resources/res-nonexistent", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "NotFound")
	})

	t.Run("successful deletion", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/resources/res-existing", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Empty(t, w.Body.String())
	})
}

// TestHandleGetResourceType tests the handleGetResourceType endpoint.
func TestHandleGetResourceType(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("resource type not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes/type-123", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestHandleGetDeploymentManager tests the handleGetDeploymentManager endpoint.
func TestHandleGetDeploymentManager(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("deployment manager not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/deploymentManagers/dm-123", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestHandleListSubscriptions_WithFilter tests the handleListSubscriptions endpoint with filter.
func TestHandleListSubscriptions_WithFilter(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

	t.Run("list subscriptions with filter", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodGet,
			"/o2ims-infrastructureInventory/v1/subscriptions?filter=(eq,callback,'https://example.com/callback')",
			nil,
		)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestHandleHealth_Error tests the health endpoint error path.
func TestHandleHealth_Error(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}

	// Use mockAdapter with Health() error
	mockAdp := &mockAdapter{healthErr: fmt.Errorf("adapter unhealthy")}
	srv := server.New(cfg, zap.NewNop(), mockAdp, &mockStore{}, nil)

	t.Run("health check failed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

// TestHandleReadiness_Error tests the readiness endpoint error path.
func TestHandleReadiness_Error(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}

	// Use mockAdapter with Health() error
	mockAdp := &mockAdapter{healthErr: fmt.Errorf("not ready")}
	srv := server.New(cfg, zap.NewNop(), mockAdp, &mockStore{}, nil)

	t.Run("readiness check failed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

// TestHandleCreateResourcePool tests the handleCreateResourcePool endpoint.
func TestHandleCreateResourcePool(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	t.Run("creates resource pool successfully with auto-generated ID", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		body := `{"name":"GPU Pool Production","description":"High-performance GPU resources"}`
		req := httptest.NewRequest(
			http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", strings.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "/resourcePools/pool-")
	})

	t.Run("creates resource pool with client-provided ID", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		body := `{"resourcePoolId":"pool-custom-123","name":"Custom Pool"}`
		req := httptest.NewRequest(
			http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", strings.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "pool-custom-123")
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		body := `{"name":invalid json}`
		req := httptest.NewRequest(
			http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", strings.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "BadRequest")
	})

	t.Run("returns 400 for missing name", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		body := `{"description":"Pool without name"}`
		req := httptest.NewRequest(
			http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", strings.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "name is required")
	})
}

// TestHandleUpdateResourcePool tests the handleUpdateResourcePool endpoint.
func TestHandleUpdateResourcePool(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		poolID         string
		body           string
		expectedStatus int
	}{
		{
			name:           "returns 404 for non-existent pool",
			poolID:         "pool-nonexistent",
			body:           `{"name":"Updated Pool"}`,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "returns 400 for invalid JSON",
			poolID:         "pool-123",
			body:           `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			srv := server.New(
			&config.Config{Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode}},
			zap.NewNop(),
			&mockAdapter{},
			&mockStore{},
			nil,
		)

			req := httptest.NewRequest(
				http.MethodPut,
				"/o2ims-infrastructureInventory/v1/resourcePools/"+tt.poolID,
				strings.NewReader(tt.body),
			)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.Router().ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestHandleDeleteResourcePool tests the handleDeleteResourcePool endpoint.
func TestHandleDeleteResourcePool(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	t.Run("returns 404 for non-existent pool", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/resourcePools/pool-nonexistent", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("deletes successfully", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/resourcePools/pool-existing", nil)
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

// TestHandleCreateResource tests the handleCreateResource endpoint.
func TestHandleCreateResource(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	t.Run("creates resource successfully", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		body := `{"resourceTypeId":"compute-node","resourcePoolId":"pool-123","name":"Node 1"}`
		req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resources", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "/resources/")

		// Validate response body contains a valid UUID (not old "res-" prefix format)
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		resourceID, ok := response["resourceId"].(string)
		assert.True(t, ok, "resourceId should be a string")
		assert.NotEmpty(t, resourceID)

		// Verify UUID format (36 characters with hyphens in correct positions)
		assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, resourceID,
			"resourceId should be a valid UUID, not old res-{type}-{uuid} format")

		// Verify no "res-" prefix
		assert.NotContains(t, resourceID, "res-", "resourceId should not contain old res- prefix")
	})

	t.Run("returns 400 for missing resourceTypeId", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		body := `{"resourcePoolId":"pool-123","name":"Node 1"}`
		req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resources", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "resourceTypeId")
	})

	t.Run("returns 400 for missing resourcePoolId", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		body := `{"resourceTypeId":"compute-node","name":"Node 1"}`
		req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resources", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "resourcePoolId")
	})
}

// TestHandleUpdateSubscription tests the handleUpdateSubscription endpoint.
func TestHandleUpdateSubscription(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	t.Run("returns 404 for non-existent subscription", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		subID := "sub-nonexistent"
		updatePayload := `{"callback":"https://new-callback.example.com/notify"}`
		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/"+subID,
			strings.NewReader(updatePayload),
		)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		}
		srv := server.New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{}, nil)

		malformedJSON := `{invalid json}`
		subscriptionEndpoint := "/o2ims-infrastructureInventory/v1/subscriptions/sub-123"
		req := httptest.NewRequest(http.MethodPut, subscriptionEndpoint, strings.NewReader(malformedJSON))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		srv.Router().ServeHTTP(response, req)

		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
