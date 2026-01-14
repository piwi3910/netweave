package starlingx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/adapter"
)

func TestGetDeploymentManager(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name           string
		requestedID    string
		systemResponse []ISystem
		wantErr        bool
		errType        error
	}{
		{
			name:        "successful retrieval",
			requestedID: "test-dm-1",
			systemResponse: []ISystem{
				{
					UUID:            "system-uuid-1",
					Name:            "starlingx-system",
					SystemType:      "All-in-one",
					SystemMode:      "simplex",
					Description:     "Test System",
					Location:        "Ottawa",
					Latitude:        "45.4215",
					Longitude:       "-75.6972",
					Timezone:        "UTC",
					SoftwareVersion: "8.0",
				},
			},
			wantErr: false,
		},
		{
			name:        "wrong deployment manager ID",
			requestedID: "wrong-dm-id",
			wantErr:     true,
			errType:     adapter.ErrDeploymentManagerNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock Keystone server
			keystoneMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v3/auth/tokens" {
					w.Header().Set("X-Subject-Token", "mock-token-123")
					w.WriteHeader(http.StatusCreated)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"token": map[string]interface{}{
							"expires_at": "2099-12-31T23:59:59.000000Z",
						},
					})
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer keystoneMock.Close()

			// Create mock StarlingX server
			starlingxMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/isystems" && r.Method == http.MethodGet {
					w.Header().Set("Content-Type", "application/json")
					response := struct {
						Systems []ISystem `json:"isystems"`
					}{
						Systems: tt.systemResponse,
					}
					json.NewEncoder(w).Encode(response)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer starlingxMock.Close()

			// Create adapter with mock servers
			adp, err := New(&Config{
				Endpoint:            starlingxMock.URL,
				KeystoneEndpoint:    keystoneMock.URL,
				Username:            "testuser",
				Password:            "testpass",
				OCloudID:            "test-ocloud",
				DeploymentManagerID: "test-dm-1",
				Logger:              logger,
			})
			require.NoError(t, err)
			defer adp.Close()

			// Execute GetDeploymentManager
			ctx := context.Background()
			dm, err := adp.GetDeploymentManager(ctx, tt.requestedID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					require.ErrorIs(t, err, tt.errType)
				}
				assert.Nil(t, dm)
			} else {
				require.NoError(t, err)
				require.NotNil(t, dm)

				assert.Equal(t, "test-dm-1", dm.DeploymentManagerID)
				assert.Equal(t, "starlingx-system", dm.Name)
				assert.Equal(t, "Test System", dm.Description)
				assert.Equal(t, "test-ocloud", dm.OCloudID)
				assert.Contains(t, dm.ServiceURI, "/o2ims-infrastructureInventory/v1")
				assert.Contains(t, dm.SupportedLocations, "Ottawa")
				assert.Contains(t, dm.Capabilities, "compute-provisioning")
				assert.Contains(t, dm.Capabilities, "label-based-pooling")

				// Check extensions
				assert.Equal(t, "All-in-one", dm.Extensions["system_type"])
				assert.Equal(t, "simplex", dm.Extensions["system_mode"])
				assert.Equal(t, "8.0", dm.Extensions["software_version"])
			}
		})
	}
}
