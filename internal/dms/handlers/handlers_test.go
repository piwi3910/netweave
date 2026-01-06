package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/dms/adapter"
	"github.com/piwi3910/netweave/internal/dms/models"
	"github.com/piwi3910/netweave/internal/dms/registry"
	"github.com/piwi3910/netweave/internal/dms/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockAdapter implements the adapter.DMSAdapter interface for testing.
type mockAdapter struct {
	name         string
	version      string
	capabilities []adapter.Capability
	healthy      bool
	healthErr    error

	deployments []*adapter.Deployment
	packages    []*adapter.DeploymentPackage

	createDeploymentErr error
	updateDeploymentErr error
	deleteDeploymentErr error
	scaleDeploymentErr  error
	rollbackErr         error
}

func newMockAdapter() *mockAdapter {
	return &mockAdapter{
		name:         "mock",
		version:      "1.0.0",
		capabilities: []adapter.Capability{adapter.CapabilityDeploymentLifecycle, adapter.CapabilityRollback, adapter.CapabilityScaling},
		healthy:      true,
		deployments:  make([]*adapter.Deployment, 0),
		packages:     make([]*adapter.DeploymentPackage, 0),
	}
}

func (m *mockAdapter) Name() string                       { return m.name }
func (m *mockAdapter) Version() string                    { return m.version }
func (m *mockAdapter) Capabilities() []adapter.Capability { return m.capabilities }

func (m *mockAdapter) ListDeploymentPackages(_ context.Context, _ *adapter.Filter) ([]*adapter.DeploymentPackage, error) {
	return m.packages, nil
}

func (m *mockAdapter) GetDeploymentPackage(_ context.Context, id string) (*adapter.DeploymentPackage, error) {
	for _, pkg := range m.packages {
		if pkg.ID == id {
			return pkg, nil
		}
	}
	return nil, adapter.ErrPackageNotFound
}

func (m *mockAdapter) UploadDeploymentPackage(_ context.Context, pkg *adapter.DeploymentPackageUpload) (*adapter.DeploymentPackage, error) {
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

func (m *mockAdapter) UpdateDeployment(_ context.Context, id string, update *adapter.DeploymentUpdate) (*adapter.Deployment, error) {
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
func setupTestHandler(t *testing.T) (*Handler, *registry.Registry, *mockAdapter) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	reg := registry.NewRegistry(logger, nil)
	mockAdp := newMockAdapter()

	err := reg.Register(context.Background(), "mock", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	store := storage.NewMemoryStore()
	handler := NewHandler(reg, store, logger)

	return handler, reg, mockAdp
}

// setupTestRouter creates a test router with the handler configured.
func setupTestRouter(handler *Handler) *gin.Engine {
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
	handler, _, mockAdp := setupTestHandler(t)
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
	handler, _, mockAdp := setupTestHandler(t)
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
	handler, _, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateNFDeployment(t *testing.T) {
	handler, _, _ := setupTestHandler(t)
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
	handler, _, _ := setupTestHandler(t)
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
	handler, _, mockAdp := setupTestHandler(t)
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
	handler, _, mockAdp := setupTestHandler(t)
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
	handler, _, mockAdp := setupTestHandler(t)
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
	handler, _, mockAdp := setupTestHandler(t)
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
	handler, _, mockAdp := setupTestHandler(t)
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
	handler, _, mockAdp := setupTestHandler(t)
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
	handler, _, mockAdp := setupTestHandler(t)
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
	handler, _, _ := setupTestHandler(t)
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
	handler, _, _ := setupTestHandler(t)
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
	handler, _, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	createReq := models.CreateDMSSubscriptionRequest{
		Callback:               "https://example.com/webhook",
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

	assert.Equal(t, http.StatusCreated, w.Code)

	var subscription models.DMSSubscription
	err = json.Unmarshal(w.Body.Bytes(), &subscription)
	require.NoError(t, err)

	assert.NotEmpty(t, subscription.SubscriptionID)
	assert.Equal(t, "https://example.com/webhook", subscription.Callback)
	assert.Equal(t, "consumer-123", subscription.ConsumerSubscriptionID)
}

func TestGetDMSSubscription(t *testing.T) {
	handler, _, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	// First create a subscription.
	createReq := models.CreateDMSSubscriptionRequest{
		Callback: "https://example.com/webhook",
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
	handler, _, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	// First create a subscription.
	createReq := models.CreateDMSSubscriptionRequest{
		Callback: "https://example.com/webhook",
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
	handler, _, _ := setupTestHandler(t)
	router := setupTestRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/subscriptions/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Deployment Lifecycle Info Test

func TestGetDeploymentLifecycleInfo(t *testing.T) {
	handler, _, _ := setupTestHandler(t)
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
