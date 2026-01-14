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
