package dtias_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapters/dtias"
)

// TestListResourceTypes tests the ListResourceTypes method with a mock server.
func TestListResourceTypes(t *testing.T) {
	tests := []struct {
		name          string
		serverTypes   []dtias.ServerType
		expectedCount int
		expectErr     bool
	}{
		{
			name: "list multiple resource types",
			serverTypes: []dtias.ServerType{
				{
					ID:                "r640",
					Name:              "Dell PowerEdge R640",
					Vendor:            "Dell",
					Model:             "PowerEdge R640",
					Generation:        "14G",
					FormFactor:        "rack",
					CPUModel:          "Xeon Gold 6248R",
					CPUCores:          48,
					MemoryGB:          256,
					StorageType:       "nvme",
					StorageCapacityGB: 2000,
					NetworkPorts:      4,
					NetworkSpeed:      "25Gbps",
					PowerWatts:        750,
					RackUnits:         1,
				},
				{
					ID:                "r740xd",
					Name:              "Dell PowerEdge R740xd",
					Vendor:            "Dell",
					Model:             "PowerEdge R740xd",
					Generation:        "14G",
					FormFactor:        "rack",
					CPUModel:          "Xeon Gold 6248R",
					CPUCores:          48,
					MemoryGB:          512,
					StorageType:       "ssd",
					StorageCapacityGB: 100000,
					NetworkPorts:      4,
					NetworkSpeed:      "25Gbps",
					PowerWatts:        1100,
					RackUnits:         2,
				},
			},
			expectedCount: 2,
			expectErr:     false,
		},
		{
			name:          "list empty resource types",
			serverTypes:   []dtias.ServerType{},
			expectedCount: 0,
			expectErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Contains(t, r.URL.Path, "/v2/resourcetypes")

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(dtias.ResourceTypesResponse{
					ResourceTypes: tt.serverTypes,
				})
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			result, err := adp.ListResourceTypes(context.Background(), nil)

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)

			for i, rt := range result {
				assert.NotEmpty(t, rt.ResourceTypeID)
				assert.Equal(t, tt.serverTypes[i].Name, rt.Name)
				assert.Equal(t, tt.serverTypes[i].Vendor, rt.Vendor)
				assert.Equal(t, tt.serverTypes[i].Model, rt.Model)
				assert.Equal(t, "physical", rt.ResourceKind)
				assert.NotEmpty(t, rt.Description)
				assert.NotNil(t, rt.Extensions)
			}
		})
	}
}

// TestListResourceTypes_APIError tests ListResourceTypes error handling.
func TestListResourceTypes_APIError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	result, err := adp.ListResourceTypes(context.Background(), nil)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to")
}

// TestGetResourceType tests the GetResourceType method with a mock server.
func TestGetResourceType(t *testing.T) {
	tests := []struct {
		name         string
		resourceID   string
		serverTypes  []dtias.ServerType
		expectErr    bool
		errContains  string
		validateName string
	}{
		{
			name:       "get existing resource type",
			resourceID: "r640",
			serverTypes: []dtias.ServerType{
				{
					ID:                "r640",
					Name:              "Dell PowerEdge R640",
					Vendor:            "Dell",
					Model:             "PowerEdge R640",
					Generation:        "14G",
					FormFactor:        "rack",
					CPUModel:          "Xeon Gold 6248R",
					CPUCores:          48,
					MemoryGB:          256,
					StorageType:       "nvme",
					StorageCapacityGB: 2000,
					NetworkPorts:      4,
					NetworkSpeed:      "25Gbps",
					PowerWatts:        750,
					RackUnits:         1,
				},
			},
			expectErr:    false,
			validateName: "Dell PowerEdge R640",
		},
		{
			name:        "resource type not found (empty response)",
			resourceID:  "nonexistent",
			serverTypes: []dtias.ServerType{},
			expectErr:   true,
			errContains: "server type not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Contains(t, r.URL.Path, "/v2/search/resourcetypes/")

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(dtias.ResourceTypesResponse{
					ResourceTypes: tt.serverTypes,
				})
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			result, err := adp.GetResourceType(context.Background(), tt.resourceID)

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
			assert.Equal(t, tt.validateName, result.Name)
			assert.NotEmpty(t, result.ResourceTypeID)
			assert.Equal(t, "physical", result.ResourceKind)
		})
	}
}

// TestGetResourceType_APIError tests GetResourceType error handling.
func TestGetResourceType_APIError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	result, err := adp.GetResourceType(context.Background(), "r640")
	require.Error(t, err)
	assert.Nil(t, result)
}

// TestTransformServerTypeToResourceType_ResourceClassDetection tests the resource class detection logic.
func TestTransformServerTypeToResourceType_ResourceClassDetection(t *testing.T) {
	tests := []struct {
		name          string
		serverType    dtias.ServerType
		expectedClass string
	}{
		{
			name: "compute class (default)",
			serverType: dtias.ServerType{
				ID:                "compute-1",
				Name:              "Compute Node",
				Vendor:            "Dell",
				CPUCores:          48,
				MemoryGB:          256,
				StorageCapacityGB: 2000,
				NetworkPorts:      4,
				NetworkSpeed:      "25Gbps",
			},
			expectedClass: "compute",
		},
		{
			name: "storage class (storage capacity >> memory * 100)",
			serverType: dtias.ServerType{
				ID:                "storage-1",
				Name:              "Storage Node",
				Vendor:            "Dell",
				CPUCores:          16,
				MemoryGB:          64,
				StorageCapacityGB: 100000, // 100000 > 64*100=6400
				NetworkPorts:      4,
				NetworkSpeed:      "25Gbps",
			},
			expectedClass: "storage",
		},
		{
			name: "network class (many network ports > 4)",
			serverType: dtias.ServerType{
				ID:                "network-1",
				Name:              "Network Node",
				Vendor:            "Dell",
				CPUCores:          16,
				MemoryGB:          64,
				StorageCapacityGB: 500, // 500 < 64*100=6400
				NetworkPorts:      8,   // > 4
				NetworkSpeed:      "100Gbps",
			},
			expectedClass: "network",
		},
		{
			name: "storage takes priority over network",
			serverType: dtias.ServerType{
				ID:                "storage-net-1",
				Name:              "Storage+Network Node",
				Vendor:            "Dell",
				CPUCores:          16,
				MemoryGB:          32,
				StorageCapacityGB: 50000, // 50000 > 32*100=3200
				NetworkPorts:      8,     // > 4
				NetworkSpeed:      "100Gbps",
			},
			expectedClass: "storage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We need to use the mock server pattern to create an adapter
			// and then call ListResourceTypes to get the transformed types
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(dtias.ResourceTypesResponse{
					ResourceTypes: []dtias.ServerType{tt.serverType},
				})
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

			result, err := adp.ListResourceTypes(context.Background(), nil)
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, tt.expectedClass, result[0].ResourceClass)
		})
	}
}

// TestTransformServerTypeToResourceType_Extensions tests that all extensions are populated.
func TestTransformServerTypeToResourceType_Extensions(t *testing.T) {
	serverType := dtias.ServerType{
		ID:                "r640",
		Name:              "Dell PowerEdge R640",
		Vendor:            "Dell",
		Model:             "PowerEdge R640",
		Generation:        "14G",
		FormFactor:        "rack",
		CPUModel:          "Xeon Gold 6248R",
		CPUCores:          48,
		MemoryGB:          256,
		StorageType:       "nvme",
		StorageCapacityGB: 2000,
		NetworkPorts:      4,
		NetworkSpeed:      "25Gbps",
		PowerWatts:        750,
		RackUnits:         1,
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(dtias.ResourceTypesResponse{
			ResourceTypes: []dtias.ServerType{serverType},
		})
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())

	result, err := adp.ListResourceTypes(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result, 1)

	rt := result[0]
	assert.Equal(t, "dtias-server-type-r640", rt.ResourceTypeID)
	assert.Equal(t, "Dell PowerEdge R640", rt.Name)
	assert.Equal(t, "Dell", rt.Vendor)
	assert.Equal(t, "PowerEdge R640", rt.Model)
	assert.Equal(t, "14G", rt.Version)
	assert.Equal(t, "physical", rt.ResourceKind)

	// Verify all extension keys
	ext := rt.Extensions
	assert.Equal(t, "r640", ext["dtias.serverTypeId"])
	assert.Equal(t, "Dell", ext["dtias.vendor"])
	assert.Equal(t, "PowerEdge R640", ext["dtias.model"])
	assert.Equal(t, "14G", ext["dtias.generation"])
	assert.Equal(t, "rack", ext["dtias.formFactor"])
	assert.Equal(t, "Xeon Gold 6248R", ext["dtias.cpu.model"])
	assert.Equal(t, 48, ext["dtias.cpu.cores"])
	assert.Equal(t, 256, ext["dtias.memory.sizeGb"])
	assert.Equal(t, "nvme", ext["dtias.storage.type"])
	assert.Equal(t, 2000, ext["dtias.storage.capacityGb"])
	assert.Equal(t, 4, ext["dtias.network.ports"])
	assert.Equal(t, "25Gbps", ext["dtias.network.speed"])
	assert.Equal(t, 750, ext["dtias.power.watts"])
	assert.Equal(t, 1, ext["dtias.rack.units"])

	// Verify description contains hardware summary
	assert.Contains(t, rt.Description, "48 cores")
	assert.Contains(t, rt.Description, "256GB RAM")
	assert.Contains(t, rt.Description, "2000GB storage")
	assert.Contains(t, rt.Description, "25Gbps")
}
