package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// mockDeploymentManagerAdapter is a mock implementation of adapter.Adapter for testing.
type mockDeploymentManagerAdapter struct {
	adapter.Adapter
	getDeploymentManagerFunc func(ctx context.Context, id string) (*adapter.DeploymentManager, error)
}

func (m *mockDeploymentManagerAdapter) GetDeploymentManager(
	ctx context.Context,
	id string,
) (*adapter.DeploymentManager, error) {
	if m.getDeploymentManagerFunc != nil {
		return m.getDeploymentManagerFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

// TestNewDeploymentManagerHandler tests handler creation.
func TestNewDeploymentManagerHandler(t *testing.T) {
	logger := zap.NewNop()
	mockAdapter := &mockDeploymentManagerAdapter{}

	t.Run("valid creation", func(t *testing.T) {
		handler := NewDeploymentManagerHandler(mockAdapter, logger)
		assert.NotNil(t, handler)
		assert.Equal(t, mockAdapter, handler.adapter)
		assert.Equal(t, logger, handler.logger)
	})

	t.Run("nil adapter panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewDeploymentManagerHandler(nil, logger)
		})
	})

	t.Run("nil logger panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewDeploymentManagerHandler(mockAdapter, nil)
		})
	})
}

// TestListDeploymentManagers tests the ListDeploymentManagers handler.
func TestListDeploymentManagers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	tests := []struct {
		name           string
		queryParams    string
		mockFunc       func(ctx context.Context, id string) (*adapter.DeploymentManager, error)
		expectedStatus int
		checkResponse  func(t *testing.T, w *httptest.ResponseRecorder)
	}{
		{
			name:        "successful list",
			queryParams: "",
			mockFunc: func(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
				return &adapter.DeploymentManager{
					DeploymentManagerID: "dm-1",
					Name:                "Test DM",
					Description:         "Test deployment manager",
					OCloudID:            "ocloud-1",
					ServiceURI:          "https://dm.example.com",
					SupportedLocations:  []string{"us-east-1"},
					Capabilities:        []string{"compute", "storage"},
				}, nil
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				assert.Contains(t, w.Body.String(), "dm-1")
				assert.Contains(t, w.Body.String(), "Test DM")
				assert.Contains(t, w.Body.String(), "totalCount")
			},
		},
		{
			name:        "with pagination",
			queryParams: "?offset=0&limit=10",
			mockFunc: func(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
				return &adapter.DeploymentManager{
					DeploymentManagerID: "dm-1",
					Name:                "Test DM",
				}, nil
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				assert.Contains(t, w.Body.String(), "dm-1")
			},
		},
		{
			name:        "adapter error",
			queryParams: "",
			mockFunc: func(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
				return nil, errors.New("adapter failure")
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				assert.Contains(t, w.Body.String(), "InternalError")
				assert.Contains(t, w.Body.String(), "Failed to retrieve deployment managers")
			},
		},
		{
			name:        "high offset pagination",
			queryParams: "?offset=100&limit=10",
			mockFunc: func(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
				return &adapter.DeploymentManager{
					DeploymentManagerID: "dm-1",
					Name:                "Test DM",
				}, nil
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				// Should return empty items array when offset > total
				assert.Contains(t, w.Body.String(), "\"items\":[]")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAdapter := &mockDeploymentManagerAdapter{
				getDeploymentManagerFunc: tt.mockFunc,
			}
			handler := NewDeploymentManagerHandler(mockAdapter, logger)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/deploymentManagers"+tt.queryParams, nil)
			c.Request = req

			handler.ListDeploymentManagers(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}
}

// TestGetDeploymentManager tests the GetDeploymentManager handler.
func TestGetDeploymentManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	tests := []struct {
		name            string
		deploymentMgrID string
		mockFunc        func(ctx context.Context, id string) (*adapter.DeploymentManager, error)
		expectedStatus  int
		checkResponse   func(t *testing.T, w *httptest.ResponseRecorder)
	}{
		{
			name:            "successful get",
			deploymentMgrID: "dm-123",
			mockFunc: func(_ context.Context, id string) (*adapter.DeploymentManager, error) {
				require.Equal(t, "dm-123", id)
				return &adapter.DeploymentManager{
					DeploymentManagerID: "dm-123",
					Name:                "Test DM",
					Description:         "Test deployment manager",
					OCloudID:            "ocloud-1",
					ServiceURI:          "https://dm.example.com",
					SupportedLocations:  []string{"us-east-1", "us-west-2"},
					Capabilities:        []string{"compute", "storage", "networking"},
					Extensions: map[string]interface{}{
						"version": "1.0.0",
					},
				}, nil
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				assert.Contains(t, w.Body.String(), "dm-123")
				assert.Contains(t, w.Body.String(), "Test DM")
				assert.Contains(t, w.Body.String(), "ocloud-1")
				assert.Contains(t, w.Body.String(), "us-east-1")
			},
		},
		{
			name:            "empty ID",
			deploymentMgrID: "",
			mockFunc: func(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
				return nil, errors.New("should not be called with empty ID")
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				assert.Contains(t, w.Body.String(), "BadRequest")
				assert.Contains(t, w.Body.String(), "cannot be empty")
			},
		},
		{
			name:            "not found",
			deploymentMgrID: "dm-404",
			mockFunc: func(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
				return nil, errors.New("deployment manager not found")
			},
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				assert.Contains(t, w.Body.String(), "NotFound")
				assert.Contains(t, w.Body.String(), "dm-404")
			},
		},
		{
			name:            "adapter error",
			deploymentMgrID: "dm-error",
			mockFunc: func(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
				return nil, errors.New("internal adapter error")
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				assert.Contains(t, w.Body.String(), "InternalError")
				assert.Contains(t, w.Body.String(), "Failed to retrieve deployment manager")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAdapter := &mockDeploymentManagerAdapter{
				getDeploymentManagerFunc: tt.mockFunc,
			}
			handler := NewDeploymentManagerHandler(mockAdapter, logger)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/deploymentManagers/"+tt.deploymentMgrID, nil)
			c.Request = req
			c.Params = gin.Params{{Key: "deploymentManagerId", Value: tt.deploymentMgrID}}

			handler.GetDeploymentManager(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}
}
