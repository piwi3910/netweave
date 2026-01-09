package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
)

func TestResourcePoolCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("POST /resourcePools - create resource pool", func(t *testing.T) {
		pool := adapter.ResourcePool{
			Name:        "test-pool",
			Description: "Test resource pool",
			Location:    "us-west-1",
		}

		body, err := json.Marshal(pool)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)

		var created adapter.ResourcePool
		err = json.Unmarshal(resp.Body.Bytes(), &created)
		require.NoError(t, err)
		assert.Equal(t, pool.Name, created.Name)
		assert.Equal(t, pool.Description, created.Description)
		assert.Equal(t, pool.Location, created.Location)
	})

	t.Run("POST /resourcePools - validation error (empty name)", func(t *testing.T) {
		pool := adapter.ResourcePool{
			Description: "Test pool",
		}

		body, err := json.Marshal(pool)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Resource pool name is required")
	})

	t.Run("POST /resourcePools - invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resourcePools",
			bytes.NewReader([]byte("invalid json")),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Invalid request body")
	})

	t.Run("PUT /resourcePools/:id - update resource pool", func(t *testing.T) {
		pool := adapter.ResourcePool{
			Description: "Updated description",
			Location:    "us-east-1",
		}

		body, err := json.Marshal(pool)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/resourcePools/test-pool",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var updated adapter.ResourcePool
		err = json.Unmarshal(resp.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.Equal(t, pool.Description, updated.Description)
		assert.Equal(t, pool.Location, updated.Location)
	})

	t.Run("PUT /resourcePools/:id - invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/resourcePools/test-pool",
			bytes.NewReader([]byte("invalid json")),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Invalid request body")
	})

	t.Run("DELETE /resourcePools/:id - delete resource pool", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/resourcePools/test-pool", nil)
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNoContent, resp.Code)
		assert.Empty(t, resp.Body.String())
	})
}
