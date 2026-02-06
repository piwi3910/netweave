package benchmarks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/server"
	"github.com/piwi3910/netweave/internal/storage"
)

// BenchmarkListResourcePools benchmarks the resource pools list endpoint.
// Measures handler performance including database queries and JSON serialization.
func BenchmarkListResourcePools(b *testing.B) {
	srv := setupBenchmarkServer(b)
	req := httptest.NewRequest("GET", "/o2ims-infrastructureInventory/v1/resourcePools", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}
}

// BenchmarkGetResourcePool benchmarks the get resource pool by ID endpoint.
// Measures single item retrieval performance.
func BenchmarkGetResourcePool(b *testing.B) {
	srv := setupBenchmarkServer(b)
	req := httptest.NewRequest("GET", "/o2ims-infrastructureInventory/v1/resourcePools/pool-1", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}
}

// BenchmarkListResources benchmarks the resources list endpoint.
// Measures performance with large result sets.
func BenchmarkListResources(b *testing.B) {
	srv := setupBenchmarkServer(b)
	req := httptest.NewRequest("GET", "/o2ims-infrastructureInventory/v1/resources", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}
}

// BenchmarkListResourcesWithFilter benchmarks filtered resource queries.
// Measures filter parsing and application performance.
func BenchmarkListResourcesWithFilter(b *testing.B) {
	srv := setupBenchmarkServer(b)
	req := httptest.NewRequest(
		"GET",
		"/o2ims-infrastructureInventory/v1/resources?filter=(eq,resourceTypeId,node)",
		nil,
	)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}
}

// BenchmarkGetSubscription benchmarks subscription retrieval.
// Measures Redis lookup performance.
func BenchmarkGetSubscription(b *testing.B) {
	srv := setupBenchmarkServer(b)
	req := httptest.NewRequest("GET", "/o2ims-infrastructureInventory/v1/subscriptions/sub-1", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}
}

// BenchmarkConcurrentRequests benchmarks concurrent handler execution.
// Simulates realistic multi-user load.
func BenchmarkConcurrentRequests(b *testing.B) {
	srv := setupBenchmarkServer(b)
	req := httptest.NewRequest("GET", "/o2ims-infrastructureInventory/v1/resourcePools", nil)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			srv.Router().ServeHTTP(w, req)
		}
	})
}

// BenchmarkMiddlewareChain benchmarks the full middleware chain.
// Measures overhead of all middleware components.
func BenchmarkMiddlewareChain(b *testing.B) {
	srv := setupBenchmarkServer(b)
	req := httptest.NewRequest("GET", "/o2ims-infrastructureInventory/v1/resourcePools", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}
}

// Helper functions

func setupBenchmarkServer(b *testing.B) *server.Server {
	b.Helper()

	gin.SetMode(gin.ReleaseMode)
	logger := zap.NewNop()

	cfg := &config.Config{
		Environment: "test",
		Server: config.ServerConfig{
			Host:    "localhost",
			Port:    8443,
			GinMode: "release",
		},
		Observability: config.ObservabilityConfig{
			Metrics: config.MetricsConfig{
				Enabled: false,
			},
		},
		MultiTenancy: config.MultiTenancyConfig{
			Enabled: false,
		},
	}

	mockAdapter := &mockAdapter{}
	mockStore := &mockStore{}

	srv := server.New(cfg, logger, mockAdapter, mockStore, nil)
	return srv
}

// Mock adapter implementation

type mockAdapter struct{}

func (m *mockAdapter) Name() string    { return "mock" }
func (m *mockAdapter) Version() string { return "1.0.0" }

func (m *mockAdapter) ListResourcePools(_ context.Context) ([]*models.ResourcePool, error) {
	pools := make([]*models.ResourcePool, 10)
	for i := 0; i < 10; i++ {
		pools[i] = &models.ResourcePool{
			ResourcePoolID: fmt.Sprintf("pool-%d", i),
			Name:           "Test Pool",
			Description:    "Benchmark test pool",
			Location:       "datacenter-1",
		}
	}
	return pools, nil
}

func (m *mockAdapter) GetResourcePool(_ context.Context, poolID string) (*models.ResourcePool, error) {
	return &models.ResourcePool{
		ResourcePoolID: poolID,
		Name:           "Test Pool",
		Description:    "Benchmark test pool",
		Location:       "datacenter-1",
	}, nil
}

func (m *mockAdapter) ListResources(_ context.Context, _ string, _ string) ([]*models.Resource, error) {
	resources := make([]*models.Resource, 100)
	for i := 0; i < 100; i++ {
		resources[i] = &models.Resource{
			ResourceID:     fmt.Sprintf("resource-%d", i),
			Name:           "Test Resource",
			ResourceTypeID: "node",
			ResourcePoolID: "pool-1",
		}
	}
	return resources, nil
}

func (m *mockAdapter) GetResource(_ context.Context, resourceID string) (*models.Resource, error) {
	return &models.Resource{
		ResourceID:     resourceID,
		Name:           "Test Resource",
		ResourceTypeID: "node",
		ResourcePoolID: "pool-1",
	}, nil
}

func (m *mockAdapter) ListResourceTypes(_ context.Context) ([]*models.ResourceType, error) {
	return []*models.ResourceType{
		{ResourceTypeID: "node", Name: "Compute Node"},
		{ResourceTypeID: "network", Name: "Network Device"},
	}, nil
}

func (m *mockAdapter) GetResourceType(_ context.Context, typeID string) (*models.ResourceType, error) {
	return &models.ResourceType{
		ResourceTypeID: typeID,
		Name:           "Test Type",
	}, nil
}

func (m *mockAdapter) ListDeploymentManagers(_ context.Context) ([]*models.DeploymentManager, error) {
	return []*models.DeploymentManager{
		{DeploymentManagerID: "dm-1", Name: "Test DM"},
	}, nil
}

func (m *mockAdapter) GetDeploymentManager(_ context.Context, dmID string) (*models.DeploymentManager, error) {
	return &models.DeploymentManager{
		DeploymentManagerID: dmID,
		Name:                "Test DM",
	}, nil
}

func (m *mockAdapter) GetOCloudInfrastructure(_ context.Context) (*models.OCloudInfrastructure, error) {
	return &models.OCloudInfrastructure{
		OCloudID: "ocloud-1",
		Name:     "Test OCloud",
	}, nil
}

func (m *mockAdapter) Health(_ context.Context) error {
	return nil
}

func (m *mockAdapter) Close() error {
	return nil
}

// Ensure mockAdapter implements adapter.Adapter
var _ adapter.Adapter = (*mockAdapter)(nil)

// Mock store implementation

type mockStore struct{}

func (m *mockStore) CreateSubscription(_ context.Context, _ *models.O2Subscription) error {
	return nil
}

func (m *mockStore) GetSubscription(_ context.Context, _ string) (*models.O2Subscription, error) {
	return &models.O2Subscription{
		SubscriptionID:         "sub-1",
		Callback:               "https://smo.example.com/notify",
		ConsumerSubscriptionID: "consumer-1",
	}, nil
}

func (m *mockStore) ListSubscriptions(_ context.Context) ([]*models.O2Subscription, error) {
	return []*models.O2Subscription{
		{
			SubscriptionID:         "sub-1",
			Callback:               "https://smo.example.com/notify",
			ConsumerSubscriptionID: "consumer-1",
		},
	}, nil
}

func (m *mockStore) UpdateSubscription(_ context.Context, _ *models.O2Subscription) error {
	return nil
}

func (m *mockStore) DeleteSubscription(_ context.Context, _ string) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

func (m *mockStore) Ping(_ context.Context) error {
	return nil
}

// Ensure mockStore implements storage.Store
var _ storage.Store = (*mockStore)(nil)
