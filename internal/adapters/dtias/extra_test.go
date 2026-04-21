package dtias

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func setupDTIASTestServer(t *testing.T, handlers map[string]http.HandlerFunc) (*httptest.Server, *Adapter) {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, handler := range handlers {
		mux.HandleFunc(pattern, handler)
	}
	server := httptest.NewServer(mux)

	adp := NewTestAdapter(server.URL, server.Client(), zaptest.NewLogger(t))
	adp.Config = &Config{
		Endpoint:            server.URL,
		APIKey:              "test-key",
		OCloudID:            "ocloud-test",
		DeploymentManagerID: "dm-test",
		Datacenter:          "dc-test-1",
	}
	adp.OCloudID = "ocloud-test"
	adp.DeploymentManagerID = "dm-test"
	adp.Subs = adapter.NewInMemorySubscriptionStore()

	return server, adp
}

func TestListResourcePools_Success(t *testing.T) {
	now := time.Now()
	handlers := map[string]http.HandlerFunc{
		"/v2/inventory/resourcepools": func(w http.ResponseWriter, r *http.Request) {
			resp := ResourcePoolsInventoryResponse{
				Rps: []ServerPool{
					{
						ID:               "pool-1",
						Name:             "Pool One",
						Description:      "First pool",
						Datacenter:       "dc-test-1",
						Type:             "compute",
						State:            "active",
						ServerCount:      10,
						AvailableServers: 5,
						Location: Location{
							City:      "Dallas",
							Latitude:  32.7767,
							Longitude: -96.7970,
						},
						CreatedAt: now,
						UpdatedAt: now,
					},
					{
						ID:         "pool-2",
						Name:       "Pool Two",
						Datacenter: "dc-test-1",
						Type:       "storage",
						State:      "active",
						CreatedAt:  now,
						UpdatedAt:  now,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	pools, err := adp.ListResourcePools(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, pools, 2)
	assert.Equal(t, "pool-1", pools[0].ResourcePoolID)
	assert.Equal(t, "Pool One", pools[0].Name)
}

func TestListResourcePools_WithLocationFilter(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/inventory/resourcepools": func(w http.ResponseWriter, r *http.Request) {
			resp := ResourcePoolsInventoryResponse{
				Rps: []ServerPool{
					{
						ID:         "pool-dallas",
						Name:       "Dallas Pool",
						Datacenter: "dc-dallas",
						Location:   Location{City: "Dallas"},
						CreatedAt:  time.Now(),
						UpdatedAt:  time.Now(),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	filter := &adapter.Filter{Location: "Dallas, dc-dallas"}
	pools, err := adp.ListResourcePools(context.Background(), filter)
	require.NoError(t, err)
	assert.Len(t, pools, 1)
}

func TestListResourcePools_APIError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/inventory/resourcepools": func(w http.ResponseWriter, r *http.Request) {
			resp := ResourcePoolsInventoryResponse{
				Error: &APIError{
					Code:    "ACCESS_DENIED",
					Message: "unauthorized",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	pools, err := adp.ListResourcePools(context.Background(), nil)
	require.Error(t, err)
	assert.Nil(t, pools)
}

func TestListResourcePools_HTTPError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/inventory/resourcepools": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	pools, err := adp.ListResourcePools(context.Background(), nil)
	require.Error(t, err)
	assert.Nil(t, pools)
}

func TestBuildServerPoolsPath(t *testing.T) {
	tests := []struct {
		name     string
		filter   *adapter.Filter
		expected string
	}{
		{
			name:     "nil filter",
			filter:   nil,
			expected: "/v2/inventory/resourcepools",
		},
		{
			name:     "empty filter",
			filter:   &adapter.Filter{},
			expected: "/v2/inventory/resourcepools",
		},
		{
			name: "filter with location",
			filter: &adapter.Filter{
				Location: "dc-dallas-1",
			},
			expected: "/v2/inventory/resourcepools?siteId=dc-dallas-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := buildServerPoolsPath(tt.filter)
			assert.Equal(t, tt.expected, path)
		})
	}
}

func TestCreateResourcePool_Success(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/resourcepools": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			resp := ServerPool{
				ID:          "new-pool-1",
				Name:        "New Pool",
				Description: "A new pool",
				Datacenter:  "dc-test-1",
				Type:        "compute",
				State:       "provisioning",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	pool := &adapter.ResourcePool{
		Name:        "New Pool",
		Description: "A new pool",
		Location:    "dc-test-1",
	}

	result, err := adp.CreateResourcePool(context.Background(), pool)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "new-pool-1", result.ResourcePoolID)
	assert.Equal(t, "New Pool", result.Name)
}

func TestCreateResourcePool_WithExtensions(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/resourcepools": func(w http.ResponseWriter, r *http.Request) {
			resp := ServerPool{
				ID:        "ext-pool",
				Name:      "Extended Pool",
				Type:      "storage",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	pool := &adapter.ResourcePool{
		Name:        "Extended Pool",
		Description: "Pool with extensions",
		Extensions: map[string]interface{}{
			"dtias.poolType": "storage",
			"dtias.metadata": map[string]string{
				"env": "staging",
			},
		},
	}

	result, err := adp.CreateResourcePool(context.Background(), pool)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestCreateResourcePool_Error(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/resourcepools": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			resp := APIError{Code: "INVALID", Message: "bad request"}
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	pool := &adapter.ResourcePool{
		Name: "Bad Pool",
	}

	result, err := adp.CreateResourcePool(context.Background(), pool)
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestUpdateResourcePool_Success(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/resourcepools/pool-update": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			resp := ServerPool{
				ID:          "pool-update",
				Name:        "Updated Pool",
				Description: "Updated description",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	pool := &adapter.ResourcePool{
		Name:        "Updated Pool",
		Description: "Updated description",
	}

	result, err := adp.UpdateResourcePool(context.Background(), "pool-update", pool)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "pool-update", result.ResourcePoolID)
	assert.Equal(t, "Updated Pool", result.Name)
}

func TestUpdateResourcePool_WithExtensions(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/resourcepools/pool-ext": func(w http.ResponseWriter, r *http.Request) {
			resp := ServerPool{
				ID:        "pool-ext",
				Name:      "Ext Pool",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	pool := &adapter.ResourcePool{
		Name: "Ext Pool",
		Extensions: map[string]interface{}{
			"dtias.metadata": map[string]string{"updated": "true"},
		},
	}

	result, err := adp.UpdateResourcePool(context.Background(), "pool-ext", pool)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestUpdateResourcePool_Error(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/resourcepools/pool-fail": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			resp := APIError{Code: "NOT_FOUND", Message: "pool not found"}
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	pool := &adapter.ResourcePool{Name: "Fail"}
	result, err := adp.UpdateResourcePool(context.Background(), "pool-fail", pool)
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestDeleteResourcePool_Success(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/resourcepools/pool-delete": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	err := adp.DeleteResourcePool(context.Background(), "pool-delete")
	require.NoError(t, err)
}

func TestDeleteResourcePool_HTTPError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/resourcepools/pool-fail": func(w http.ResponseWriter, r *http.Request) {
			// Return 500 so doRequest retries and eventually fails
			w.WriteHeader(http.StatusInternalServerError)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	err := adp.DeleteResourcePool(context.Background(), "pool-fail")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete server pool")
}

func TestListDeploymentManagers_Success(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/inventory/sites/dc-test-1": func(w http.ResponseWriter, r *http.Request) {
			resp := DatacenterInfo{
				ID:        "dc-test-1",
				Name:      "Test DC",
				City:      "Dallas",
				Country:   "US",
				Latitude:  32.7767,
				Longitude: -96.797,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	dms, err := adp.ListDeploymentManagers(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, dms, 1)
	assert.Equal(t, "dm-test", dms[0].DeploymentManagerID)
}

func TestGetDeploymentManager_WithDatacenterInfo(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/inventory/sites/dc-test-1": func(w http.ResponseWriter, r *http.Request) {
			resp := DatacenterInfo{
				ID:        "dc-test-1",
				Name:      "Test DC",
				City:      "Dallas",
				Country:   "US",
				Latitude:  32.7767,
				Longitude: -96.797,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	dm, err := adp.GetDeploymentManager(context.Background(), "dm-test")
	require.NoError(t, err)
	require.NotNil(t, dm)
	assert.Equal(t, "dm-test", dm.DeploymentManagerID)
	assert.Contains(t, dm.Extensions, "dtias.location.city")
	assert.Equal(t, "Dallas", dm.Extensions["dtias.location.city"])
	assert.Contains(t, dm.Extensions, "dtias.location.country")
	assert.Contains(t, dm.Extensions, "dtias.location.latitude")
	assert.Contains(t, dm.Extensions, "dtias.location.longitude")
	assert.Len(t, dm.SupportedLocations, 1)
}

func TestGetDeploymentManager_DatacenterError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/inventory/sites/dc-test-1": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	// Should still succeed, just with defaults
	dm, err := adp.GetDeploymentManager(context.Background(), "dm-test")
	require.NoError(t, err)
	require.NotNil(t, dm)
	assert.Equal(t, "dm-test", dm.DeploymentManagerID)
}

func TestGetDeploymentManager_WrongID(t *testing.T) {
	server, adp := setupDTIASTestServer(t, map[string]http.HandlerFunc{})
	defer server.Close()

	_, err := adp.GetDeploymentManager(context.Background(), "wrong-id")
	require.Error(t, err)
}

func TestHealthCheck_Success(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/version": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"version": "1.0"})
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	err := adp.Health(context.Background())
	require.NoError(t, err)
}

func TestHealthCheck_Failure(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/version": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	err := adp.Health(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DTIAS API unreachable")
}

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      APIError
		expected string
	}{
		{
			name: "with details",
			err: APIError{
				Code:    "INVALID_REQUEST",
				Message: "bad request",
				Details: "field X is missing",
			},
			expected: "DTIAS API error [INVALID_REQUEST]: bad request - field X is missing",
		},
		{
			name: "without details",
			err: APIError{
				Code:    "NOT_FOUND",
				Message: "resource not found",
			},
			expected: "DTIAS API error [NOT_FOUND]: resource not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestTransformAndFilterPools(t *testing.T) {
	now := time.Now()
	adp := &Adapter{
		logger:   zaptest.NewLogger(t),
		OCloudID: "ocloud-test",
	}

	serverPools := []ServerPool{
		{
			ID:         "pool-1",
			Name:       "Pool One",
			Datacenter: "dc-1",
			Location:   Location{City: "Dallas"},
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ID:         "pool-2",
			Name:       "Pool Two",
			Datacenter: "dc-2",
			Location:   Location{City: "Austin"},
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}

	t.Run("no filter returns all", func(t *testing.T) {
		result := adp.transformAndFilterPools(serverPools, nil)
		assert.Len(t, result, 2)
	})

	t.Run("filter by location", func(t *testing.T) {
		filter := &adapter.Filter{Location: "Dallas, dc-1"}
		result := adp.transformAndFilterPools(serverPools, filter)
		assert.Len(t, result, 1)
		assert.Equal(t, "pool-1", result[0].ResourcePoolID)
	})
}

func TestGetResourcePool_APIError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/inventory/resourcepools/pool-error": func(w http.ResponseWriter, r *http.Request) {
			resp := ResourcePoolInventoryResponse{
				Error: &APIError{
					Code:    "NOT_FOUND",
					Message: "pool not found",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	_, err := adp.GetResourcePool(context.Background(), "pool-error")
	require.Error(t, err)
}

func TestGetResourcePool_Success(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/inventory/resourcepools/pool-ok": func(w http.ResponseWriter, r *http.Request) {
			resp := ResourcePoolInventoryResponse{
				Rp: ServerPool{
					ID:         "pool-ok",
					Name:       "OK Pool",
					Datacenter: "dc-1",
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	server, adp := setupDTIASTestServer(t, handlers)
	defer server.Close()

	pool, err := adp.GetResourcePool(context.Background(), "pool-ok")
	require.NoError(t, err)
	require.NotNil(t, pool)
	assert.Equal(t, "pool-ok", pool.ResourcePoolID)
	assert.Equal(t, "OK Pool", pool.Name)
}

func TestClientHealthCheck_Success(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/v2/version": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for pattern, handler := range handlers {
			if r.URL.Path == pattern {
				handler(w, r)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
		logger:     zaptest.NewLogger(t),
	}

	err := client.HealthCheck(context.Background())
	require.NoError(t, err)
}

func TestClientHealthCheck_BadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
		logger:     zaptest.NewLogger(t),
	}

	err := client.HealthCheck(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server error")
}
