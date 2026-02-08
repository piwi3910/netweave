package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	imsadapter "github.com/piwi3910/netweave/internal/adapter"
	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
	"github.com/piwi3910/netweave/internal/dms/registry"
	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/storage"
	"go.uber.org/zap"
)

// ========================================
// Mock Implementations for TMForum Tests
// ========================================

// mockIMSAdapter implements imsadapter.Adapter for testing.
type mockIMSAdapter struct {
	pools         []*imsadapter.ResourcePool
	resources     []*imsadapter.Resource
	subscriptions []*imsadapter.Subscription

	listPoolsErr  error
	getPoolErr    error
	createPoolErr error
	updatePoolErr error
	deletePoolErr error
	listResErr    error
	getResErr     error
	createResErr  error
	deleteResErr  error
	createSubErr  error
	deleteSubErr  error
	createdSub    *imsadapter.Subscription
	createdPool   *imsadapter.ResourcePool
	createdRes    *imsadapter.Resource
	updatedPool   *imsadapter.ResourcePool
	poolForUpdate *imsadapter.ResourcePool // Pool returned by GetResourcePool for update flow
	resForGet     *imsadapter.Resource     // Resource returned by GetResource
}

func (m *mockIMSAdapter) Name() string    { return "mock-ims" }
func (m *mockIMSAdapter) Version() string { return "1.0.0" }
func (m *mockIMSAdapter) Capabilities() []imsadapter.Capability {
	return []imsadapter.Capability{imsadapter.CapabilityResourcePools}
}
func (m *mockIMSAdapter) Health(_ context.Context) error { return nil }
func (m *mockIMSAdapter) Close() error                   { return nil }

func (m *mockIMSAdapter) ListResourcePools(_ context.Context, _ *imsadapter.Filter) ([]*imsadapter.ResourcePool, error) {
	if m.listPoolsErr != nil {
		return nil, m.listPoolsErr
	}
	return m.pools, nil
}

func (m *mockIMSAdapter) GetResourcePool(_ context.Context, id string) (*imsadapter.ResourcePool, error) {
	if m.getPoolErr != nil {
		return nil, m.getPoolErr
	}
	if m.poolForUpdate != nil {
		return m.poolForUpdate, nil
	}
	for _, p := range m.pools {
		if p.ResourcePoolID == id {
			return p, nil
		}
	}
	return nil, imsadapter.ErrResourcePoolNotFound
}

func (m *mockIMSAdapter) CreateResourcePool(_ context.Context, pool *imsadapter.ResourcePool) (*imsadapter.ResourcePool, error) {
	if m.createPoolErr != nil {
		return nil, m.createPoolErr
	}
	if m.createdPool != nil {
		return m.createdPool, nil
	}
	pool.ResourcePoolID = "new-pool-1"
	return pool, nil
}

func (m *mockIMSAdapter) UpdateResourcePool(_ context.Context, _ string, pool *imsadapter.ResourcePool) (*imsadapter.ResourcePool, error) {
	if m.updatePoolErr != nil {
		return nil, m.updatePoolErr
	}
	if m.updatedPool != nil {
		return m.updatedPool, nil
	}
	return pool, nil
}

func (m *mockIMSAdapter) DeleteResourcePool(_ context.Context, id string) error {
	if m.deletePoolErr != nil {
		return m.deletePoolErr
	}
	for _, p := range m.pools {
		if p.ResourcePoolID == id {
			return nil
		}
	}
	return imsadapter.ErrResourcePoolNotFound
}

func (m *mockIMSAdapter) ListResources(_ context.Context, _ *imsadapter.Filter) ([]*imsadapter.Resource, error) {
	if m.listResErr != nil {
		return nil, m.listResErr
	}
	return m.resources, nil
}

func (m *mockIMSAdapter) GetResource(_ context.Context, id string) (*imsadapter.Resource, error) {
	if m.getResErr != nil {
		return nil, m.getResErr
	}
	if m.resForGet != nil {
		return m.resForGet, nil
	}
	for _, r := range m.resources {
		if r.ResourceID == id {
			return r, nil
		}
	}
	return nil, imsadapter.ErrResourceNotFound
}

func (m *mockIMSAdapter) CreateResource(_ context.Context, res *imsadapter.Resource) (*imsadapter.Resource, error) {
	if m.createResErr != nil {
		return nil, m.createResErr
	}
	if m.createdRes != nil {
		return m.createdRes, nil
	}
	return res, nil
}

func (m *mockIMSAdapter) UpdateResource(_ context.Context, _ string, res *imsadapter.Resource) (*imsadapter.Resource, error) {
	return res, nil
}

func (m *mockIMSAdapter) DeleteResource(_ context.Context, id string) error {
	if m.deleteResErr != nil {
		return m.deleteResErr
	}
	for _, r := range m.resources {
		if r.ResourceID == id {
			return nil
		}
	}
	return imsadapter.ErrResourceNotFound
}

func (m *mockIMSAdapter) ListResourceTypes(_ context.Context, _ *imsadapter.Filter) ([]*imsadapter.ResourceType, error) {
	return nil, nil
}

func (m *mockIMSAdapter) GetResourceType(_ context.Context, _ string) (*imsadapter.ResourceType, error) {
	return nil, imsadapter.ErrResourceTypeNotFound
}

func (m *mockIMSAdapter) ListDeploymentManagers(_ context.Context, _ *imsadapter.Filter) ([]*imsadapter.DeploymentManager, error) {
	return nil, nil
}

func (m *mockIMSAdapter) GetDeploymentManager(_ context.Context, _ string) (*imsadapter.DeploymentManager, error) {
	return nil, imsadapter.ErrDeploymentManagerNotFound
}

func (m *mockIMSAdapter) ListSubscriptions(_ context.Context, _ *imsadapter.Filter) ([]*imsadapter.Subscription, error) {
	return m.subscriptions, nil
}

func (m *mockIMSAdapter) GetSubscription(_ context.Context, _ string) (*imsadapter.Subscription, error) {
	return nil, imsadapter.ErrSubscriptionNotFound
}

func (m *mockIMSAdapter) CreateSubscription(_ context.Context, sub *imsadapter.Subscription) (*imsadapter.Subscription, error) {
	if m.createSubErr != nil {
		return nil, m.createSubErr
	}
	if m.createdSub != nil {
		return m.createdSub, nil
	}
	sub.SubscriptionID = "sub-new-1"
	return sub, nil
}

func (m *mockIMSAdapter) UpdateSubscription(_ context.Context, _ string, sub *imsadapter.Subscription) (*imsadapter.Subscription, error) {
	return sub, nil
}

func (m *mockIMSAdapter) DeleteSubscription(_ context.Context, _ string) error {
	if m.deleteSubErr != nil {
		return m.deleteSubErr
	}
	return nil
}

// mockDMSAdapter implements dmsadapter.DMSAdapter for testing.
type mockDMSAdapter struct {
	name        string
	deployments []*dmsadapter.Deployment
	packages    []*dmsadapter.DeploymentPackage

	listDeploymentsErr  error
	getDeploymentErr    error
	createDeploymentErr error
	deleteDeploymentErr error
	listPackagesErr     error
	getPackageErr       error
	createdDeployment   *dmsadapter.Deployment
}

func (m *mockDMSAdapter) Name() string    { return m.name }
func (m *mockDMSAdapter) Version() string { return "1.0.0" }
func (m *mockDMSAdapter) Capabilities() []dmsadapter.Capability {
	return []dmsadapter.Capability{dmsadapter.CapabilityDeploymentLifecycle}
}
func (m *mockDMSAdapter) Health(_ context.Context) error { return nil }
func (m *mockDMSAdapter) Close() error                   { return nil }

func (m *mockDMSAdapter) ListDeployments(_ context.Context, _ *dmsadapter.Filter) ([]*dmsadapter.Deployment, error) {
	if m.listDeploymentsErr != nil {
		return nil, m.listDeploymentsErr
	}
	return m.deployments, nil
}

func (m *mockDMSAdapter) GetDeployment(_ context.Context, id string) (*dmsadapter.Deployment, error) {
	if m.getDeploymentErr != nil {
		return nil, m.getDeploymentErr
	}
	for _, d := range m.deployments {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, dmsadapter.ErrDeploymentNotFound
}

func (m *mockDMSAdapter) CreateDeployment(_ context.Context, req *dmsadapter.DeploymentRequest) (*dmsadapter.Deployment, error) {
	if m.createDeploymentErr != nil {
		return nil, m.createDeploymentErr
	}
	if m.createdDeployment != nil {
		return m.createdDeployment, nil
	}
	return &dmsadapter.Deployment{
		ID:        "dep-new-1",
		Name:      req.Name,
		PackageID: req.PackageID,
		Namespace: req.Namespace,
		Status:    dmsadapter.DeploymentStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (m *mockDMSAdapter) UpdateDeployment(_ context.Context, _ string, _ *dmsadapter.DeploymentUpdate) (*dmsadapter.Deployment, error) {
	return nil, nil
}

func (m *mockDMSAdapter) DeleteDeployment(_ context.Context, id string) error {
	if m.deleteDeploymentErr != nil {
		return m.deleteDeploymentErr
	}
	for _, d := range m.deployments {
		if d.ID == id {
			return nil
		}
	}
	return dmsadapter.ErrDeploymentNotFound
}

func (m *mockDMSAdapter) ScaleDeployment(_ context.Context, _ string, _ int) error {
	return dmsadapter.ErrOperationNotSupported
}

func (m *mockDMSAdapter) RollbackDeployment(_ context.Context, _ string, _ int) error {
	return dmsadapter.ErrOperationNotSupported
}

func (m *mockDMSAdapter) GetDeploymentStatus(_ context.Context, _ string) (*dmsadapter.DeploymentStatusDetail, error) {
	return nil, nil
}

func (m *mockDMSAdapter) GetDeploymentHistory(_ context.Context, _ string) (*dmsadapter.DeploymentHistory, error) {
	return nil, nil
}

func (m *mockDMSAdapter) GetDeploymentLogs(_ context.Context, _ string, _ *dmsadapter.LogOptions) ([]byte, error) {
	return nil, nil
}

func (m *mockDMSAdapter) SupportsRollback() bool { return false }
func (m *mockDMSAdapter) SupportsScaling() bool  { return false }
func (m *mockDMSAdapter) SupportsGitOps() bool   { return false }

func (m *mockDMSAdapter) ListDeploymentPackages(_ context.Context, _ *dmsadapter.Filter) ([]*dmsadapter.DeploymentPackage, error) {
	if m.listPackagesErr != nil {
		return nil, m.listPackagesErr
	}
	return m.packages, nil
}

func (m *mockDMSAdapter) GetDeploymentPackage(_ context.Context, id string) (*dmsadapter.DeploymentPackage, error) {
	if m.getPackageErr != nil {
		return nil, m.getPackageErr
	}
	for _, p := range m.packages {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, dmsadapter.ErrPackageNotFound
}

func (m *mockDMSAdapter) UploadDeploymentPackage(_ context.Context, _ *dmsadapter.DeploymentPackageUpload) (*dmsadapter.DeploymentPackage, error) {
	return nil, nil
}

func (m *mockDMSAdapter) DeleteDeploymentPackage(_ context.Context, _ string) error {
	return nil
}

// ========================================
// Helper Functions
// ========================================

func setupTMForumHandler(t *testing.T) (*TMForumHandler, *mockIMSAdapter, *mockDMSAdapter) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	imsAdp := &mockIMSAdapter{}
	dmsAdp := &mockDMSAdapter{
		name: "test-dms",
	}

	// Create registry and register the mock DMS adapter
	reg := registry.NewRegistry(logger, nil)
	err = reg.Register(context.Background(), "test-dms", "helm", dmsAdp, nil, true)
	require.NoError(t, err)

	hubStore := storage.NewInMemoryHubStore()

	handler := NewTMForumHandler(imsAdp, reg, hubStore, logger)

	return handler, imsAdp, dmsAdp
}

func performRequest(handler gin.HandlerFunc, method, path string, body interface{}) *httptest.ResponseRecorder {
	router := gin.New()
	router.Handle(method, path, handler)

	var bodyReader *bytes.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(w, req)
	return w
}

func performRequestWithParam(handler gin.HandlerFunc, method, path, paramKey, paramValue string, body interface{}) *httptest.ResponseRecorder {
	router := gin.New()
	// Build route pattern with param placeholder
	routePath := path
	// Replace the actual value with a param placeholder in the route pattern
	if paramValue != "" {
		lastSlash := len(routePath) - 1
		for lastSlash >= 0 && routePath[lastSlash] != '/' {
			lastSlash--
		}
		routePath = routePath[:lastSlash+1] + ":" + paramKey
	}
	router.Handle(method, routePath, handler)

	var bodyReader *bytes.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(w, req)
	return w
}

func performRequestWithQuery(handler gin.HandlerFunc, method, path string, query map[string]string) *httptest.ResponseRecorder {
	router := gin.New()
	router.Handle(method, path, handler)

	fullPath := path
	if len(query) > 0 {
		fullPath += "?"
		first := true
		for k, v := range query {
			if !first {
				fullPath += "&"
			}
			fullPath += k + "=" + v
			first = false
		}
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, fullPath, nil)
	router.ServeHTTP(w, req)
	return w
}

// ========================================
// TMF639 - Resource Inventory Management Tests
// ========================================

func TestNewTMForumHandler(t *testing.T) {
	handler, _, _ := setupTMForumHandler(t)
	assert.NotNil(t, handler)
}

func TestListTMF639Resources(t *testing.T) {
	tests := []struct {
		name           string
		setupAdapter   func(*mockIMSAdapter)
		query          map[string]string
		expectedStatus int
		expectEmpty    bool
	}{
		{
			name: "list all resources - pools and resources",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.pools = []*imsadapter.ResourcePool{
					{ResourcePoolID: "pool-1", Name: "Pool 1", Extensions: map[string]interface{}{}},
				}
				adp.resources = []*imsadapter.Resource{
					{ResourceID: "res-1", ResourceTypeID: "rt-1", Extensions: map[string]interface{}{}},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "filter by resourcePool category",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.pools = []*imsadapter.ResourcePool{
					{ResourcePoolID: "pool-1", Name: "Pool 1", Extensions: map[string]interface{}{}},
				}
			},
			query:          map[string]string{"resourceSpecification.category": "categoryResourcePool"},
			expectedStatus: http.StatusOK,
		},
		{
			name: "filter by non-pool category",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.resources = []*imsadapter.Resource{
					{ResourceID: "res-1", ResourceTypeID: "compute", Extensions: map[string]interface{}{}},
				}
			},
			query:          map[string]string{"resourceSpecification.category": "compute"},
			expectedStatus: http.StatusOK,
		},
		{
			name: "list pools error",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.listPoolsErr = errors.New("pool list error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "list resources error",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.listResErr = errors.New("resource list error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "empty result",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.pools = []*imsadapter.ResourcePool{}
				adp.resources = []*imsadapter.Resource{}
			},
			expectedStatus: http.StatusOK,
			expectEmpty:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, imsAdp, _ := setupTMForumHandler(t)
			tt.setupAdapter(imsAdp)

			var w *httptest.ResponseRecorder
			if tt.query != nil {
				w = performRequestWithQuery(handler.ListTMF639Resources, http.MethodGet, "/tmf-api/resourceInventoryManagement/v4/resource", tt.query)
			} else {
				w = performRequest(handler.ListTMF639Resources, http.MethodGet, "/tmf-api/resourceInventoryManagement/v4/resource", nil)
			}

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestGetTMF639Resource(t *testing.T) {
	tests := []struct {
		name           string
		resourceID     string
		setupAdapter   func(*mockIMSAdapter)
		expectedStatus int
	}{
		{
			name:       "get resource pool by ID",
			resourceID: "pool-1",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.pools = []*imsadapter.ResourcePool{
					{ResourcePoolID: "pool-1", Name: "Pool 1", Extensions: map[string]interface{}{}},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:       "get individual resource by ID",
			resourceID: "res-1",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.getPoolErr = imsadapter.ErrResourcePoolNotFound
				adp.resources = []*imsadapter.Resource{
					{ResourceID: "res-1", ResourceTypeID: "rt-1", Extensions: map[string]interface{}{}},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:       "resource not found",
			resourceID: "non-existent",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.getPoolErr = imsadapter.ErrResourcePoolNotFound
				adp.getResErr = imsadapter.ErrResourceNotFound
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:       "resource internal error",
			resourceID: "err-res",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.getPoolErr = imsadapter.ErrResourcePoolNotFound
				adp.getResErr = errors.New("internal error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, imsAdp, _ := setupTMForumHandler(t)
			tt.setupAdapter(imsAdp)

			w := performRequestWithParam(handler.GetTMF639Resource, http.MethodGet,
				"/tmf-api/resourceInventoryManagement/v4/resource/"+tt.resourceID,
				"id", tt.resourceID, nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestCreateTMF639Resource(t *testing.T) {
	tests := []struct {
		name           string
		body           interface{}
		setupAdapter   func(*mockIMSAdapter)
		expectedStatus int
	}{
		{
			name: "create resource pool (empty category)",
			body: models.TMF639ResourceCreate{
				Name:     "New Pool",
				Category: "",
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.createdPool = &imsadapter.ResourcePool{
					ResourcePoolID: "new-pool-1",
					Name:           "New Pool",
					Extensions:     map[string]interface{}{},
				}
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "create resource pool (categoryResourcePool)",
			body: models.TMF639ResourceCreate{
				Name:     "New Pool",
				Category: "categoryResourcePool",
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.createdPool = &imsadapter.ResourcePool{
					ResourcePoolID: "new-pool-1",
					Name:           "New Pool",
					Extensions:     map[string]interface{}{},
				}
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "create individual resource",
			body: models.TMF639ResourceCreate{
				Name:     "New Resource",
				Category: "compute",
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.createdRes = &imsadapter.Resource{
					ResourceID:     "new-res-1",
					ResourceTypeID: "compute",
					Extensions:     map[string]interface{}{},
				}
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "create pool error",
			body: models.TMF639ResourceCreate{
				Name:     "Fail Pool",
				Category: "categoryResourcePool",
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.createPoolErr = errors.New("create pool failed")
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "create resource error",
			body: models.TMF639ResourceCreate{
				Name:     "Fail Resource",
				Category: "compute",
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.createResErr = errors.New("create resource failed")
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "invalid JSON body",
			body:           "invalid",
			setupAdapter:   func(adp *mockIMSAdapter) {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, imsAdp, _ := setupTMForumHandler(t)
			tt.setupAdapter(imsAdp)

			w := performRequest(handler.CreateTMF639Resource, http.MethodPost,
				"/tmf-api/resourceInventoryManagement/v4/resource", tt.body)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestUpdateTMF639Resource(t *testing.T) {
	newName := "Updated Pool"
	newDesc := "Updated Description"

	tests := []struct {
		name           string
		resourceID     string
		body           interface{}
		setupAdapter   func(*mockIMSAdapter)
		expectedStatus int
	}{
		{
			name:       "update resource pool successfully",
			resourceID: "pool-1",
			body: models.TMF639ResourceUpdate{
				Name:        &newName,
				Description: &newDesc,
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.poolForUpdate = &imsadapter.ResourcePool{
					ResourcePoolID: "pool-1",
					Name:           "Original",
					Extensions:     map[string]interface{}{},
				}
				adp.updatedPool = &imsadapter.ResourcePool{
					ResourcePoolID: "pool-1",
					Name:           "Updated Pool",
					Description:    "Updated Description",
					Extensions:     map[string]interface{}{},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:       "update pool - update fails",
			resourceID: "pool-1",
			body: models.TMF639ResourceUpdate{
				Name: &newName,
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.poolForUpdate = &imsadapter.ResourcePool{
					ResourcePoolID: "pool-1",
					Name:           "Original",
					Extensions:     map[string]interface{}{},
				}
				adp.updatePoolErr = errors.New("update error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:       "resource not found for update",
			resourceID: "non-existent",
			body: models.TMF639ResourceUpdate{
				Name: &newName,
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.getPoolErr = imsadapter.ErrResourcePoolNotFound
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:       "invalid JSON body",
			resourceID: "pool-1",
			body:       "invalid",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.poolForUpdate = &imsadapter.ResourcePool{
					ResourcePoolID: "pool-1",
					Name:           "Original",
					Extensions:     map[string]interface{}{},
				}
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, imsAdp, _ := setupTMForumHandler(t)
			tt.setupAdapter(imsAdp)

			w := performRequestWithParam(handler.UpdateTMF639Resource, http.MethodPatch,
				"/tmf-api/resourceInventoryManagement/v4/resource/"+tt.resourceID,
				"id", tt.resourceID, tt.body)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestDeleteTMF639Resource(t *testing.T) {
	tests := []struct {
		name           string
		resourceID     string
		setupAdapter   func(*mockIMSAdapter)
		expectedStatus int
	}{
		{
			name:       "delete resource pool",
			resourceID: "pool-1",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.pools = []*imsadapter.ResourcePool{
					{ResourcePoolID: "pool-1"},
				}
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:       "delete individual resource (pool not found)",
			resourceID: "res-1",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.deletePoolErr = imsadapter.ErrResourcePoolNotFound
				adp.resources = []*imsadapter.Resource{
					{ResourceID: "res-1"},
				}
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:       "resource not found",
			resourceID: "non-existent",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.deletePoolErr = imsadapter.ErrResourcePoolNotFound
				adp.deleteResErr = imsadapter.ErrResourceNotFound
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:       "delete resource internal error",
			resourceID: "err-res",
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.deletePoolErr = imsadapter.ErrResourcePoolNotFound
				adp.deleteResErr = errors.New("internal error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, imsAdp, _ := setupTMForumHandler(t)
			tt.setupAdapter(imsAdp)

			w := performRequestWithParam(handler.DeleteTMF639Resource, http.MethodDelete,
				"/tmf-api/resourceInventoryManagement/v4/resource/"+tt.resourceID,
				"id", tt.resourceID, nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// ========================================
// TMF638 - Service Inventory Management Tests
// ========================================

func TestListTMF638Services(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name: "list services from deployments",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{
						ID:        "dep-1",
						Name:      "Service 1",
						Status:    dmsadapter.DeploymentStatusDeployed,
						CreatedAt: now,
					},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "list with DMS error (continues)",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.listDeploymentsErr = errors.New("dms error")
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "empty deployments",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{}
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequest(handler.ListTMF638Services, http.MethodGet,
				"/tmf-api/serviceInventoryManagement/v4/service", nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestGetTMF638Service(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		serviceID      string
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name:      "get service found",
			serviceID: "dep-1",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{
						ID:        "dep-1",
						Name:      "Service 1",
						Status:    dmsadapter.DeploymentStatusDeployed,
						CreatedAt: now,
					},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "service not found",
			serviceID: "non-existent",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{}
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequestWithParam(handler.GetTMF638Service, http.MethodGet,
				"/tmf-api/serviceInventoryManagement/v4/service/"+tt.serviceID,
				"id", tt.serviceID, nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestCreateTMF638Service(t *testing.T) {
	tests := []struct {
		name           string
		body           interface{}
		setupDMS       func(*mockDMSAdapter)
		noDefault      bool
		expectedStatus int
	}{
		{
			name: "create service successfully",
			body: models.TMF638ServiceCreate{
				Name:        "New Service",
				Description: "Test service",
				ServiceSpecification: &models.ServiceSpecificationRef{
					ID: "pkg-1",
				},
			},
			setupDMS:       func(dms *mockDMSAdapter) {},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "create deployment error",
			body: models.TMF638ServiceCreate{
				Name: "Fail Service",
			},
			setupDMS: func(dms *mockDMSAdapter) {
				dms.createDeploymentErr = errors.New("deployment creation failed")
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "invalid JSON body",
			body:           "invalid",
			setupDMS:       func(dms *mockDMSAdapter) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "no default DMS adapter",
			body: models.TMF638ServiceCreate{
				Name: "No Adapter Service",
			},
			noDefault:      true,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.noDefault {
				// Create handler with empty registry (no default)
				gin.SetMode(gin.TestMode)
				logger, err := zap.NewDevelopment()
				require.NoError(t, err)
				imsAdp := &mockIMSAdapter{}
				reg := registry.NewRegistry(logger, nil)
				hubStore := storage.NewInMemoryHubStore()
				handler := NewTMForumHandler(imsAdp, reg, hubStore, logger)

				w := performRequest(handler.CreateTMF638Service, http.MethodPost,
					"/tmf-api/serviceInventoryManagement/v4/service", tt.body)
				assert.Equal(t, tt.expectedStatus, w.Code)
				return
			}

			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequest(handler.CreateTMF638Service, http.MethodPost,
				"/tmf-api/serviceInventoryManagement/v4/service", tt.body)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestUpdateTMF638Service(t *testing.T) {
	now := time.Now()
	newName := "Updated Service"

	tests := []struct {
		name           string
		serviceID      string
		body           interface{}
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name:      "update service found",
			serviceID: "dep-1",
			body: models.TMF638ServiceUpdate{
				Name: &newName,
			},
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{
						ID:        "dep-1",
						Name:      "Original Service",
						Status:    dmsadapter.DeploymentStatusDeployed,
						CreatedAt: now,
					},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "service not found",
			serviceID: "non-existent",
			body: models.TMF638ServiceUpdate{
				Name: &newName,
			},
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{}
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:      "invalid body",
			serviceID: "dep-1",
			body:      "invalid",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{ID: "dep-1", CreatedAt: now},
				}
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequestWithParam(handler.UpdateTMF638Service, http.MethodPatch,
				"/tmf-api/serviceInventoryManagement/v4/service/"+tt.serviceID,
				"id", tt.serviceID, tt.body)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestDeleteTMF638Service(t *testing.T) {
	tests := []struct {
		name           string
		serviceID      string
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name:      "delete service found",
			serviceID: "dep-1",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{ID: "dep-1"},
				}
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:      "service not found",
			serviceID: "non-existent",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{}
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequestWithParam(handler.DeleteTMF638Service, http.MethodDelete,
				"/tmf-api/serviceInventoryManagement/v4/service/"+tt.serviceID,
				"id", tt.serviceID, nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// ========================================
// TMF641 - Service Ordering Management Tests
// ========================================

func TestListTMF641ServiceOrders(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		setupDMS       func(*mockDMSAdapter)
		query          map[string]string
		expectedStatus int
	}{
		{
			name: "list all orders",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{
						ID:         "dep-1",
						Name:       "Order 1",
						Status:     dmsadapter.DeploymentStatusDeployed,
						CreatedAt:  now,
						Extensions: map[string]interface{}{"orderExternalId": "ext-1"},
					},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "filter by state",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{
						ID:         "dep-1",
						Status:     dmsadapter.DeploymentStatusDeployed,
						CreatedAt:  now,
						Extensions: map[string]interface{}{},
					},
					{
						ID:         "dep-2",
						Status:     dmsadapter.DeploymentStatusPending,
						CreatedAt:  now,
						Extensions: map[string]interface{}{},
					},
				}
			},
			query:          map[string]string{"state": "completed"},
			expectedStatus: http.StatusOK,
		},
		{
			name: "filter by externalId",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{
						ID:         "dep-1",
						Status:     dmsadapter.DeploymentStatusDeployed,
						CreatedAt:  now,
						Extensions: map[string]interface{}{"orderExternalId": "ext-1"},
					},
				}
			},
			query:          map[string]string{"externalId": "ext-1"},
			expectedStatus: http.StatusOK,
		},
		{
			name: "DMS error continues",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.listDeploymentsErr = errors.New("dms error")
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			var w *httptest.ResponseRecorder
			if tt.query != nil {
				w = performRequestWithQuery(handler.ListTMF641ServiceOrders, http.MethodGet,
					"/tmf-api/serviceOrdering/v4/serviceOrder", tt.query)
			} else {
				w = performRequest(handler.ListTMF641ServiceOrders, http.MethodGet,
					"/tmf-api/serviceOrdering/v4/serviceOrder", nil)
			}

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestGetTMF641ServiceOrder(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		orderID        string
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name:    "get order found",
			orderID: "dep-1",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{ID: "dep-1", Status: dmsadapter.DeploymentStatusDeployed, CreatedAt: now, Extensions: map[string]interface{}{}},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "order not found",
			orderID: "non-existent",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{}
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequestWithParam(handler.GetTMF641ServiceOrder, http.MethodGet,
				"/tmf-api/serviceOrdering/v4/serviceOrder/"+tt.orderID,
				"id", tt.orderID, nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestCreateTMF641ServiceOrder(t *testing.T) {
	tests := []struct {
		name           string
		body           interface{}
		setupDMS       func(*mockDMSAdapter)
		noDefault      bool
		expectedStatus int
	}{
		{
			name: "create order successfully",
			body: models.TMF641ServiceOrderCreate{
				ExternalID:  "ext-1",
				Description: "Test order",
				ServiceOrderItem: []models.ServiceOrderItemCreate{
					{
						Action: "add",
						Service: &models.TMF638ServiceCreate{
							Name: "Order Service",
							ServiceSpecification: &models.ServiceSpecificationRef{
								ID: "pkg-1",
							},
						},
					},
				},
			},
			setupDMS:       func(dms *mockDMSAdapter) {},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "create order - empty items",
			body: models.TMF641ServiceOrderCreate{
				ServiceOrderItem: []models.ServiceOrderItemCreate{},
			},
			setupDMS:       func(dms *mockDMSAdapter) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "create order - no default adapter",
			body: models.TMF641ServiceOrderCreate{
				ServiceOrderItem: []models.ServiceOrderItemCreate{
					{Action: "add", Service: &models.TMF638ServiceCreate{Name: "Svc"}},
				},
			},
			noDefault:      true,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "create order - deployment error",
			body: models.TMF641ServiceOrderCreate{
				ServiceOrderItem: []models.ServiceOrderItemCreate{
					{Action: "add", Service: &models.TMF638ServiceCreate{Name: "Svc"}},
				},
			},
			setupDMS: func(dms *mockDMSAdapter) {
				dms.createDeploymentErr = errors.New("deployment error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "invalid body",
			body:           "invalid",
			setupDMS:       func(dms *mockDMSAdapter) {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.noDefault {
				gin.SetMode(gin.TestMode)
				logger, err := zap.NewDevelopment()
				require.NoError(t, err)
				imsAdp := &mockIMSAdapter{}
				reg := registry.NewRegistry(logger, nil)
				hubStore := storage.NewInMemoryHubStore()
				handler := NewTMForumHandler(imsAdp, reg, hubStore, logger)

				w := performRequest(handler.CreateTMF641ServiceOrder, http.MethodPost,
					"/tmf-api/serviceOrdering/v4/serviceOrder", tt.body)
				assert.Equal(t, tt.expectedStatus, w.Code)
				return
			}

			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequest(handler.CreateTMF641ServiceOrder, http.MethodPost,
				"/tmf-api/serviceOrdering/v4/serviceOrder", tt.body)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestUpdateTMF641ServiceOrder(t *testing.T) {
	now := time.Now()
	newDesc := "Updated order"

	tests := []struct {
		name           string
		orderID        string
		body           interface{}
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name:    "update order found",
			orderID: "dep-1",
			body: models.TMF641ServiceOrderUpdate{
				Description: &newDesc,
			},
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{ID: "dep-1", Status: dmsadapter.DeploymentStatusPending, CreatedAt: now, Extensions: map[string]interface{}{}},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "order not found",
			orderID: "non-existent",
			body: models.TMF641ServiceOrderUpdate{
				Description: &newDesc,
			},
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{}
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:    "invalid body",
			orderID: "dep-1",
			body:    "invalid",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{ID: "dep-1", CreatedAt: now, Extensions: map[string]interface{}{}},
				}
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequestWithParam(handler.UpdateTMF641ServiceOrder, http.MethodPatch,
				"/tmf-api/serviceOrdering/v4/serviceOrder/"+tt.orderID,
				"id", tt.orderID, tt.body)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestDeleteTMF641ServiceOrder(t *testing.T) {
	tests := []struct {
		name           string
		orderID        string
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name:    "delete order found",
			orderID: "dep-1",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{{ID: "dep-1"}}
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:    "order not found",
			orderID: "non-existent",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{}
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequestWithParam(handler.DeleteTMF641ServiceOrder, http.MethodDelete,
				"/tmf-api/serviceOrdering/v4/serviceOrder/"+tt.orderID,
				"id", tt.orderID, nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// ========================================
// TMF688 - Event Management Tests
// ========================================

func TestListTMF688Events(t *testing.T) {
	handler, _, _ := setupTMForumHandler(t)

	w := performRequest(handler.ListTMF688Events, http.MethodGet,
		"/tmf-api/eventManagement/v4/event", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var events []models.TMF688Event
	err := json.Unmarshal(w.Body.Bytes(), &events)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestGetTMF688Event(t *testing.T) {
	handler, _, _ := setupTMForumHandler(t)

	w := performRequestWithParam(handler.GetTMF688Event, http.MethodGet,
		"/tmf-api/eventManagement/v4/event/evt-1",
		"id", "evt-1", nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateTMF688Event(t *testing.T) {
	tests := []struct {
		name           string
		body           interface{}
		expectedStatus int
	}{
		{
			name: "create event returns not implemented",
			body: models.TMF688EventCreate{
				EventType: "ResourceCreationNotification",
				EventTime: func() *time.Time { t := time.Now(); return &t }(),
				Event:     &models.EventPayload{},
			},
			expectedStatus: http.StatusNotImplemented,
		},
		{
			name:           "invalid body",
			body:           "invalid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, _ := setupTMForumHandler(t)

			w := performRequest(handler.CreateTMF688Event, http.MethodPost,
				"/tmf-api/eventManagement/v4/event", tt.body)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestRegisterTMF688Hub(t *testing.T) {
	tests := []struct {
		name           string
		body           interface{}
		setupAdapter   func(*mockIMSAdapter)
		expectedStatus int
	}{
		{
			name: "register hub successfully",
			body: models.TMF688HubCreate{
				Callback: "https://example.com/notify",
				Query:    "resourcePoolId=pool-1",
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.createdSub = &imsadapter.Subscription{
					SubscriptionID: "sub-1",
					Callback:       "https://example.com/notify",
				}
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "register hub with empty query",
			body: models.TMF688HubCreate{
				Callback: "https://example.com/notify",
				Query:    "",
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.createdSub = &imsadapter.Subscription{
					SubscriptionID: "sub-2",
					Callback:       "https://example.com/notify",
				}
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "register hub - invalid query",
			body: models.TMF688HubCreate{
				Callback: "https://example.com/notify",
				Query:    "eventType=InvalidEventType",
			},
			setupAdapter:   func(adp *mockIMSAdapter) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "register hub - subscription creation fails",
			body: models.TMF688HubCreate{
				Callback: "https://example.com/notify",
				Query:    "",
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.createSubErr = errors.New("subscription creation failed")
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "invalid body",
			body:           "invalid",
			setupAdapter:   func(adp *mockIMSAdapter) {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, imsAdp, _ := setupTMForumHandler(t)
			tt.setupAdapter(imsAdp)

			w := performRequest(handler.RegisterTMF688Hub, http.MethodPost,
				"/tmf-api/eventManagement/v4/hub", tt.body)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestRegisterTMF688Hub_HubStoreError(t *testing.T) {
	// Test when hub store fails to create (hub already exists)
	gin.SetMode(gin.TestMode)
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	imsAdp := &mockIMSAdapter{
		createdSub: &imsadapter.Subscription{
			SubscriptionID: "sub-existing",
			Callback:       "https://example.com/notify",
		},
	}
	reg := registry.NewRegistry(logger, nil)
	hubStore := storage.NewInMemoryHubStore()

	handler := NewTMForumHandler(imsAdp, reg, hubStore, logger)

	// Pre-populate hub store with a registration that will have the same ID
	err = hubStore.Create(context.Background(), &storage.HubRegistration{
		HubID:          "hub-sub-existing",
		Callback:       "https://example.com/notify",
		SubscriptionID: "sub-existing",
		CreatedAt:      time.Now(),
	})
	require.NoError(t, err)

	body := models.TMF688HubCreate{
		Callback: "https://example.com/notify",
		Query:    "",
	}

	w := performRequest(handler.RegisterTMF688Hub, http.MethodPost,
		"/tmf-api/eventManagement/v4/hub", body)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUnregisterTMF688Hub(t *testing.T) {
	tests := []struct {
		name           string
		hubID          string
		setupStore     func(storage.HubStore)
		setupAdapter   func(*mockIMSAdapter)
		expectedStatus int
	}{
		{
			name:  "unregister hub successfully",
			hubID: "hub-1",
			setupStore: func(hs storage.HubStore) {
				err := hs.Create(context.Background(), &storage.HubRegistration{
					HubID:          "hub-1",
					Callback:       "https://example.com/notify",
					SubscriptionID: "sub-1",
					CreatedAt:      time.Now(),
				})
				if err != nil {
					panic(err)
				}
			},
			setupAdapter:   func(adp *mockIMSAdapter) {},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "hub not found",
			hubID:          "non-existent",
			setupStore:     func(hs storage.HubStore) {},
			setupAdapter:   func(adp *mockIMSAdapter) {},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:  "subscription deletion fails (continues)",
			hubID: "hub-1",
			setupStore: func(hs storage.HubStore) {
				err := hs.Create(context.Background(), &storage.HubRegistration{
					HubID:          "hub-1",
					Callback:       "https://example.com/notify",
					SubscriptionID: "sub-1",
					CreatedAt:      time.Now(),
				})
				if err != nil {
					panic(err)
				}
			},
			setupAdapter: func(adp *mockIMSAdapter) {
				adp.deleteSubErr = errors.New("delete sub failed")
			},
			expectedStatus: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			logger, err := zap.NewDevelopment()
			require.NoError(t, err)

			imsAdp := &mockIMSAdapter{}
			tt.setupAdapter(imsAdp)

			reg := registry.NewRegistry(logger, nil)
			hubStore := storage.NewInMemoryHubStore()
			tt.setupStore(hubStore)

			handler := NewTMForumHandler(imsAdp, reg, hubStore, logger)

			w := performRequestWithParam(handler.UnregisterTMF688Hub, http.MethodDelete,
				"/tmf-api/eventManagement/v4/hub/"+tt.hubID,
				"id", tt.hubID, nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestUnregisterTMF688Hub_HubDeleteError(t *testing.T) {
	// Test using a custom mock hub store that fails on delete
	gin.SetMode(gin.TestMode)
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	imsAdp := &mockIMSAdapter{}
	reg := registry.NewRegistry(logger, nil)
	hubStore := &failDeleteHubStore{
		inner: storage.NewInMemoryHubStore(),
	}

	// Pre-populate store
	err = hubStore.inner.Create(context.Background(), &storage.HubRegistration{
		HubID:          "hub-1",
		Callback:       "https://example.com/notify",
		SubscriptionID: "sub-1",
		CreatedAt:      time.Now(),
	})
	require.NoError(t, err)

	handler := NewTMForumHandler(imsAdp, reg, hubStore, logger)

	w := performRequestWithParam(handler.UnregisterTMF688Hub, http.MethodDelete,
		"/tmf-api/eventManagement/v4/hub/hub-1",
		"id", "hub-1", nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// failDeleteHubStore wraps InMemoryHubStore but fails on Delete.
type failDeleteHubStore struct {
	inner *storage.InMemoryHubStore
}

func (s *failDeleteHubStore) Create(ctx context.Context, hub *storage.HubRegistration) error {
	return s.inner.Create(ctx, hub)
}

func (s *failDeleteHubStore) Get(ctx context.Context, hubID string) (*storage.HubRegistration, error) {
	return s.inner.Get(ctx, hubID)
}

func (s *failDeleteHubStore) List(ctx context.Context) ([]*storage.HubRegistration, error) {
	return s.inner.List(ctx)
}

func (s *failDeleteHubStore) Delete(_ context.Context, _ string) error {
	return errors.New("delete failed")
}

func (s *failDeleteHubStore) Close() error {
	return s.inner.Close()
}

// failGetHubStore returns internal error on Get (not ErrHubNotFound).
type failGetHubStore struct {
	inner *storage.InMemoryHubStore
}

func (s *failGetHubStore) Create(ctx context.Context, hub *storage.HubRegistration) error {
	return s.inner.Create(ctx, hub)
}

func (s *failGetHubStore) Get(_ context.Context, _ string) (*storage.HubRegistration, error) {
	return nil, errors.New("internal store error")
}

func (s *failGetHubStore) List(ctx context.Context) ([]*storage.HubRegistration, error) {
	return s.inner.List(ctx)
}

func (s *failGetHubStore) Delete(ctx context.Context, hubID string) error {
	return s.inner.Delete(ctx, hubID)
}

func (s *failGetHubStore) Close() error {
	return s.inner.Close()
}

func TestUnregisterTMF688Hub_InternalGetError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	imsAdp := &mockIMSAdapter{}
	reg := registry.NewRegistry(logger, nil)
	hubStore := &failGetHubStore{
		inner: storage.NewInMemoryHubStore(),
	}

	handler := NewTMForumHandler(imsAdp, reg, hubStore, logger)

	w := performRequestWithParam(handler.UnregisterTMF688Hub, http.MethodDelete,
		"/tmf-api/eventManagement/v4/hub/hub-1",
		"id", "hub-1", nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ========================================
// TMF642 - Alarm Management Tests
// ========================================

func TestListTMF642Alarms(t *testing.T) {
	handler, _, _ := setupTMForumHandler(t)

	w := performRequestWithQuery(handler.ListTMF642Alarms, http.MethodGet,
		"/tmf-api/alarmManagement/v4/alarm",
		map[string]string{"perceivedSeverity": "critical", "state": "raised"})

	assert.Equal(t, http.StatusOK, w.Code)

	var alarms []models.TMF642Alarm
	err := json.Unmarshal(w.Body.Bytes(), &alarms)
	require.NoError(t, err)
	assert.Empty(t, alarms)
}

func TestGetTMF642Alarm(t *testing.T) {
	handler, _, _ := setupTMForumHandler(t)

	w := performRequestWithParam(handler.GetTMF642Alarm, http.MethodGet,
		"/tmf-api/alarmManagement/v4/alarm/alarm-1",
		"id", "alarm-1", nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAcknowledgeTMF642Alarm(t *testing.T) {
	tests := []struct {
		name           string
		alarmID        string
		body           interface{}
		expectedStatus int
	}{
		{
			name:    "acknowledge alarm (returns not found)",
			alarmID: "alarm-1",
			body: map[string]string{
				"state": "acknowledged",
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid body",
			alarmID:        "alarm-1",
			body:           "invalid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, _ := setupTMForumHandler(t)

			w := performRequestWithParam(handler.AcknowledgeTMF642Alarm, http.MethodPatch,
				"/tmf-api/alarmManagement/v4/alarm/"+tt.alarmID+"/acknowledge",
				"id", tt.alarmID, tt.body)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestClearTMF642Alarm(t *testing.T) {
	handler, _, _ := setupTMForumHandler(t)

	w := performRequestWithParam(handler.ClearTMF642Alarm, http.MethodPatch,
		"/tmf-api/alarmManagement/v4/alarm/alarm-1/clear",
		"id", "alarm-1", nil)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

// ========================================
// TMF640 - Service Activation Tests
// ========================================

func TestListTMF640ServiceActivations(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name: "list activations from deployments",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{
						ID:        "dep-1",
						Name:      "Activation 1",
						Status:    dmsadapter.DeploymentStatusDeployed,
						CreatedAt: now,
						UpdatedAt: now,
					},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "DMS error continues",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.listDeploymentsErr = errors.New("dms error")
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequest(handler.ListTMF640ServiceActivations, http.MethodGet,
				"/tmf-api/serviceActivation/v4/serviceActivation", nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestGetTMF640ServiceActivation(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		activationID   string
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name:         "get activation found",
			activationID: "dep-1",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{
					{ID: "dep-1", Status: dmsadapter.DeploymentStatusDeployed, CreatedAt: now, UpdatedAt: now},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:         "activation not found",
			activationID: "non-existent",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.deployments = []*dmsadapter.Deployment{}
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequestWithParam(handler.GetTMF640ServiceActivation, http.MethodGet,
				"/tmf-api/serviceActivation/v4/serviceActivation/"+tt.activationID,
				"id", tt.activationID, nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestCreateTMF640ServiceActivation(t *testing.T) {
	handler, _, _ := setupTMForumHandler(t)

	w := performRequest(handler.CreateTMF640ServiceActivation, http.MethodPost,
		"/tmf-api/serviceActivation/v4/serviceActivation", nil)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

// ========================================
// TMF620 - Product Catalog Management Tests
// ========================================

func TestListTMF620ProductOfferings(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name: "list offerings from packages",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.packages = []*dmsadapter.DeploymentPackage{
					{
						ID:          "pkg-1",
						Name:        "Package 1",
						Version:     "1.0.0",
						Description: "Test package",
						UploadedAt:  now,
					},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "DMS error continues",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.listPackagesErr = errors.New("dms error")
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequest(handler.ListTMF620ProductOfferings, http.MethodGet,
				"/tmf-api/productCatalog/v4/productOffering", nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestGetTMF620ProductOffering(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		offeringID     string
		setupDMS       func(*mockDMSAdapter)
		expectedStatus int
	}{
		{
			name:       "get offering found",
			offeringID: "pkg-1",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.packages = []*dmsadapter.DeploymentPackage{
					{ID: "pkg-1", Name: "Package 1", Version: "1.0.0", UploadedAt: now},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:       "offering not found",
			offeringID: "non-existent",
			setupDMS: func(dms *mockDMSAdapter) {
				dms.packages = []*dmsadapter.DeploymentPackage{}
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, dmsAdp := setupTMForumHandler(t)
			tt.setupDMS(dmsAdp)

			w := performRequestWithParam(handler.GetTMF620ProductOffering, http.MethodGet,
				"/tmf-api/productCatalog/v4/productOffering/"+tt.offeringID,
				"id", tt.offeringID, nil)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// ========================================
// Helper Function Tests (from tmforum_handler.go)
// ========================================

func TestTransformDeploymentToActivation(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)

	tests := []struct {
		name          string
		deployment    *dmsadapter.Deployment
		baseURL       string
		expectedState string
		hasActualDate bool
	}{
		{
			name: "deployed deployment",
			deployment: &dmsadapter.Deployment{
				ID:        "dep-1",
				Name:      "Test",
				Status:    dmsadapter.DeploymentStatusDeployed,
				CreatedAt: now,
				UpdatedAt: later,
			},
			baseURL:       "http://localhost",
			expectedState: "activated",
			hasActualDate: true,
		},
		{
			name: "pending deployment",
			deployment: &dmsadapter.Deployment{
				ID:        "dep-2",
				Name:      "Test2",
				Status:    dmsadapter.DeploymentStatusPending,
				CreatedAt: now,
				UpdatedAt: later,
			},
			baseURL:       "http://localhost",
			expectedState: "pending",
			hasActualDate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformDeploymentToActivation(tt.deployment, tt.baseURL)

			assert.Equal(t, tt.deployment.ID, result.ID)
			assert.Equal(t, tt.expectedState, result.State)
			assert.Equal(t, "automatic", result.Mode)
			assert.Equal(t, "ServiceActivation", result.AtType)
			assert.NotEmpty(t, result.Href)
			assert.NotNil(t, result.Service)
			assert.NotNil(t, result.RequestedActivationDate)

			if tt.hasActualDate {
				assert.NotNil(t, result.ActualActivationDate)
			} else {
				assert.Nil(t, result.ActualActivationDate)
			}
		})
	}
}

func TestMapDeploymentStatusToActivationState(t *testing.T) {
	tests := []struct {
		name     string
		status   dmsadapter.DeploymentStatus
		expected string
	}{
		{"pending", dmsadapter.DeploymentStatusPending, "pending"},
		{"deploying", dmsadapter.DeploymentStatusDeploying, stateInProgress},
		{"deployed", dmsadapter.DeploymentStatusDeployed, "activated"},
		{"failed", dmsadapter.DeploymentStatusFailed, "failed"},
		{"rolling back", dmsadapter.DeploymentStatusRollingBack, stateInProgress},
		{"deleting", dmsadapter.DeploymentStatusDeleting, stateInProgress},
		{"unknown", dmsadapter.DeploymentStatus("unknown"), "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapDeploymentStatusToActivationState(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransformPackageToOffering(t *testing.T) {
	tests := []struct {
		name    string
		pkg     *dmsadapter.DeploymentPackage
		baseURL string
	}{
		{
			name: "package with all fields",
			pkg: &dmsadapter.DeploymentPackage{
				ID:          "pkg-1",
				Name:        "Test Package",
				Version:     "1.0.0",
				Description: "A test package",
			},
			baseURL: "http://localhost",
		},
		{
			name: "package with empty ID",
			pkg: &dmsadapter.DeploymentPackage{
				ID:      "",
				Name:    "Unnamed",
				Version: "0.0.1",
			},
			baseURL: "http://localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformPackageToOffering(tt.pkg, tt.baseURL)

			assert.Equal(t, tt.pkg.ID, result.ID)
			assert.Equal(t, tt.pkg.Name, result.Name)
			assert.Equal(t, tt.pkg.Description, result.Description)
			assert.Equal(t, tt.pkg.Version, result.Version)
			assert.Equal(t, "Active", result.LifecycleStatus)
			assert.False(t, result.IsBundle)
			assert.Equal(t, "ProductOffering", result.AtType)

			if tt.pkg.ID != "" {
				assert.NotNil(t, result.ProductSpecification)
				assert.Equal(t, tt.pkg.ID, result.ProductSpecification.ID)
			} else {
				assert.Nil(t, result.ProductSpecification)
			}
		})
	}
}
