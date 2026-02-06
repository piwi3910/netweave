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

// TestGetDeploymentManager tests the GetDeploymentManager method with a mock server.
func TestGetDeploymentManager(t *testing.T) {
	tests := []struct {
		name              string
		requestID         string
		datacenterHandler func(w http.ResponseWriter, r *http.Request)
		expectErr         bool
		errContains       string
		validate          func(*testing.T, *dtias.Adapter, *interface{})
	}{
		{
			name:      "valid ID returns deployment manager metadata",
			requestID: "dm-test",
			datacenterHandler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(dtias.DatacenterInfo{
					ID:        "dc-test",
					Name:      "Test Datacenter",
					City:      "Dallas",
					Country:   "US",
					Latitude:  32.7767,
					Longitude: -96.7970,
				})
			},
			expectErr: false,
		},
		{
			name:      "empty ID is accepted as alias",
			requestID: "",
			datacenterHandler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(dtias.DatacenterInfo{
					ID:   "dc-test",
					Name: "Test Datacenter",
				})
			},
			expectErr: false,
		},
		{
			name:      "default ID is accepted as alias",
			requestID: "default",
			datacenterHandler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(dtias.DatacenterInfo{
					ID:   "dc-test",
					Name: "Test Datacenter",
				})
			},
			expectErr: false,
		},
		{
			name:      "wrong ID returns error",
			requestID: "wrong-dm-id",
			datacenterHandler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectErr:   true,
			errContains: "deployment manager not found",
		},
		{
			name:      "datacenter info failure uses defaults gracefully",
			requestID: "dm-test",
			datacenterHandler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error": "internal error"}`))
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.datacenterHandler(w, r)
			}))
			defer mockServer.Close()

			adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())
			adp.DeploymentManagerID = "dm-test"
			adp.OCloudID = "ocloud-test"
			adp.Config = &dtias.Config{
				Endpoint:   mockServer.URL,
				Datacenter: "dc-test",
			}

			dm, err := adp.GetDeploymentManager(context.Background(), tt.requestID)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, dm)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, dm)

			// Verify common deployment manager fields
			assert.Equal(t, "dm-test", dm.DeploymentManagerID)
			assert.Equal(t, "ocloud-test", dm.OCloudID)
			assert.Equal(t, mockServer.URL, dm.ServiceURI)
			assert.Contains(t, dm.Name, "DTIAS")
			assert.Contains(t, dm.Description, "DTIAS")
			assert.NotEmpty(t, dm.Capabilities)
			assert.NotNil(t, dm.Extensions)

			// Verify extensions contain expected keys
			assert.Equal(t, mockServer.URL, dm.Extensions["dtias.endpoint"])
			assert.Equal(t, "dc-test", dm.Extensions["dtias.datacenter"])
			assert.Equal(t, "1.0", dm.Extensions["dtias.apiVersion"])
		})
	}
}

// TestGetDeploymentManager_WithDatacenterLocation tests that datacenter location info is included.
func TestGetDeploymentManager_WithDatacenterLocation(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(dtias.DatacenterInfo{
			ID:        "dc-dallas",
			Name:      "Dallas DC",
			City:      "Dallas",
			Country:   "US",
			Latitude:  32.7767,
			Longitude: -96.7970,
		})
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())
	adp.DeploymentManagerID = "dm-test"
	adp.OCloudID = "ocloud-test"
	adp.Config = &dtias.Config{
		Endpoint:   mockServer.URL,
		Datacenter: "dc-dallas",
	}

	dm, err := adp.GetDeploymentManager(context.Background(), "dm-test")
	require.NoError(t, err)
	require.NotNil(t, dm)

	// Verify datacenter location info is present
	assert.NotEmpty(t, dm.SupportedLocations)
	assert.Contains(t, dm.SupportedLocations, "dc-dallas")
	assert.Equal(t, "Dallas", dm.Extensions["dtias.location.city"])
	assert.Equal(t, "US", dm.Extensions["dtias.location.country"])
	assert.Equal(t, 32.7767, dm.Extensions["dtias.location.latitude"])
	assert.Equal(t, -96.7970, dm.Extensions["dtias.location.longitude"])
}

// TestGetDeploymentManager_DatacenterInfoPartial tests partial datacenter info.
func TestGetDeploymentManager_DatacenterInfoPartial(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Return datacenter info with only city (no lat/lon)
		_ = json.NewEncoder(w).Encode(dtias.DatacenterInfo{
			ID:   "dc-test",
			Name: "Test DC",
			City: "Dallas",
		})
	}))
	defer mockServer.Close()

	adp := dtias.NewTestAdapter(mockServer.URL, mockServer.Client(), zap.NewNop())
	adp.DeploymentManagerID = "dm-test"
	adp.OCloudID = "ocloud-test"
	adp.Config = &dtias.Config{
		Endpoint:   mockServer.URL,
		Datacenter: "dc-test",
	}

	dm, err := adp.GetDeploymentManager(context.Background(), "dm-test")
	require.NoError(t, err)
	require.NotNil(t, dm)

	// City should be present
	assert.Equal(t, "Dallas", dm.Extensions["dtias.location.city"])
	// Country should not be present (empty string)
	_, hasCountry := dm.Extensions["dtias.location.country"]
	assert.False(t, hasCountry)
	// Lat/lon should not be present (zero values)
	_, hasLat := dm.Extensions["dtias.location.latitude"]
	assert.False(t, hasLat)
}
