package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/dms/adapter"
	dmsregistry "github.com/piwi3910/netweave/internal/dms/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Sentinel error for testing.
var errNotImplemented = errors.New("not implemented in mock")

// mockDMSAdapter implements adapter.DMSAdapter for testing.
type mockDMSAdapter struct {
	name         string
	version      string
	capabilities []adapter.Capability
}

func newMockDMSAdapter(name string) *mockDMSAdapter {
	return &mockDMSAdapter{
		name:         name,
		version:      "1.0.0",
		capabilities: []adapter.Capability{adapter.CapabilityDeploymentLifecycle},
	}
}

func (m *mockDMSAdapter) Name() string                       { return m.name }
func (m *mockDMSAdapter) Version() string                    { return m.version }
func (m *mockDMSAdapter) Capabilities() []adapter.Capability { return m.capabilities }

func (m *mockDMSAdapter) ListDeploymentPackages(_ context.Context, _ *adapter.Filter) ([]*adapter.DeploymentPackage, error) {
	return nil, errNotImplemented
}
func (m *mockDMSAdapter) GetDeploymentPackage(_ context.Context, _ string) (*adapter.DeploymentPackage, error) {
	return nil, errNotImplemented
}
func (m *mockDMSAdapter) UploadDeploymentPackage(_ context.Context, _ *adapter.DeploymentPackageUpload) (*adapter.DeploymentPackage, error) {
	return nil, errNotImplemented
}
func (m *mockDMSAdapter) DeleteDeploymentPackage(_ context.Context, _ string) error { return nil }
func (m *mockDMSAdapter) ListDeployments(_ context.Context, _ *adapter.Filter) ([]*adapter.Deployment, error) {
	return nil, errNotImplemented
}
func (m *mockDMSAdapter) GetDeployment(_ context.Context, _ string) (*adapter.Deployment, error) {
	return nil, errNotImplemented
}
func (m *mockDMSAdapter) CreateDeployment(_ context.Context, _ *adapter.DeploymentRequest) (*adapter.Deployment, error) {
	return nil, errNotImplemented
}
func (m *mockDMSAdapter) UpdateDeployment(_ context.Context, _ string, _ *adapter.DeploymentUpdate) (*adapter.Deployment, error) {
	return nil, errNotImplemented
}
func (m *mockDMSAdapter) DeleteDeployment(_ context.Context, _ string) error { return nil }
func (m *mockDMSAdapter) ScaleDeployment(_ context.Context, _ string, _ int) error {
	return nil
}
func (m *mockDMSAdapter) RollbackDeployment(_ context.Context, _ string, _ int) error {
	return nil
}
func (m *mockDMSAdapter) GetDeploymentStatus(_ context.Context, _ string) (*adapter.DeploymentStatusDetail, error) {
	return nil, errNotImplemented
}
func (m *mockDMSAdapter) GetDeploymentHistory(_ context.Context, _ string) (*adapter.DeploymentHistory, error) {
	return nil, errNotImplemented
}
func (m *mockDMSAdapter) GetDeploymentLogs(_ context.Context, _ string, _ *adapter.LogOptions) ([]byte, error) {
	return nil, errNotImplemented
}
func (m *mockDMSAdapter) SupportsRollback() bool         { return true }
func (m *mockDMSAdapter) SupportsScaling() bool          { return true }
func (m *mockDMSAdapter) SupportsGitOps() bool           { return false }
func (m *mockDMSAdapter) Health(_ context.Context) error { return nil }
func (m *mockDMSAdapter) Close() error                   { return nil }

// setupTestServer creates a minimal test server with DMS routes.
func setupTestServer(t *testing.T) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	router := gin.New()

	srv := &Server{
		router: router,
		logger: logger,
	}

	return srv
}

func TestHandleDMSAPIInfo(t *testing.T) {
	srv := setupTestServer(t)

	// Set up the route directly.
	srv.router.GET("/o2dms", srv.handleDMSAPIInfo)

	req := httptest.NewRequest(http.MethodGet, "/o2dms", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "v1", response["api_version"])
	assert.Equal(t, "/o2dms/v1", response["base_path"])
	assert.Equal(t, "O-RAN O2-DMS (Deployment Management Service) API", response["description"])

	resources, ok := response["resources"].([]interface{})
	require.True(t, ok)
	assert.Len(t, resources, 4)
	assert.Contains(t, resources, "deploymentLifecycle")
	assert.Contains(t, resources, "nfDeployments")
	assert.Contains(t, resources, "nfDeploymentDescriptors")
	assert.Contains(t, resources, "subscriptions")

	operations, ok := response["operations"].([]interface{})
	require.True(t, ok)
	assert.Len(t, operations, 6)
	assert.Contains(t, operations, "instantiate")
	assert.Contains(t, operations, "terminate")
	assert.Contains(t, operations, "scale")
	assert.Contains(t, operations, "heal")
	assert.Contains(t, operations, "upgrade")
	assert.Contains(t, operations, "rollback")
}

func TestSetupDMSRoutes(t *testing.T) {
	srv := setupTestServer(t)
	logger := zap.NewNop()

	// Create a registry with a mock adapter.
	reg := dmsregistry.NewRegistry(logger, nil)
	mockAdp := newMockDMSAdapter("test-adapter")
	err := reg.Register(context.Background(), "test-adapter", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	// Set up DMS using the server method.
	srv.SetupDMS(reg)

	// Verify all routes are registered by checking each endpoint.
	routes := srv.router.Routes()
	routePaths := make(map[string][]string)
	for _, r := range routes {
		routePaths[r.Path] = append(routePaths[r.Path], r.Method)
	}

	// Check main DMS info endpoint.
	assert.Contains(t, routePaths["/o2dms"], "GET")

	// Check deployment lifecycle endpoint.
	assert.Contains(t, routePaths["/o2dms/v1/deploymentLifecycle"], "GET")

	// Check nfDeployments endpoints.
	assert.Contains(t, routePaths["/o2dms/v1/nfDeployments"], "GET")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeployments"], "POST")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeployments/:nfDeploymentId"], "GET")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeployments/:nfDeploymentId"], "PUT")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeployments/:nfDeploymentId"], "DELETE")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeployments/:nfDeploymentId/scale"], "POST")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeployments/:nfDeploymentId/rollback"], "POST")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeployments/:nfDeploymentId/status"], "GET")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeployments/:nfDeploymentId/history"], "GET")

	// Check nfDeploymentDescriptors endpoints.
	assert.Contains(t, routePaths["/o2dms/v1/nfDeploymentDescriptors"], "GET")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeploymentDescriptors"], "POST")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeploymentDescriptors/:nfDeploymentDescriptorId"], "GET")
	assert.Contains(t, routePaths["/o2dms/v1/nfDeploymentDescriptors/:nfDeploymentDescriptorId"], "DELETE")

	// Check subscriptions endpoints.
	assert.Contains(t, routePaths["/o2dms/v1/subscriptions"], "GET")
	assert.Contains(t, routePaths["/o2dms/v1/subscriptions"], "POST")
	assert.Contains(t, routePaths["/o2dms/v1/subscriptions/:subscriptionId"], "GET")
	assert.Contains(t, routePaths["/o2dms/v1/subscriptions/:subscriptionId"], "DELETE")
}

func TestDMSRoutesIntegration(t *testing.T) {
	srv := setupTestServer(t)
	logger := zap.NewNop()

	// Create a registry with a mock adapter.
	reg := dmsregistry.NewRegistry(logger, nil)
	mockAdp := newMockDMSAdapter("test-adapter")
	err := reg.Register(context.Background(), "test-adapter", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	// Set up DMS.
	srv.SetupDMS(reg)

	// Test that we can hit each endpoint type.
	testCases := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "DMS API info",
			method:         http.MethodGet,
			path:           "/o2dms",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Deployment lifecycle info",
			method:         http.MethodGet,
			path:           "/o2dms/v1/deploymentLifecycle",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "List NF deployments",
			method:         http.MethodGet,
			path:           "/o2dms/v1/nfDeployments",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "List NF deployment descriptors",
			method:         http.MethodGet,
			path:           "/o2dms/v1/nfDeploymentDescriptors",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "List DMS subscriptions",
			method:         http.MethodGet,
			path:           "/o2dms/v1/subscriptions",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			srv.router.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code, "Unexpected status for %s %s", tc.method, tc.path)
		})
	}
}

func TestDMSRegistry(t *testing.T) {
	srv := setupTestServer(t)
	logger := zap.NewNop()

	// Initially should be nil.
	assert.Nil(t, srv.DMSRegistry())

	// Create and set up DMS.
	reg := dmsregistry.NewRegistry(logger, nil)
	srv.SetupDMS(reg)

	// Now should return the registry.
	assert.NotNil(t, srv.DMSRegistry())
	assert.Equal(t, reg, srv.DMSRegistry())
}
