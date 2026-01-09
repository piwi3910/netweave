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

func TestResourceCREATE(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	t.Run("POST /resources - create resource", func(t *testing.T) {
		resource := adapter.Resource{
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Description:    "Test compute resource",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)

		var created adapter.Resource
		err = json.Unmarshal(resp.Body.Bytes(), &created)
		require.NoError(t, err)
		assert.Equal(t, resource.ResourceTypeID, created.ResourceTypeID)
		assert.Equal(t, resource.ResourcePoolID, created.ResourcePoolID)
		assert.Equal(t, resource.Description, created.Description)
		assert.NotEmpty(t, created.ResourceID)
	})

	t.Run("POST /resources - validation error (empty resourceTypeId)", func(t *testing.T) {
		resource := adapter.Resource{
			ResourcePoolID: "pool-1",
			Description:    "Test resource",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Resource type ID is required")
	})

	t.Run("POST /resources - invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader([]byte("invalid json")),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Invalid request body")
	})

	t.Run("POST /resources - with custom resource ID", func(t *testing.T) {
		resource := adapter.Resource{
			ResourceID:     "custom-res-123",
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)

		var created adapter.Resource
		err = json.Unmarshal(resp.Body.Bytes(), &created)
		require.NoError(t, err)
		assert.Equal(t, "custom-res-123", created.ResourceID)
	})

	t.Run("POST /resources - with extensions", func(t *testing.T) {
		resource := adapter.Resource{
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Description:    "Test resource with extensions",
			Extensions: map[string]interface{}{
				"cpu":    "16 cores",
				"memory": "64GB",
				"disk":   "1TB SSD",
			},
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)

		var created adapter.Resource
		err = json.Unmarshal(resp.Body.Bytes(), &created)
		require.NoError(t, err)
		assert.NotNil(t, created.Extensions)
		assert.Equal(t, "16 cores", created.Extensions["cpu"])
	})
}
