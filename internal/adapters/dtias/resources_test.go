package dtias_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/dtias"
)

// TestUpdateResource tests the UpdateResource method.
func TestUpdateResource(t *testing.T) {
	tests := []struct {
		name           string
		resourceID     string
		resource       *adapter.Resource
		mockResponse   dtias.Server
		expectedError  bool
		errorContains  string
		validateResult func(*testing.T, *adapter.Resource)
	}{
		{
			name:       "update description",
			resourceID: "server-123",
			resource: &adapter.Resource{
				ResourceID:  "server-123",
				Description: "Updated description",
			},
			mockResponse: dtias.Server{
				ID:              "server-123",
				Hostname:        "server123.example.com",
				Type:            "compute",
				ServerPoolID:    "pool-1",
				State:           "active",
				PowerState:      "on",
				HealthState:     "healthy",
				CPU:             dtias.CPUInfo{Vendor: "Intel", Model: "Xeon", TotalCores: 16},
				Memory:          dtias.MemoryInfo{TotalGB: 64, Type: "DDR4"},
				Metadata:        map[string]string{"env": "production"},
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
				LastHealthCheck: time.Now(),
			},
			expectedError: false,
			validateResult: func(t *testing.T, res *adapter.Resource) {
				t.Helper()
				assert.Equal(t, "server-123", res.ResourceID)
				assert.Contains(t, res.Description, "server123.example.com")
			},
		},
		{
			name:       "update hostname via extensions",
			resourceID: "server-456",
			resource: &adapter.Resource{
				ResourceID: "server-456",
				Extensions: map[string]interface{}{
					"dtias.hostname": "new-hostname.example.com",
				},
			},
			mockResponse: dtias.Server{
				ID:              "server-456",
				Hostname:        "new-hostname.example.com",
				Type:            "compute",
				ServerPoolID:    "pool-1",
				State:           "active",
				PowerState:      "on",
				HealthState:     "healthy",
				CPU:             dtias.CPUInfo{Vendor: "Intel", Model: "Xeon", TotalCores: 16},
				Memory:          dtias.MemoryInfo{TotalGB: 64, Type: "DDR4"},
				Metadata:        map[string]string{},
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
				LastHealthCheck: time.Now(),
			},
			expectedError: false,
			validateResult: func(t *testing.T, res *adapter.Resource) {
				t.Helper()
				assert.Equal(t, "server-456", res.ResourceID)
				hostname, ok := res.Extensions["dtias.hostname"].(string)
				require.True(t, ok)
				assert.Equal(t, "new-hostname.example.com", hostname)
			},
		},
		{
			name:       "update metadata via extensions",
			resourceID: "server-789",
			resource: &adapter.Resource{
				ResourceID: "server-789",
				Extensions: map[string]interface{}{
					"dtias.metadata": map[string]string{
						"env":     "staging",
						"owner":   "team-a",
						"project": "test-project",
					},
				},
			},
			mockResponse: dtias.Server{
				ID:           "server-789",
				Hostname:     "server789.example.com",
				Type:         "compute",
				ServerPoolID: "pool-1",
				State:        "active",
				PowerState:   "on",
				HealthState:  "healthy",
				CPU:          dtias.CPUInfo{Vendor: "Intel", Model: "Xeon", TotalCores: 16},
				Memory:       dtias.MemoryInfo{TotalGB: 64, Type: "DDR4"},
				Metadata: map[string]string{
					"env":     "staging",
					"owner":   "team-a",
					"project": "test-project",
				},
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
				LastHealthCheck: time.Now(),
			},
			expectedError: false,
			validateResult: func(t *testing.T, res *adapter.Resource) {
				t.Helper()
				assert.Equal(t, "server-789", res.ResourceID)
				metadata, ok := res.Extensions["dtias.metadata"].(map[string]string)
				require.True(t, ok)
				assert.Equal(t, "staging", metadata["env"])
				assert.Equal(t, "team-a", metadata["owner"])
				assert.Equal(t, "test-project", metadata["project"])
			},
		},
		{
			name:       "update all updatable fields",
			resourceID: "server-999",
			resource: &adapter.Resource{
				ResourceID:  "server-999",
				Description: "Fully updated server",
				Extensions: map[string]interface{}{
					"dtias.hostname": "updated-server.example.com",
					"dtias.metadata": map[string]string{
						"env":    "production",
						"team":   "platform",
						"region": "us-east-1",
					},
				},
			},
			mockResponse: dtias.Server{
				ID:           "server-999",
				Hostname:     "updated-server.example.com",
				Type:         "compute",
				ServerPoolID: "pool-1",
				State:        "active",
				PowerState:   "on",
				HealthState:  "healthy",
				CPU:          dtias.CPUInfo{Vendor: "Intel", Model: "Xeon", TotalCores: 32},
				Memory:       dtias.MemoryInfo{TotalGB: 128, Type: "DDR4"},
				Metadata: map[string]string{
					"env":    "production",
					"team":   "platform",
					"region": "us-east-1",
				},
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
				LastHealthCheck: time.Now(),
			},
			expectedError: false,
			validateResult: func(t *testing.T, res *adapter.Resource) {
				t.Helper()
				assert.Equal(t, "server-999", res.ResourceID)
				hostname, ok := res.Extensions["dtias.hostname"].(string)
				require.True(t, ok)
				assert.Equal(t, "updated-server.example.com", hostname)
				metadata, ok := res.Extensions["dtias.metadata"].(map[string]string)
				require.True(t, ok)
				assert.Equal(t, "production", metadata["env"])
				assert.Equal(t, "platform", metadata["team"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock HTTP server
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and path
				assert.Equal(t, http.MethodPut, r.Method)
				expectedPath := "/v2/inventory/servers/" + tt.resourceID + "/metadata"
				assert.Equal(t, expectedPath, r.URL.Path)

				// Verify request body contains expected fields
				var updateReq dtias.ServerUpdateRequest
				err := json.NewDecoder(r.Body).Decode(&updateReq)
				require.NoError(t, err)

				// Write mock response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				err = json.NewEncoder(w).Encode(tt.mockResponse)
				require.NoError(t, err)
			}))
			defer mockServer.Close()

			// Create adapter with mock client
			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			// Call UpdateResource
			result, err := adp.UpdateResource(context.Background(), tt.resourceID, tt.resource)

			// Verify error expectations
			if tt.expectedError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			// Verify success
			require.NoError(t, err)
			require.NotNil(t, result)

			// Run custom validation
			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

// TestUpdateResourceAPIError tests error handling in UpdateResource.
func TestUpdateResourceAPIError(t *testing.T) {
	// Create mock HTTP server that returns an error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		// Ignore write errors in test mock
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer mockServer.Close()

	// Create adapter with mock client
	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	testResource := &adapter.Resource{
		ResourceID:  "server-error",
		Description: "This will fail",
	}

	// Call UpdateResource
	result, err := adp.UpdateResource(context.Background(), "server-error", testResource)

	// Verify error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update server metadata")
	assert.Nil(t, result)
}

// TestUpdateResourceNotFound tests handling of non-existent resource.
func TestUpdateResourceNotFound(t *testing.T) {
	// Create mock HTTP server that returns 404
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "server not found"}`))
	}))
	defer mockServer.Close()

	// Create adapter with mock client
	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	testResource := &adapter.Resource{
		ResourceID:  "nonexistent-server",
		Description: "This server does not exist",
	}

	// Call UpdateResource
	result, err := adp.UpdateResource(context.Background(), "nonexistent-server", testResource)

	// Verify error
	require.Error(t, err)
	assert.Nil(t, result)
}

// TestListResources tests the ListResources method with a mock server.
func TestListResources(t *testing.T) {
	now := time.Now()
	servers := []dtias.Server{
		{
			ID:           "server-001",
			Hostname:     "web-server-1.example.com",
			ServerPoolID: "pool-compute",
			Type:         "r640",
			State:        "ready",
			PowerState:   "on",
			HealthState:  "healthy",
			CPU:          dtias.CPUInfo{Vendor: "Intel", Model: "Xeon Gold 6248R", TotalCores: 48},
			Memory:       dtias.MemoryInfo{TotalGB: 256, Type: "DDR4"},
			Metadata:     map[string]string{"env": "production"},
			CreatedAt:    now, UpdatedAt: now, LastHealthCheck: now,
		},
		{
			ID:           "server-002",
			Hostname:     "db-server-1.example.com",
			ServerPoolID: "pool-storage",
			Type:         "r740xd",
			State:        "in-use",
			PowerState:   "on",
			HealthState:  "healthy",
			CPU:          dtias.CPUInfo{Vendor: "Intel", Model: "Xeon Gold 6248R", TotalCores: 48},
			Memory:       dtias.MemoryInfo{TotalGB: 512, Type: "DDR4"},
			Metadata:     map[string]string{"env": "production", "o2ims.io/tenant-id": "tenant-a"},
			CreatedAt:    now, UpdatedAt: now, LastHealthCheck: now,
		},
	}

	tests := []struct {
		name          string
		filter        *adapter.Filter
		expectedCount int
	}{
		{
			name:          "list all resources without filter",
			filter:        nil,
			expectedCount: 2,
		},
		{
			name: "filter by resource pool ID",
			filter: &adapter.Filter{
				ResourcePoolID: "pool-compute",
			},
			expectedCount: 1,
		},
		{
			name: "filter by resource type ID",
			filter: &adapter.Filter{
				ResourceTypeID: "dtias-server-type-r740xd",
			},
			expectedCount: 1,
		},
		{
			name: "filter by tenant ID",
			filter: &adapter.Filter{
				TenantID: "tenant-a",
			},
			expectedCount: 1,
		},
		{
			name: "filter by non-matching tenant ID",
			filter: &adapter.Filter{
				TenantID: "tenant-nonexistent",
			},
			expectedCount: 0,
		},
		{
			name: "filter by labels",
			filter: &adapter.Filter{
				Labels: map[string]string{"env": "production"},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Contains(t, r.URL.Path, "/v2/inventory/servers")

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(dtias.ServersInventoryResponse{
					Full: servers,
				})
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			result, err := adp.ListResources(context.Background(), tt.filter)
			require.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)

			for _, res := range result {
				assert.NotEmpty(t, res.ResourceID)
				assert.NotEmpty(t, res.ResourceTypeID)
				assert.NotEmpty(t, res.Description)
				assert.NotNil(t, res.Extensions)
				assert.Contains(t, res.GlobalAssetID, "urn:dtias:server:")
			}
		})
	}
}

// TestListResources_APIError tests ListResources error handling.
func TestListResources_APIError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	result, err := adp.ListResources(context.Background(), nil)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to")
}

// TestListResources_APIResponseError tests ListResources with API error in response body.
func TestListResources_APIResponseError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(dtias.ServersInventoryResponse{
			Error: &dtias.APIError{
				Message: "access denied",
			},
		})
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	result, err := adp.ListResources(context.Background(), nil)
	require.Error(t, err)
	assert.Nil(t, result)
}

// TestGetResource tests the GetResource method with a mock server.
func TestGetResource(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		resourceID  string
		servers     []dtias.Server
		expectErr   bool
		errContains string
	}{
		{
			name:       "get existing resource",
			resourceID: "server-001",
			servers: []dtias.Server{
				{
					ID:           "server-001",
					Hostname:     "web-server-1.example.com",
					ServerPoolID: "pool-compute",
					Type:         "r640",
					State:        "ready",
					PowerState:   "on",
					HealthState:  "healthy",
					CPU:          dtias.CPUInfo{Vendor: "Intel", Model: "Xeon", TotalCores: 16},
					Memory:       dtias.MemoryInfo{TotalGB: 64, Type: "DDR4"},
					Metadata:     map[string]string{},
					CreatedAt:    now, UpdatedAt: now, LastHealthCheck: now,
				},
			},
			expectErr: false,
		},
		{
			name:        "resource not found (empty response)",
			resourceID:  "nonexistent",
			servers:     []dtias.Server{},
			expectErr:   true,
			errContains: "server not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(dtias.ServersInventoryResponse{
					Full: tt.servers,
				})
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			result, err := adp.GetResource(context.Background(), tt.resourceID)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, result)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.resourceID, result.ResourceID)
			assert.NotEmpty(t, result.Description)
			assert.NotNil(t, result.Extensions)
		})
	}
}

// TestCreateResource tests the CreateResource method with a mock server.
func TestCreateResource(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		resource     *adapter.Resource
		mockResponse dtias.Server
		expectErr    bool
	}{
		{
			name: "create basic resource",
			resource: &adapter.Resource{
				ResourcePoolID: "pool-compute",
				ResourceTypeID: "r640",
			},
			mockResponse: dtias.Server{
				ID:           "new-server-001",
				Hostname:     "new-server.example.com",
				ServerPoolID: "pool-compute",
				Type:         "r640",
				State:        "provisioning",
				PowerState:   "off",
				HealthState:  "unknown",
				CPU:          dtias.CPUInfo{Vendor: "Intel", Model: "Xeon", TotalCores: 16},
				Memory:       dtias.MemoryInfo{TotalGB: 64, Type: "DDR4"},
				Metadata:     map[string]string{},
				CreatedAt:    now, UpdatedAt: now, LastHealthCheck: now,
			},
			expectErr: false,
		},
		{
			name: "create resource with tenant ID and extensions",
			resource: &adapter.Resource{
				ResourcePoolID: "pool-compute",
				ResourceTypeID: "r640",
				TenantID:       "tenant-a",
				Extensions: map[string]interface{}{
					"dtias.hostname":        "custom-host.example.com",
					"dtias.operatingSystem": "ubuntu-22.04",
					"dtias.metadata": map[string]string{
						"team": "platform",
					},
				},
			},
			mockResponse: dtias.Server{
				ID:           "new-server-002",
				Hostname:     "custom-host.example.com",
				ServerPoolID: "pool-compute",
				Type:         "r640",
				State:        "provisioning",
				PowerState:   "off",
				HealthState:  "unknown",
				CPU:          dtias.CPUInfo{Vendor: "Intel", Model: "Xeon", TotalCores: 16},
				Memory:       dtias.MemoryInfo{TotalGB: 64, Type: "DDR4"},
				Metadata: map[string]string{
					"o2ims.io/tenant-id": "tenant-a",
					"team":               "platform",
				},
				CreatedAt: now, UpdatedAt: now, LastHealthCheck: now,
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/v2/resources/allocate", r.URL.Path)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			result, err := adp.CreateResource(context.Background(), tt.resource)

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.mockResponse.ID, result.ResourceID)
			assert.NotEmpty(t, result.Description)
			assert.NotNil(t, result.Extensions)
		})
	}
}

// TestCreateResource_APIError tests CreateResource error handling.
func TestCreateResource_APIError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	result, err := adp.CreateResource(context.Background(), &adapter.Resource{
		ResourcePoolID: "pool-1",
		ResourceTypeID: "r640",
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to provision server")
}

// TestDeleteResource tests the DeleteResource method with a mock server.
func TestDeleteResource(t *testing.T) {
	tests := []struct {
		name       string
		serverID   string
		statusCode int
		expectErr  bool
	}{
		{
			name:       "delete existing resource",
			serverID:   "server-001",
			statusCode: http.StatusOK,
			expectErr:  false,
		},
		{
			name:       "delete with accepted status",
			serverID:   "server-002",
			statusCode: http.StatusAccepted,
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/v2/resources/release", r.URL.Path)

				// Verify request body contains the server ID
				var reqBody map[string]interface{}
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.Equal(t, tt.serverID, reqBody["id"])

				w.WriteHeader(tt.statusCode)
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			err := adp.DeleteResource(context.Background(), tt.serverID)

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

// TestDeleteResource_APIError tests DeleteResource error handling.
func TestDeleteResource_APIError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	err := adp.DeleteResource(context.Background(), "server-error")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decommission server")
}

// TestPowerControl tests the PowerControl method with a mock server.
func TestPowerControl(t *testing.T) {
	tests := []struct {
		name       string
		serverID   string
		operation  dtias.ServerPowerOperation
		statusCode int
		expectErr  bool
	}{
		{
			name:       "power on",
			serverID:   "server-001",
			operation:  dtias.PowerOn,
			statusCode: http.StatusOK,
			expectErr:  false,
		},
		{
			name:       "power off",
			serverID:   "server-001",
			operation:  dtias.PowerOff,
			statusCode: http.StatusOK,
			expectErr:  false,
		},
		{
			name:       "power reset",
			serverID:   "server-001",
			operation:  dtias.PowerReset,
			statusCode: http.StatusOK,
			expectErr:  false,
		},
		{
			name:       "power cycle",
			serverID:   "server-001",
			operation:  dtias.PowerCycle,
			statusCode: http.StatusOK,
			expectErr:  false,
		},
		{
			name:       "force power off",
			serverID:   "server-001",
			operation:  dtias.PowerForceOff,
			statusCode: http.StatusOK,
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/v2/resources/action", r.URL.Path)

				var reqBody map[string]interface{}
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.Equal(t, tt.serverID, reqBody["id"])
				assert.Equal(t, string(tt.operation), reqBody["action"])

				w.WriteHeader(tt.statusCode)
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			err := adp.PowerControl(context.Background(), tt.serverID, tt.operation)

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

// TestPowerControl_APIError tests PowerControl error handling.
func TestPowerControl_APIError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	err := adp.PowerControl(context.Background(), "server-error", dtias.PowerOn)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to perform power operation")
}

// TestGetHealthMetrics tests the GetHealthMetrics method with a mock server.
func TestGetHealthMetrics(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "/v2/inventory/servers/server-001")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(dtias.HealthMetrics{
			ServerID:              "server-001",
			CPUUtilization:        45.5,
			MemoryUtilization:     62.3,
			CPUTemperature:        55.0,
			PowerConsumptionWatts: 350,
			FanSpeeds: []dtias.FanSpeed{
				{Name: "Fan1", SpeedRPM: 3500, SpeedPercent: 50, Status: "ok"},
			},
			Temperatures: []dtias.TemperatureSensor{
				{Name: "CPU0", TemperatureCelsius: 55.0, Status: "ok"},
			},
		})
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	metrics, err := adp.GetHealthMetrics(context.Background(), "server-001")
	require.NoError(t, err)
	require.NotNil(t, metrics)
	assert.Equal(t, 45.5, metrics.CPUUtilization)
	assert.Equal(t, 62.3, metrics.MemoryUtilization)
	assert.Equal(t, 55.0, metrics.CPUTemperature)
	assert.Equal(t, 350, metrics.PowerConsumptionWatts)
	assert.Len(t, metrics.FanSpeeds, 1)
	assert.Len(t, metrics.Temperatures, 1)
}

// TestGetHealthMetrics_APIError tests GetHealthMetrics error handling.
func TestGetHealthMetrics_APIError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	metrics, err := adp.GetHealthMetrics(context.Background(), "server-error")
	require.Error(t, err)
	assert.Nil(t, metrics)
	assert.Contains(t, err.Error(), "failed to get health metrics")
}

// TestBuildServersPath tests the buildServersPath function indirectly through ListResources.
func TestBuildServersPath(t *testing.T) {
	tests := []struct {
		name        string
		filter      *adapter.Filter
		expectPath  string
		expectQuery map[string]string
	}{
		{
			name:       "nil filter produces base path",
			filter:     nil,
			expectPath: "/v2/inventory/servers",
		},
		{
			name: "filter with resource pool ID",
			filter: &adapter.Filter{
				ResourcePoolID: "pool-1",
			},
			expectQuery: map[string]string{"resourcePool": "pool-1"},
		},
		{
			name: "filter with resource type ID",
			filter: &adapter.Filter{
				ResourceTypeID: "type-1",
			},
			expectQuery: map[string]string{"resourceProfileId": "type-1"},
		},
		{
			name: "filter with location",
			filter: &adapter.Filter{
				Location: "dc-dallas",
			},
			expectQuery: map[string]string{"location": "dc-dallas"},
		},
		{
			name: "filter with pagination",
			filter: &adapter.Filter{
				Limit:  10,
				Offset: 20,
			},
			expectQuery: map[string]string{
				"pageSize":   "10",
				"pageNumber": "3", // (20/10) + 1 = 3
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.expectPath != "" {
					assert.Equal(t, tt.expectPath, r.URL.Path)
				}
				for key, value := range tt.expectQuery {
					assert.Equal(t, value, r.URL.Query().Get(key),
						"expected query param %s=%s", key, value)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(dtias.ServersInventoryResponse{
					Full: []dtias.Server{},
				})
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			_, err := adp.ListResources(context.Background(), tt.filter)
			require.NoError(t, err)
		})
	}
}

// TestTransformServerToResource tests the server-to-resource transformation indirectly.
func TestTransformServerToResource(t *testing.T) {
	now := time.Now()
	server := dtias.Server{
		ID:           "server-001",
		Hostname:     "web-server-1.example.com",
		ServerPoolID: "pool-compute",
		Type:         "r640",
		State:        "ready",
		PowerState:   "on",
		HealthState:  "healthy",
		CPU: dtias.CPUInfo{
			Vendor:         "Intel",
			Model:          "Xeon Gold 6248R",
			Architecture:   "x86_64",
			Sockets:        2,
			CoresPerSocket: 24,
			TotalCores:     48,
			TotalThreads:   96,
			FrequencyMHz:   2400,
		},
		Memory: dtias.MemoryInfo{
			TotalGB:        256,
			AvailableGB:    200,
			Type:           "DDR4",
			SpeedMHz:       3200,
			DIMMs:          16,
			SlotsUsed:      16,
			SlotsAvailable: 0,
		},
		BIOS: dtias.BIOSInfo{
			Vendor:      "Dell",
			Version:     "2.14.0",
			ReleaseDate: "2023-06-15",
		},
		Management: dtias.ManagementInfo{
			Type:       "idrac",
			Version:    "6.00.30.00",
			IPAddress:  "10.0.0.100",
			MACAddress: "AA:BB:CC:DD:EE:FF",
			Hostname:   "idrac-001.example.com",
		},
		Location: dtias.Location{
			Datacenter: "dc-dallas-1",
			Rack:       "rack-a01",
			RackUnit:   15,
			City:       "Dallas",
			Country:    "US",
		},
		Metadata: map[string]string{
			"env":                "production",
			"o2ims.io/tenant-id": "tenant-a",
		},
		CreatedAt:       now,
		UpdatedAt:       now,
		LastHealthCheck: now,
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(dtias.ServersInventoryResponse{
			Full: []dtias.Server{server},
		})
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	resources, err := adp.ListResources(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	res := resources[0]
	assert.Equal(t, "server-001", res.ResourceID)
	assert.Equal(t, "tenant-a", res.TenantID)
	assert.Equal(t, "dtias-server-type-r640", res.ResourceTypeID)
	assert.Equal(t, "pool-compute", res.ResourcePoolID)
	assert.Equal(t, "urn:dtias:server:server-001", res.GlobalAssetID)
	assert.Contains(t, res.Description, "web-server-1.example.com")
	assert.Contains(t, res.Description, "r640")

	// Verify extensions
	ext := res.Extensions
	assert.Equal(t, "server-001", ext["dtias.serverId"])
	assert.Equal(t, "web-server-1.example.com", ext["dtias.hostname"])
	assert.Equal(t, "r640", ext["dtias.serverType"])
	assert.Equal(t, "ready", ext["dtias.state"])
	assert.Equal(t, "on", ext["dtias.powerState"])
	assert.Equal(t, "healthy", ext["dtias.healthState"])

	// CPU info
	assert.Equal(t, "Intel", ext["dtias.cpu.vendor"])
	assert.Equal(t, "Xeon Gold 6248R", ext["dtias.cpu.model"])
	assert.Equal(t, "x86_64", ext["dtias.cpu.architecture"])
	assert.Equal(t, 48, ext["dtias.cpu.totalCores"])

	// Memory info
	assert.Equal(t, 256, ext["dtias.memory.totalGb"])
	assert.Equal(t, "DDR4", ext["dtias.memory.type"])

	// Location info
	assert.Equal(t, "dc-dallas-1", ext["dtias.location.datacenter"])
	assert.Equal(t, "rack-a01", ext["dtias.location.rack"])

	// Metadata
	metadata, ok := ext["dtias.metadata"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "production", metadata["env"])
}

// TestTransformServerToResource_UnhealthyServer tests description for unhealthy servers.
func TestTransformServerToResource_UnhealthyServer(t *testing.T) {
	now := time.Now()
	server := dtias.Server{
		ID:           "server-sick",
		Hostname:     "sick-server.example.com",
		ServerPoolID: "pool-1",
		Type:         "r640",
		State:        "error",
		PowerState:   "on",
		HealthState:  "critical",
		CPU:          dtias.CPUInfo{Vendor: "Intel", Model: "Xeon", TotalCores: 16},
		Memory:       dtias.MemoryInfo{TotalGB: 64, Type: "DDR4"},
		Metadata:     map[string]string{},
		CreatedAt:    now, UpdatedAt: now, LastHealthCheck: now,
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(dtias.ServersInventoryResponse{
			Full: []dtias.Server{server},
		})
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	resources, err := adp.ListResources(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	// Unhealthy server should include health info in description
	assert.Contains(t, resources[0].Description, "critical")
}

// TestMatchesResourceFilter tests the resource filter matching logic indirectly.
func TestMatchesResourceFilter(t *testing.T) {
	now := time.Now()
	servers := []dtias.Server{
		{
			ID: "server-pool1-type1", Hostname: "s1.example.com",
			ServerPoolID: "pool-1", Type: "r640",
			State: "ready", PowerState: "on", HealthState: "healthy",
			CPU: dtias.CPUInfo{TotalCores: 16}, Memory: dtias.MemoryInfo{TotalGB: 64, Type: "DDR4"},
			Metadata:  map[string]string{"env": "prod", "tier": "gold"},
			CreatedAt: now, UpdatedAt: now, LastHealthCheck: now,
		},
		{
			ID: "server-pool2-type2", Hostname: "s2.example.com",
			ServerPoolID: "pool-2", Type: "r740xd",
			State: "ready", PowerState: "on", HealthState: "healthy",
			CPU: dtias.CPUInfo{TotalCores: 48}, Memory: dtias.MemoryInfo{TotalGB: 512, Type: "DDR4"},
			Metadata:  map[string]string{"env": "staging"},
			CreatedAt: now, UpdatedAt: now, LastHealthCheck: now,
		},
	}

	tests := []struct {
		name          string
		filter        *adapter.Filter
		expectedCount int
	}{
		{
			name:          "no filter returns all",
			filter:        nil,
			expectedCount: 2,
		},
		{
			name:          "filter by pool matches one",
			filter:        &adapter.Filter{ResourcePoolID: "pool-1"},
			expectedCount: 1,
		},
		{
			name:          "filter by type matches one",
			filter:        &adapter.Filter{ResourceTypeID: "dtias-server-type-r640"},
			expectedCount: 1,
		},
		{
			name: "filter by matching label",
			filter: &adapter.Filter{
				Labels: map[string]string{"env": "prod"},
			},
			expectedCount: 1,
		},
		{
			name: "filter by non-matching label",
			filter: &adapter.Filter{
				Labels: map[string]string{"env": "dev"},
			},
			expectedCount: 0,
		},
		{
			name: "filter by multiple labels (all must match)",
			filter: &adapter.Filter{
				Labels: map[string]string{"env": "prod", "tier": "gold"},
			},
			expectedCount: 1,
		},
		{
			name: "filter by multiple labels (partial match fails)",
			filter: &adapter.Filter{
				Labels: map[string]string{"env": "prod", "tier": "silver"},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(dtias.ServersInventoryResponse{
					Full: servers,
				})
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			result, err := adp.ListResources(context.Background(), tt.filter)
			require.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}
