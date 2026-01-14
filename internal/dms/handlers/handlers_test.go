package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/dms/handlers"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/dms/adapter"
	"github.com/piwi3910/netweave/internal/dms/models"
	"github.com/piwi3910/netweave/internal/dms/registry"
	"github.com/piwi3910/netweave/internal/dms/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const testCallbackURL = "https://google.com/webhook"

// mockAdapter implements the adapter.DMSAdapter interface for testing.
type mockAdapter struct {
	name         string
	version      string
	capabilities []adapter.Capability
	healthy      bool
	healthErr    error

	deployments []*adapter.Deployment
	packages    []*adapter.DeploymentPackage

	getDeploymentErr        error
	createDeploymentErr     error
	updateDeploymentErr     error
	deleteDeploymentErr     error
	scaleDeploymentErr      error
	rollbackErr             error
	getDeploymentStatusErr  error
	getDeploymentHistoryErr error
	getPackageErr           error
	deleteDeploymentPkgErr  error
}

func newMockAdapter() *mockAdapter {
	return &mockAdapter{
		name:    "mock",
		version: "1.0.0",
		capabilities: []adapter.Capability{
			adapter.CapabilityDeploymentLifecycle, adapter.CapabilityRollback, adapter.CapabilityScaling,
		},
		healthy:     true,
		deployments: make([]*adapter.Deployment, 0),
		packages:    make([]*adapter.DeploymentPackage, 0),
	}
}

func (m *mockAdapter) Name() string                       { return m.name }
func (m *mockAdapter) Version() string                    { return m.version }
func (m *mockAdapter) Capabilities() []adapter.Capability { return m.capabilities }

func (m *mockAdapter) ListDeploymentPackages(
	_ context.Context, _ *adapter.Filter,
) ([]*adapter.DeploymentPackage, error) {
	return m.packages, nil
}

func (m *mockAdapter) GetDeploymentPackage(_ context.Context, id string) (*adapter.DeploymentPackage, error) {
	if m.getPackageErr != nil {
		return nil, m.getPackageErr
	}
	for _, pkg := range m.packages {
		if pkg.ID == id {
			return pkg, nil
		}
	}
	return nil, adapter.ErrPackageNotFound
}

func (m *mockAdapter) UploadDeploymentPackage(
	_ context.Context, pkg *adapter.DeploymentPackageUpload,
) (*adapter.DeploymentPackage, error) {
	newPkg := &adapter.DeploymentPackage{
		ID:          "pkg-" + pkg.Name,
		Name:        pkg.Name,
		Version:     pkg.Version,
		PackageType: pkg.PackageType,
		Description: pkg.Description,
		UploadedAt:  time.Now(),
	}
	m.packages = append(m.packages, newPkg)
	return newPkg, nil
}

func (m *mockAdapter) DeleteDeploymentPackage(_ context.Context, id string) error {
	if m.deleteDeploymentPkgErr != nil {
		return m.deleteDeploymentPkgErr
	}
	for i, pkg := range m.packages {
		if pkg.ID == id {
			m.packages = append(m.packages[:i], m.packages[i+1:]...)
			return nil
		}
	}
	return adapter.ErrPackageNotFound
}

func (m *mockAdapter) ListDeployments(_ context.Context, _ *adapter.Filter) ([]*adapter.Deployment, error) {
	return m.deployments, nil
}

func (m *mockAdapter) GetDeployment(_ context.Context, id string) (*adapter.Deployment, error) {
	if m.getDeploymentErr != nil {
		return nil, m.getDeploymentErr
	}
	for _, d := range m.deployments {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, adapter.ErrDeploymentNotFound
}

func (m *mockAdapter) CreateDeployment(_ context.Context, req *adapter.DeploymentRequest) (*adapter.Deployment, error) {
	if m.createDeploymentErr != nil {
		return nil, m.createDeploymentErr
	}
	deployment := &adapter.Deployment{
		ID:          "dep-" + req.Name,
		Name:        req.Name,
		PackageID:   req.PackageID,
		Namespace:   req.Namespace,
		Status:      adapter.DeploymentStatusDeployed,
		Version:     1,
		Description: req.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.deployments = append(m.deployments, deployment)
	return deployment, nil
}

func (m *mockAdapter) UpdateDeployment(
	_ context.Context, id string, update *adapter.DeploymentUpdate,
) (*adapter.Deployment, error) {
	if m.updateDeploymentErr != nil {
		return nil, m.updateDeploymentErr
	}
	for _, d := range m.deployments {
		if d.ID == id {
			if update.Description != "" {
				d.Description = update.Description
			}
			d.Version++
			d.UpdatedAt = time.Now()
			return d, nil
		}
	}
	return nil, adapter.ErrDeploymentNotFound
}

func (m *mockAdapter) DeleteDeployment(_ context.Context, id string) error {
	if m.deleteDeploymentErr != nil {
		return m.deleteDeploymentErr
	}
	for i, d := range m.deployments {
		if d.ID == id {
			m.deployments = append(m.deployments[:i], m.deployments[i+1:]...)
			return nil
		}
	}
	return adapter.ErrDeploymentNotFound
}

func (m *mockAdapter) ScaleDeployment(_ context.Context, _ string, _ int) error {
	return m.scaleDeploymentErr
}

func (m *mockAdapter) RollbackDeployment(_ context.Context, _ string, _ int) error {
	return m.rollbackErr
}

func (m *mockAdapter) GetDeploymentStatus(_ context.Context, id string) (*adapter.DeploymentStatusDetail, error) {
	if m.getDeploymentStatusErr != nil {
		return nil, m.getDeploymentStatusErr
	}
	for _, d := range m.deployments {
		if d.ID == id {
			return &adapter.DeploymentStatusDetail{
				DeploymentID: id,
				Status:       d.Status,
				Message:      "Deployment is healthy",
				Progress:     100,
				UpdatedAt:    d.UpdatedAt,
			}, nil
		}
	}
	return nil, adapter.ErrDeploymentNotFound
}

func (m *mockAdapter) GetDeploymentHistory(_ context.Context, id string) (*adapter.DeploymentHistory, error) {
	if m.getDeploymentHistoryErr != nil {
		return nil, m.getDeploymentHistoryErr
	}
	for _, d := range m.deployments {
		if d.ID == id {
			return &adapter.DeploymentHistory{
				DeploymentID: id,
				Revisions: []adapter.DeploymentRevision{
					{
						Revision:   1,
						Version:    "1.0.0",
						DeployedAt: d.CreatedAt,
						Status:     adapter.DeploymentStatusDeployed,
					},
				},
			}, nil
		}
	}
	return nil, adapter.ErrDeploymentNotFound
}

func (m *mockAdapter) GetDeploymentLogs(_ context.Context, _ string, _ *adapter.LogOptions) ([]byte, error) {
	return []byte("mock logs"), nil
}

func (m *mockAdapter) SupportsRollback() bool { return true }
func (m *mockAdapter) SupportsScaling() bool  { return true }
func (m *mockAdapter) SupportsGitOps() bool   { return false }

func (m *mockAdapter) Health(_ context.Context) error {
	if !m.healthy {
		return m.healthErr
	}
	return nil
}

func (m *mockAdapter) Close() error { return nil }

// setupTestHandler creates a test handler with mock dependencies.
func setupTestHandler(t *testing.T) (*handlers.Handler, *mockAdapter) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	reg := registry.NewRegistry(logger, nil)
	mockAdp := newMockAdapter()

	err := reg.Register(context.Background(), "mock", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	store := storage.NewMemoryStore()
	handler := handlers.NewHandler(reg, store, logger)

	return handler, mockAdp
}

// setupTestRouter creates a test router with the handler configured.
func setupTestRouter(handler *handlers.Handler) *gin.Engine {
	router := gin.New()

	v1 := router.Group("/o2dms/v1")
	{
		v1.GET("/deploymentLifecycle", handler.GetDeploymentLifecycleInfo)

		nfDeployments := v1.Group("/nfDeployments")
		{
			nfDeployments.GET("", handler.ListNFDeployments)
			nfDeployments.POST("", handler.CreateNFDeployment)
			nfDeployments.GET("/:nfDeploymentId", handler.GetNFDeployment)
			nfDeployments.PUT("/:nfDeploymentId", handler.UpdateNFDeployment)
			nfDeployments.DELETE("/:nfDeploymentId", handler.DeleteNFDeployment)
			nfDeployments.POST("/:nfDeploymentId/scale", handler.ScaleNFDeployment)
			nfDeployments.POST("/:nfDeploymentId/rollback", handler.RollbackNFDeployment)
			nfDeployments.GET("/:nfDeploymentId/status", handler.GetNFDeploymentStatus)
			nfDeployments.GET("/:nfDeploymentId/history", handler.GetNFDeploymentHistory)
		}

		descriptors := v1.Group("/nfDeploymentDescriptors")
		{
			descriptors.GET("", handler.ListNFDeploymentDescriptors)
			descriptors.POST("", handler.CreateNFDeploymentDescriptor)
			descriptors.GET("/:nfDeploymentDescriptorId", handler.GetNFDeploymentDescriptor)
			descriptors.DELETE("/:nfDeploymentDescriptorId", handler.DeleteNFDeploymentDescriptor)
		}

		subscriptions := v1.Group("/subscriptions")
		{
			subscriptions.GET("", handler.ListDMSSubscriptions)
			subscriptions.POST("", handler.CreateDMSSubscription)
			subscriptions.GET("/:subscriptionId", handler.GetDMSSubscription)
			subscriptions.DELETE("/:subscriptionId", handler.DeleteDMSSubscription)
		}
	}

	return router
}

// NF Deployment Tests

func TestListNFDeployments(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Add some deployments to the mock adapter.
	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:        "dep-1",
			Name:      "test-deployment-1",
			PackageID: "pkg-1",
			Namespace: "default",
			Status:    adapter.DeploymentStatusDeployed,
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        "dep-2",
			Name:      "test-deployment-2",
			PackageID: "pkg-2",
			Namespace: "production",
			Status:    adapter.DeploymentStatusDeploying,
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.NFDeploymentListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.Total)
	assert.Len(t, response.NFDeployments, 2)
}

func TestGetNFDeployment(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:        "dep-1",
			Name:      "test-deployment",
			PackageID: "pkg-1",
			Namespace: "default",
			Status:    adapter.DeploymentStatusDeployed,
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments/dep-1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var deployment models.NFDeployment
	err := json.Unmarshal(w.Body.Bytes(), &deployment)
	require.NoError(t, err)

	assert.Equal(t, "dep-1", deployment.NFDeploymentID)
	assert.Equal(t, "test-deployment", deployment.Name)
}

func TestGetNFDeployment_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateNFDeployment(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	createReq := models.CreateNFDeploymentRequest{
		Name:                     "new-deployment",
		Description:              "Test deployment",
		NFDeploymentDescriptorID: "pkg-1",
		Namespace:                "default",
		ParameterValues:          map[string]interface{}{"replicas": 3},
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var deployment models.NFDeployment
	err = json.Unmarshal(w.Body.Bytes(), &deployment)
	require.NoError(t, err)

	assert.Equal(t, "new-deployment", deployment.Name)
	assert.Equal(t, "pkg-1", deployment.NFDeploymentDescriptorID)
}

func TestCreateNFDeployment_InvalidRequest(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Missing required fields.
	createReq := models.CreateNFDeploymentRequest{
		Description: "Missing name",
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateNFDeployment(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:        "dep-1",
			Name:      "test-deployment",
			PackageID: "pkg-1",
			Namespace: "default",
			Status:    adapter.DeploymentStatusDeployed,
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	updateReq := models.UpdateNFDeploymentRequest{
		Description: "Updated description",
		ParameterValues: map[string]interface{}{
			"replicas": 5,
		},
	}

	body, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/o2dms/v1/nfDeployments/dep-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var deployment models.NFDeployment
	err = json.Unmarshal(w.Body.Bytes(), &deployment)
	require.NoError(t, err)

	assert.Equal(t, "Updated description", deployment.Description)
	assert.Equal(t, 2, deployment.Version) // Version should be incremented.
}

func TestDeleteNFDeployment(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:        "dep-1",
			Name:      "test-deployment",
			PackageID: "pkg-1",
			Status:    adapter.DeploymentStatusDeployed,
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/nfDeployments/dep-1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, mockAdp.deployments)
}

func TestScaleNFDeployment(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:     "dep-1",
			Name:   "test-deployment",
			Status: adapter.DeploymentStatusDeployed,
		},
	}

	scaleReq := models.ScaleNFDeploymentRequest{
		Replicas: 5,
	}

	body, err := json.Marshal(scaleReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/dep-1/scale", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestRollbackNFDeployment(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:      "dep-1",
			Name:    "test-deployment",
			Status:  adapter.DeploymentStatusDeployed,
			Version: 3,
		},
	}

	revision := 1
	rollbackReq := models.RollbackNFDeploymentRequest{
		TargetRevision: &revision,
	}

	body, err := json.Marshal(rollbackReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/dep-1/rollback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestGetNFDeploymentStatus(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:        "dep-1",
			Name:      "test-deployment",
			Status:    adapter.DeploymentStatusDeployed,
			UpdatedAt: time.Now(),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments/dep-1/status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status models.DeploymentStatusResponse
	err := json.Unmarshal(w.Body.Bytes(), &status)
	require.NoError(t, err)

	assert.Equal(t, "dep-1", status.NFDeploymentID)
	assert.Equal(t, models.NFDeploymentStatusDeployed, status.Status)
	assert.Equal(t, 100, status.Progress)
}

func TestGetNFDeploymentHistory(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:        "dep-1",
			Name:      "test-deployment",
			Status:    adapter.DeploymentStatusDeployed,
			CreatedAt: time.Now(),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments/dep-1/history", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var history models.DeploymentHistoryResponse
	err := json.Unmarshal(w.Body.Bytes(), &history)
	require.NoError(t, err)

	assert.Equal(t, "dep-1", history.NFDeploymentID)
	assert.Len(t, history.Revisions, 1)
}

// NF Deployment Descriptor Tests

func TestListNFDeploymentDescriptors(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.packages = []*adapter.DeploymentPackage{
		{
			ID:          "pkg-1",
			Name:        "test-chart",
			Version:     "1.0.0",
			PackageType: "helm-chart",
			UploadedAt:  time.Now(),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeploymentDescriptors", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.NFDeploymentDescriptorListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 1, response.Total)
	assert.Len(t, response.NFDeploymentDescriptors, 1)
}

func TestCreateNFDeploymentDescriptor(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	createReq := models.CreateNFDeploymentDescriptorRequest{
		Name:            "new-descriptor",
		Description:     "Test descriptor",
		ArtifactName:    "my-chart",
		ArtifactVersion: "1.0.0",
		ArtifactType:    "helm-chart",
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeploymentDescriptors", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var descriptor models.NFDeploymentDescriptor
	err = json.Unmarshal(w.Body.Bytes(), &descriptor)
	require.NoError(t, err)

	assert.Equal(t, "my-chart", descriptor.ArtifactName)
}

// DMS Subscription Tests

func TestListDMSSubscriptions(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/subscriptions", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.DMSSubscriptionListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Total)
}

func TestCreateDMSSubscription(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Use a public HTTPS URL that will pass DNS validation.
	// google.com reliably resolves to public IPs in all environments.

	createReq := models.CreateDMSSubscriptionRequest{
		Callback:               testCallbackURL,
		ConsumerSubscriptionID: "consumer-123",
		Filter: &models.DMSSubscriptionFilter{
			Namespace: "default",
		},
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/subscriptions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Logf("Response body: %s", w.Body.String())
	}
	assert.Equal(t, http.StatusCreated, w.Code)

	var subscription models.DMSSubscription
	err = json.Unmarshal(w.Body.Bytes(), &subscription)
	require.NoError(t, err)

	assert.NotEmpty(t, subscription.SubscriptionID)
	assert.Equal(t, testCallbackURL, subscription.Callback)
	assert.Equal(t, "consumer-123", subscription.ConsumerSubscriptionID)
}

func TestGetDMSSubscription(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	// First create a subscription.
	createReq := models.CreateDMSSubscriptionRequest{
		Callback: testCallbackURL,
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	createReqHTTP := httptest.NewRequest(http.MethodPost, "/o2dms/v1/subscriptions", bytes.NewReader(body))
	createReqHTTP.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()

	router.ServeHTTP(createResp, createReqHTTP)
	require.Equal(t, http.StatusCreated, createResp.Code)

	var created models.DMSSubscription
	err = json.Unmarshal(createResp.Body.Bytes(), &created)
	require.NoError(t, err)

	// Now get the subscription.
	getReq := httptest.NewRequest(http.MethodGet, "/o2dms/v1/subscriptions/"+created.SubscriptionID, nil)
	getResp := httptest.NewRecorder()

	router.ServeHTTP(getResp, getReq)

	assert.Equal(t, http.StatusOK, getResp.Code)

	var retrieved models.DMSSubscription
	err = json.Unmarshal(getResp.Body.Bytes(), &retrieved)
	require.NoError(t, err)

	assert.Equal(t, created.SubscriptionID, retrieved.SubscriptionID)
}

func TestDeleteDMSSubscription(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	// First create a subscription.
	createReq := models.CreateDMSSubscriptionRequest{
		Callback: testCallbackURL,
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	createReqHTTP := httptest.NewRequest(http.MethodPost, "/o2dms/v1/subscriptions", bytes.NewReader(body))
	createReqHTTP.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()

	router.ServeHTTP(createResp, createReqHTTP)
	require.Equal(t, http.StatusCreated, createResp.Code)

	var created models.DMSSubscription
	err = json.Unmarshal(createResp.Body.Bytes(), &created)
	require.NoError(t, err)

	// Now delete the subscription.
	deleteReq := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/subscriptions/"+created.SubscriptionID, nil)
	deleteResp := httptest.NewRecorder()

	router.ServeHTTP(deleteResp, deleteReq)

	assert.Equal(t, http.StatusNoContent, deleteResp.Code)

	// Verify it's deleted.
	getReq := httptest.NewRequest(http.MethodGet, "/o2dms/v1/subscriptions/"+created.SubscriptionID, nil)
	getResp := httptest.NewRecorder()

	router.ServeHTTP(getResp, getReq)

	assert.Equal(t, http.StatusNotFound, getResp.Code)
}

func TestGetDMSSubscription_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/subscriptions/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Deployment Lifecycle Info Test

func TestGetDeploymentLifecycleInfo(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/deploymentLifecycle", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "v1", response["apiVersion"])
	assert.Equal(t, "/o2dms/v1", response["basePath"])
	assert.NotNil(t, response["adapters"])
}

// Error Handling Tests

func TestHandler_NoDefaultAdapter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	// Create empty registry with no default adapter.
	reg := registry.NewRegistry(logger, nil)
	store := storage.NewMemoryStore()
	handler := handlers.NewHandler(reg, store, logger)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response models.APIError
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "ServiceUnavailable", response.Error)
}

func TestHandler_AdapterByQueryParam(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:     "dep-1",
			Name:   "test-deployment",
			Status: adapter.DeploymentStatusDeployed,
		},
	}

	// Use the adapter name in query param.
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments?adapter=mock", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_AdapterByQueryParam_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments?adapter=nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandler_InvalidJSONBody(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Send invalid JSON.
	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateDeploymentError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set error on mock adapter.
	mockAdp.createDeploymentErr = adapter.ErrDeploymentNotFound

	createReq := models.CreateNFDeploymentRequest{
		Name:                     "test",
		NFDeploymentDescriptorID: "pkg-1",
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_UpdateDeploymentError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{ID: "dep-1", Name: "test", Status: adapter.DeploymentStatusDeployed},
	}
	mockAdp.updateDeploymentErr = adapter.ErrDeploymentNotFound

	updateReq := models.UpdateNFDeploymentRequest{
		Description: "Updated",
	}

	body, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/o2dms/v1/nfDeployments/dep-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_DeleteDeploymentError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deleteDeploymentErr = adapter.ErrDeploymentNotFound

	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/nfDeployments/dep-1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_ScaleDeploymentError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{ID: "dep-1", Name: "test", Status: adapter.DeploymentStatusDeployed},
	}
	mockAdp.scaleDeploymentErr = adapter.ErrOperationNotSupported

	scaleReq := models.ScaleNFDeploymentRequest{Replicas: 5}
	body, err := json.Marshal(scaleReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/dep-1/scale", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_RollbackDeploymentError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{ID: "dep-1", Name: "test", Status: adapter.DeploymentStatusDeployed, Version: 3},
	}
	mockAdp.rollbackErr = adapter.ErrOperationNotSupported

	rollbackReq := models.RollbackNFDeploymentRequest{}
	body, err := json.Marshal(rollbackReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/dep-1/rollback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_GetNFDeploymentStatus_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments/nonexistent/status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_GetNFDeploymentHistory_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments/nonexistent/history", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_GetNFDeploymentDescriptor_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeploymentDescriptors/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_DeleteNFDeploymentDescriptor(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.packages = []*adapter.DeploymentPackage{
		{
			ID:      "pkg-1",
			Name:    "test-chart",
			Version: "1.0.0",
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/nfDeploymentDescriptors/pkg-1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandler_DeleteNFDeploymentDescriptor_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/nfDeploymentDescriptors/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_DeleteDMSSubscription_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/subscriptions/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_Health(t *testing.T) {
	handler, _ := setupTestHandler(t)

	err := handler.Health(context.Background())
	assert.NoError(t, err)
}

func TestHandler_Health_NoAdapter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	// Create empty registry with no adapter.
	reg := registry.NewRegistry(logger, nil)
	store := storage.NewMemoryStore()
	handler := handlers.NewHandler(reg, store, logger)

	err := handler.Health(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no DMS adapter available")
}

// Mock adapter that doesn't support scaling/rollback

type noScaleRollbackAdapter struct {
	*mockAdapter
}

func (m *noScaleRollbackAdapter) SupportsRollback() bool { return false }
func (m *noScaleRollbackAdapter) SupportsScaling() bool  { return false }

func TestHandler_ScaleNotSupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	reg := registry.NewRegistry(logger, nil)
	adp := &noScaleRollbackAdapter{mockAdapter: newMockAdapter()}

	err := reg.Register(context.Background(), "no-scale", "mock", adp, nil, true)
	require.NoError(t, err)

	store := storage.NewMemoryStore()
	handler := handlers.NewHandler(reg, store, logger)
	router := setupTestRouter(handler)

	scaleReq := models.ScaleNFDeploymentRequest{Replicas: 5}
	body, err := json.Marshal(scaleReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/dep-1/scale", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

func TestHandler_RollbackNotSupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	reg := registry.NewRegistry(logger, nil)
	adp := &noScaleRollbackAdapter{mockAdapter: newMockAdapter()}

	err := reg.Register(context.Background(), "no-rollback", "mock", adp, nil, true)
	require.NoError(t, err)

	store := storage.NewMemoryStore()
	handler := handlers.NewHandler(reg, store, logger)
	router := setupTestRouter(handler)

	rollbackReq := models.RollbackNFDeploymentRequest{}
	body, err := json.Marshal(rollbackReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/dep-1/rollback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

func TestHandler_SubscriptionNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	reg := registry.NewRegistry(logger, nil)
	mockAdp := newMockAdapter()
	err := reg.Register(context.Background(), "mock", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	// Create handler with nil store.
	handler := handlers.NewHandler(reg, nil, logger)
	router := setupTestRouter(handler)

	// Test list subscriptions with no store.
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/subscriptions", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandler_CreateSubscriptionNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	reg := registry.NewRegistry(logger, nil)
	mockAdp := newMockAdapter()
	err := reg.Register(context.Background(), "mock", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	handler := handlers.NewHandler(reg, nil, logger)
	router := setupTestRouter(handler)

	createReq := models.CreateDMSSubscriptionRequest{
		Callback: testCallbackURL,
	}
	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/subscriptions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandler_GetSubscriptionNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	reg := registry.NewRegistry(logger, nil)
	mockAdp := newMockAdapter()
	err := reg.Register(context.Background(), "mock", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	handler := handlers.NewHandler(reg, nil, logger)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/subscriptions/sub-1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandler_DeleteSubscriptionNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	reg := registry.NewRegistry(logger, nil)
	mockAdp := newMockAdapter()
	err := reg.Register(context.Background(), "mock", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	handler := handlers.NewHandler(reg, nil, logger)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/subscriptions/sub-1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandler_ListWithFilter(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:        "dep-1",
			Name:      "test-deployment",
			Namespace: "default",
			Status:    adapter.DeploymentStatusDeployed,
		},
	}

	// Test with namespace filter.
	req := httptest.NewRequest(
		http.MethodGet,
		"/o2dms/v1/nfDeployments?namespace=default&status=deployed&limit=10&offset=0",
		nil,
	)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_ListDescriptorsWithFilter(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.packages = []*adapter.DeploymentPackage{
		{
			ID:      "pkg-1",
			Name:    "test-chart",
			Version: "1.0.0",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeploymentDescriptors?limit=10&offset=0", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_CreateSubscriptionWithFilter(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	createReq := models.CreateDMSSubscriptionRequest{
		Callback:               testCallbackURL,
		ConsumerSubscriptionID: "consumer-123",
		Filter: &models.DMSSubscriptionFilter{
			NFDeploymentIDs:           []string{"nfd-1", "nfd-2"},
			NFDeploymentDescriptorIDs: []string{"nfdd-1"},
			Namespace:                 "production",
			EventTypes:                []models.DMSEventType{models.DMSEventTypeDeploymentCreated},
		},
		Extensions: map[string]interface{}{
			"custom": "value",
		},
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/subscriptions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var subscription models.DMSSubscription
	err = json.Unmarshal(w.Body.Bytes(), &subscription)
	require.NoError(t, err)

	assert.NotNil(t, subscription.Filter)
	assert.Equal(t, "production", subscription.Filter.Namespace)
	assert.Len(t, subscription.Filter.NFDeploymentIDs, 2)
}

// Security Function Tests

func TestValidateCallbackURL(t *testing.T) {
	tests := []struct {
		name        string
		callbackURL string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid HTTPS URL",
			callbackURL: "https://example.com/webhook",
			wantErr:     false,
		},
		{
			name:        "valid HTTPS URL with port",
			callbackURL: "https://example.com:8443/webhook",
			wantErr:     false,
		},
		{
			name:        "valid HTTPS URL with path",
			callbackURL: "https://smo.example.com/api/v1/notifications",
			wantErr:     false,
		},
		{
			name:        "HTTP URL rejected",
			callbackURL: "http://example.com/webhook",
			wantErr:     true,
			errContains: "must use HTTPS",
		},
		{
			name:        "localhost rejected",
			callbackURL: "https://localhost/webhook",
			wantErr:     true,
			errContains: "cannot point to localhost",
		},
		{
			name:        "localhost with port rejected",
			callbackURL: "https://localhost:8443/webhook",
			wantErr:     true,
			errContains: "cannot point to localhost",
		},
		{
			name:        "127.0.0.1 rejected",
			callbackURL: "https://127.0.0.1/webhook",
			wantErr:     true,
			errContains: "cannot point to localhost",
		},
		{
			name:        "127.x.x.x rejected",
			callbackURL: "https://127.0.1.1/webhook",
			wantErr:     true,
			errContains: "cannot point to localhost",
		},
		{
			name:        "IPv6 loopback rejected",
			callbackURL: "https://[::1]/webhook",
			wantErr:     true,
			errContains: "cannot point to localhost",
		},
		{
			name:        "invalid URL format",
			callbackURL: "not-a-url",
			wantErr:     true,
			errContains: "must have a valid host", // Checked before scheme.
		},
		{
			name:        "empty URL",
			callbackURL: "",
			wantErr:     true,
			errContains: "must have a valid host",
		},
		{
			name:        "URL with no host",
			callbackURL: "https:///webhook",
			wantErr:     true,
			errContains: "must have a valid host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handlers.ValidateCallbackURL(tt.callbackURL)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		isPrivate bool
	}{
		// Public IPs.
		{"public IP 8.8.8.8", "8.8.8.8", false},
		{"public IP 1.1.1.1", "1.1.1.1", false},
		// Note: 203.0.113.0/24 is TEST-NET-3 (RFC 5737), reserved for documentation.
		{"reserved TEST-NET-3", "203.0.113.1", true},

		// Private Class A (10.0.0.0/8).
		{"private 10.0.0.1", "10.0.0.1", true},
		{"private 10.255.255.255", "10.255.255.255", true},

		// Private Class B (172.16.0.0/12).
		{"private 172.16.0.1", "172.16.0.1", true},
		{"private 172.31.255.255", "172.31.255.255", true},
		{"public 172.32.0.1", "172.32.0.1", false}, // Not in 172.16/12.

		// Private Class C (192.168.0.0/16).
		{"private 192.168.0.1", "192.168.0.1", true},
		{"private 192.168.255.255", "192.168.255.255", true},

		// Loopback.
		{"loopback 127.0.0.1", "127.0.0.1", true},
		{"loopback 127.255.255.255", "127.255.255.255", true},

		// Link-local (169.254.0.0/16).
		{"link-local 169.254.0.1", "169.254.0.1", true},
		{"link-local 169.254.255.255", "169.254.255.255", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "failed to parse IP: %s", tt.ip)
			result := handlers.IsPrivateIP(ip)
			assert.Equal(t, tt.isPrivate, result)
		})
	}
}

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple URL unchanged",
			input:    "https://example.com/webhook",
			expected: "https://example.com/webhook",
		},
		{
			name:     "query params removed",
			input:    "https://example.com/webhook?token=secret&key=apikey",
			expected: "https://example.com/webhook",
		},
		{
			name:     "user info removed",
			input:    "https://user:password@example.com/webhook",
			expected: "https://example.com/webhook",
		},
		{
			name:     "fragment removed",
			input:    "https://example.com/webhook#section",
			expected: "https://example.com/webhook",
		},
		{
			name:     "all sensitive parts removed",
			input:    "https://user:pass@example.com/webhook?token=secret#section",
			expected: "https://example.com/webhook",
		},
		{
			name:     "invalid URL returns placeholder",
			input:    "://invalid",
			expected: "[invalid-url]",
		},
		{
			name:     "port preserved",
			input:    "https://example.com:8443/webhook?secret=value",
			expected: "https://example.com:8443/webhook",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handlers.RedactURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidatePaginationLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{
			name:     "zero returns default",
			input:    0,
			expected: handlers.DefaultPaginationLimit,
		},
		{
			name:     "negative returns default",
			input:    -1,
			expected: handlers.DefaultPaginationLimit,
		},
		{
			name:     "within range unchanged",
			input:    50,
			expected: 50,
		},
		{
			name:     "at max unchanged",
			input:    handlers.MaxPaginationLimit,
			expected: handlers.MaxPaginationLimit,
		},
		{
			name:     "exceeds max capped",
			input:    handlers.MaxPaginationLimit + 1,
			expected: handlers.MaxPaginationLimit,
		},
		{
			name:     "very large capped",
			input:    10000,
			expected: handlers.MaxPaginationLimit,
		},
		{
			name:     "one is valid",
			input:    1,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handlers.ValidatePaginationLimit(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Callback URL validation integration tests

func TestCreateDMSSubscription_HTTPCallbackRejected(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	createReq := models.CreateDMSSubscriptionRequest{
		Callback: "http://example.com/webhook", // HTTP, not HTTPS.
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/subscriptions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var apiErr models.APIError
	err = json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Contains(t, apiErr.Message, "HTTPS")
}

func TestCreateDMSSubscription_LocalhostCallbackRejected(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	createReq := models.CreateDMSSubscriptionRequest{
		Callback: "https://localhost/webhook",
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/subscriptions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var apiErr models.APIError
	err = json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Contains(t, apiErr.Message, "localhost")
}

func TestCreateDMSSubscription_LoopbackIPRejected(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	createReq := models.CreateDMSSubscriptionRequest{
		Callback: "https://127.0.0.1/webhook",
	}

	body, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/subscriptions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateDMSSubscription_InvalidCallbackBody(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Missing callback URL entirely - test binding validation.
	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/subscriptions", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should fail because callback is required.
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Cloud metadata endpoint tests

func TestIsCloudMetadataEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		isMetadata bool
	}{
		// Cloud metadata endpoints.
		{"AWS/GCP/Azure metadata", "169.254.169.254", true},
		{"GCP internal metadata", "metadata.google.internal", true},
		{"GCP metadata short", "metadata.goog", true},
		{"AWS ECS metadata", "169.254.170.2", true},
		{"AWS IPv6 metadata", "fd00:ec2::254", true},

		// Not metadata endpoints.
		{"regular IP", "8.8.8.8", false},
		{"regular domain", "example.com", false},
		{"similar but not metadata", "169.254.169.253", false},
		{"metadata subdomain", "test.metadata.google.internal", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handlers.IsCloudMetadataEndpoint(tt.host)
			assert.Equal(t, tt.isMetadata, result)
		})
	}
}

func TestIsPrivateIP_IPv6(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		isPrivate bool
	}{
		// IPv6 public addresses.
		{"public IPv6", "2607:f8b0:4004:800::200e", false},

		// IPv6 private/reserved ranges.
		{"IPv6 ULA fc00::", "fc00::1", true},
		{"IPv6 ULA fd00::", "fd00::1", true},
		{"IPv6 link-local", "fe80::1", true},
		{"IPv6 multicast", "ff02::1", true},
		{"IPv6 loopback", "::1", true},
		{"IPv6 documentation", "2001:db8::1", true},

		// IPv4-mapped IPv6.
		// Note: Go's net.ParseIP normalizes ::ffff:x.x.x.x to x.x.x.x (IPv4 form),
		// so IPv4-mapped addresses are indistinguishable from pure IPv4 after parsing.
		// Therefore, they follow IPv4 private/public rules based on the underlying IP.
		{"IPv4-mapped private", "::ffff:192.168.1.1", true}, // Private underlying IPv4.
		{"IPv4-mapped public", "::ffff:8.8.8.8", false},     // Public underlying IPv4.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "failed to parse IP: %s", tt.ip)
			result := handlers.IsPrivateIP(ip)
			assert.Equal(t, tt.isPrivate, result)
		})
	}
}

func TestValidateCallbackURL_CloudMetadata(t *testing.T) {
	tests := []struct {
		name        string
		callbackURL string
		wantErr     bool
		errContains string
	}{
		{
			name:        "AWS metadata endpoint rejected",
			callbackURL: "https://169.254.169.254/latest/meta-data",
			wantErr:     true,
			errContains: "cloud metadata",
		},
		{
			name:        "GCP metadata endpoint rejected",
			callbackURL: "https://metadata.google.internal/computeMetadata/v1",
			wantErr:     true,
			errContains: "cloud metadata",
		},
		{
			name:        "AWS ECS metadata rejected",
			callbackURL: "https://169.254.170.2/v2/metadata",
			wantErr:     true,
			errContains: "cloud metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handlers.ValidateCallbackURL(tt.callbackURL)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Deployment name validation tests

func TestValidateDeploymentName(t *testing.T) {
	tests := []struct {
		name        string
		deployName  string
		wantErr     bool
		errContains string
	}{
		// Valid names.
		{
			name:       "simple lowercase name",
			deployName: "myapp",
			wantErr:    false,
		},
		{
			name:       "name with hyphens",
			deployName: "my-app-v1",
			wantErr:    false,
		},
		{
			name:       "name with numbers",
			deployName: "app123",
			wantErr:    false,
		},
		{
			name:       "single character",
			deployName: "a",
			wantErr:    false,
		},
		{
			name:       "two characters",
			deployName: "ab",
			wantErr:    false,
		},
		{
			name:       "max length 63 chars",
			deployName: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantErr:    false,
		},

		// Invalid names.
		{
			name:        "empty name",
			deployName:  "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "too long (64 chars)",
			deployName:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantErr:     true,
			errContains: "exceeds maximum length",
		},
		{
			name:        "uppercase letters",
			deployName:  "MyApp",
			wantErr:     true,
			errContains: "DNS-1123",
		},
		{
			name:        "starts with hyphen",
			deployName:  "-myapp",
			wantErr:     true,
			errContains: "DNS-1123",
		},
		{
			name:        "ends with hyphen",
			deployName:  "myapp-",
			wantErr:     true,
			errContains: "DNS-1123",
		},
		{
			name:        "contains underscore",
			deployName:  "my_app",
			wantErr:     true,
			errContains: "DNS-1123",
		},
		{
			name:        "contains dot",
			deployName:  "my.app",
			wantErr:     true,
			errContains: "DNS-1123",
		},
		{
			name:        "contains space",
			deployName:  "my app",
			wantErr:     true,
			errContains: "DNS-1123",
		},
		{
			name:        "special characters",
			deployName:  "my@app",
			wantErr:     true,
			errContains: "DNS-1123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handlers.ValidateDeploymentName(tt.deployName)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Integration tests for deployment name validation

func TestCreateNFDeployment_InvalidName(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	tests := []struct {
		name        string
		deployName  string
		errContains string
	}{
		{
			name:        "empty name",
			deployName:  "",
			errContains: "required",
		},
		{
			name:        "uppercase name",
			deployName:  "MyApp",
			errContains: "DNS-1123",
		},
		{
			name:        "name with underscore",
			deployName:  "my_app",
			errContains: "DNS-1123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createReq := models.CreateNFDeploymentRequest{
				Name:                     tt.deployName,
				NFDeploymentDescriptorID: "pkg-1",
			}

			body, err := json.Marshal(createReq)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var apiErr models.APIError
			err = json.Unmarshal(w.Body.Bytes(), &apiErr)
			require.NoError(t, err)
			assert.Contains(t, apiErr.Message, tt.errContains)
		})
	}
}

// Edge case tests

func TestScaleNFDeployment_InvalidReplicas(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:     "dep-1",
			Name:   "test-deployment",
			Status: adapter.DeploymentStatusDeployed,
		},
	}

	// Test with invalid JSON body.
	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/dep-1/scale", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRollbackNFDeployment_InvalidBody(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{
			ID:      "dep-1",
			Name:    "test-deployment",
			Status:  adapter.DeploymentStatusDeployed,
			Version: 3,
		},
	}

	// Test with invalid JSON body.
	req := httptest.NewRequest(
		http.MethodPost, "/o2dms/v1/nfDeployments/dep-1/rollback", bytes.NewReader([]byte("invalid")),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListNFDeployments_InvalidFilterParams(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Test with invalid limit (string instead of int).
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments?limit=invalid", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateNFDeploymentDescriptor_InvalidBody(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Test with invalid JSON body.
	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeploymentDescriptors", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateNFDeployment_InvalidBody(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	mockAdp.deployments = []*adapter.Deployment{
		{ID: "dep-1", Name: "test", Status: adapter.DeploymentStatusDeployed},
	}

	// Test with invalid JSON body.
	req := httptest.NewRequest(http.MethodPut, "/o2dms/v1/nfDeployments/dep-1", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Error handling differentiation tests (404 vs 500)

func TestGetNFDeployment_InternalError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set internal error (not ErrDeploymentNotFound).
	mockAdp.getDeploymentErr = errors.New("database connection failed")

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments/any-id", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var apiErr models.APIError
	err := json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Equal(t, "InternalError", apiErr.Error)
}

func TestGetNFDeploymentStatus_InternalError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set internal error (not ErrDeploymentNotFound).
	mockAdp.getDeploymentStatusErr = errors.New("backend unavailable")

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments/any-id/status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var apiErr models.APIError
	err := json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Equal(t, "InternalError", apiErr.Error)
}

func TestGetNFDeploymentHistory_InternalError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set internal error (not ErrDeploymentNotFound).
	mockAdp.getDeploymentHistoryErr = errors.New("timeout reading history")

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments/any-id/history", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var apiErr models.APIError
	err := json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Equal(t, "InternalError", apiErr.Error)
}

func TestGetNFDeploymentDescriptor_InternalError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set internal error (not ErrPackageNotFound).
	mockAdp.getPackageErr = errors.New("storage service error")

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeploymentDescriptors/any-id", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var apiErr models.APIError
	err := json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Equal(t, "InternalError", apiErr.Error)
}

func TestUpdateNFDeployment_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	updateReq := models.UpdateNFDeploymentRequest{
		Description: "Updated description",
	}
	body, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/o2dms/v1/nfDeployments/nonexistent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var apiErr models.APIError
	err = json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Equal(t, "NotFound", apiErr.Error)
}

func TestUpdateNFDeployment_InternalError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set internal error (not ErrDeploymentNotFound).
	mockAdp.updateDeploymentErr = errors.New("failed to update in backend")

	updateReq := models.UpdateNFDeploymentRequest{
		Description: "Updated description",
	}
	body, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/o2dms/v1/nfDeployments/any-id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var apiErr models.APIError
	err = json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Equal(t, "InternalError", apiErr.Error)
}

func TestDeleteNFDeployment_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/nfDeployments/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var apiErr models.APIError
	err := json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Equal(t, "NotFound", apiErr.Error)
}

func TestDeleteNFDeployment_InternalError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set internal error (not ErrDeploymentNotFound).
	mockAdp.deleteDeploymentErr = errors.New("failed to uninstall release")

	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/nfDeployments/any-id", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var apiErr models.APIError
	err := json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Equal(t, "InternalError", apiErr.Error)
}

func TestDeleteNFDeploymentDescriptor_NotFound(t *testing.T) {
	handler, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/nfDeploymentDescriptors/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var apiErr models.APIError
	err := json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Equal(t, "NotFound", apiErr.Error)
}

func TestDeleteNFDeploymentDescriptor_InternalError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set internal error (not ErrPackageNotFound).
	mockAdp.deleteDeploymentPkgErr = errors.New("failed to delete from repo")

	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/nfDeploymentDescriptors/any-id", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var apiErr models.APIError
	err := json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Equal(t, "InternalError", apiErr.Error)
}

func TestScaleNFDeployment_NotFound(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set not found error.
	mockAdp.scaleDeploymentErr = adapter.ErrDeploymentNotFound

	scaleReq := models.ScaleNFDeploymentRequest{Replicas: 3}
	body, err := json.Marshal(scaleReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/any-id/scale", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestScaleNFDeployment_InternalError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set internal error (not ErrDeploymentNotFound).
	mockAdp.scaleDeploymentErr = errors.New("kubernetes API error")

	scaleReq := models.ScaleNFDeploymentRequest{Replicas: 3}
	body, err := json.Marshal(scaleReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/any-id/scale", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRollbackNFDeployment_NotFound(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set not found error.
	mockAdp.rollbackErr = adapter.ErrDeploymentNotFound

	rollbackReq := models.RollbackNFDeploymentRequest{}
	body, err := json.Marshal(rollbackReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/any-id/rollback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRollbackNFDeployment_InternalError(t *testing.T) {
	handler, mockAdp := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Set internal error (not ErrDeploymentNotFound).
	mockAdp.rollbackErr = errors.New("helm rollback failed")

	rollbackReq := models.RollbackNFDeploymentRequest{}
	body, err := json.Marshal(rollbackReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/nfDeployments/any-id/rollback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestConvertDeploymentStatus tests deployment status conversion.
func TestConvertDeploymentStatus(t *testing.T) {
	tests := []struct {
		name   string
		status adapter.DeploymentStatus
		want   models.NFDeploymentStatus
	}{
		{
			name:   "pending",
			status: adapter.DeploymentStatusPending,
			want:   models.NFDeploymentStatusPending,
		},
		{
			name:   "deploying",
			status: adapter.DeploymentStatusDeploying,
			want:   models.NFDeploymentStatusInstantiating,
		},
		{
			name:   "deployed",
			status: adapter.DeploymentStatusDeployed,
			want:   models.NFDeploymentStatusDeployed,
		},
		{
			name:   "failed",
			status: adapter.DeploymentStatusFailed,
			want:   models.NFDeploymentStatusFailed,
		},
		{
			name:   "rolling back",
			status: adapter.DeploymentStatusRollingBack,
			want:   models.NFDeploymentStatusUpdating,
		},
		{
			name:   "deleting",
			status: adapter.DeploymentStatusDeleting,
			want:   models.NFDeploymentStatusTerminating,
		},
		{
			name:   "unknown status",
			status: adapter.DeploymentStatus("unknown"),
			want:   models.NFDeploymentStatus("unknown"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handlers.ConvertDeploymentStatus(tt.status)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestConvertToNFDeployment tests NFDeployment conversion.
func TestConvertToNFDeployment(t *testing.T) {
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		deployment *adapter.Deployment
		wantNil    bool
	}{
		{
			name: "full deployment",
			deployment: &adapter.Deployment{
				ID:        "deploy-1",
				Name:      "my-deployment",
				Status:    adapter.DeploymentStatusDeployed,
				PackageID: "pkg-1",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
			wantNil: false,
		},
		{
			name: "deployment with description",
			deployment: &adapter.Deployment{
				ID:          "deploy-2",
				Name:        "test-deploy",
				Status:      adapter.DeploymentStatusPending,
				Description: "Test deployment",
			},
			wantNil: false,
		},
		{
			name:       "nil deployment",
			deployment: nil,
			wantNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handlers.ConvertToNFDeployment(tt.deployment)
			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.deployment.ID, got.NFDeploymentID)
				assert.Equal(t, tt.deployment.Name, got.Name)
				assert.Equal(t, tt.deployment.Description, got.Description)
			}
		})
	}
}

// TestConvertToNFDeploymentDescriptor tests NFDeploymentDescriptor conversion.
func TestConvertToNFDeploymentDescriptor(t *testing.T) {
	uploadedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		pkg     *adapter.DeploymentPackage
		wantNil bool
	}{
		{
			name: "full package",
			pkg: &adapter.DeploymentPackage{
				ID:          "pkg-1",
				Name:        "my-package",
				Description: "Test package",
				Version:     "1.0.0",
				UploadedAt:  uploadedAt,
			},
			wantNil: false,
		},
		{
			name: "minimal package",
			pkg: &adapter.DeploymentPackage{
				ID:   "pkg-2",
				Name: "minimal",
			},
			wantNil: false,
		},
		{
			name:    "nil package",
			pkg:     nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handlers.ConvertToNFDeploymentDescriptor(tt.pkg)
			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.pkg.ID, got.NFDeploymentDescriptorID)
				assert.Equal(t, tt.pkg.Name, got.Name)
				assert.Equal(t, tt.pkg.Description, got.Description)
			}
		})
	}
}
