package starlingx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/piwi3910/netweave/internal/adapters/starlingx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestGetDeploymentManager(t *testing.T) {
	// Create mock servers
	keystoneMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/auth/tokens" && r.Method == http.MethodPost {
			w.Header().Set("X-Subject-Token", "mock-token-123")
			w.WriteHeader(http.StatusCreated)
			resp := map[string]interface{}{
				"token": map[string]interface{}{
					"expires_at": "2099-12-31T23:59:59.000000Z",
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer keystoneMock.Close()

	testSystem := starlingx.ISystem{
		UUID:        "system-uuid-1",
		Name:        "starlingx-test",
		Description: "Test StarlingX System",
		SoftwareVersion: "8.0",
		Capabilities: map[string]interface{}{
			"region": "RegionOne",
		},
	}

	starlingxMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/v1/isystems" && r.Method == http.MethodGet {
			response := struct {
				Systems []starlingx.ISystem `json:"isystems"`
			}{
				Systems: []starlingx.ISystem{testSystem},
			}
			if err := json.NewEncoder(w).Encode(response); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer starlingxMock.Close()

	// Create adapter with mock endpoints
	logger := zaptest.NewLogger(t)
	tests := []struct {
		name    string
		dmID    string
		wantErr bool
	}{
		{
			name:    "successful retrieval",
			dmID:    "test-dm-1",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := starlingx.New(&starlingx.Config{
				Endpoint:            starlingxMock.URL,
				KeystoneEndpoint:    keystoneMock.URL,
				Username:            "admin",
				Password:            "secret",
				OCloudID:            "test-ocloud",
				DeploymentManagerID: tt.dmID,
				Logger:              logger,
			})
			require.NoError(t, err)
			defer func() { _ = adp.Close() }()

			ctx := context.Background()
			dm, err := adp.GetDeploymentManager(ctx, tt.dmID)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, dm)
				assert.Equal(t, tt.dmID, dm.DeploymentManagerID)
				assert.Equal(t, "test-ocloud", dm.OCloudID)
				assert.NotEmpty(t, dm.Name)
				assert.NotEmpty(t, dm.ServiceURI)
			}
		})
	}
}
