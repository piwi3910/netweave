package benchmarks

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
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

	adp := &benchMockAdapter{}
	str := &benchMockStore{}

	srv := server.New(cfg, logger, adp, str, nil)
	return srv
}

// Mock adapter implementation for benchmarks.

type benchMockAdapter struct{}

func (m *benchMockAdapter) Name() string                       { return "mock" }
func (m *benchMockAdapter) Version() string                    { return "1.0.0" }
func (m *benchMockAdapter) Capabilities() []adapter.Capability { return nil }

func (m *benchMockAdapter) GetDeploymentManager(_ context.Context, dmID string) (*adapter.DeploymentManager, error) {
	return &adapter.DeploymentManager{
		DeploymentManagerID: dmID,
		Name:                "Test DM",
	}, nil
}

func (m *benchMockAdapter) ListResourcePools(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourcePool, error) {
	pools := make([]*adapter.ResourcePool, 10)
	for i := 0; i < 10; i++ {
		pools[i] = &adapter.ResourcePool{
			ResourcePoolID: fmt.Sprintf("pool-%d", i),
			Name:           "Test Pool",
			Description:    "Benchmark test pool",
			Location:       "datacenter-1",
		}
	}
	return pools, nil
}

func (m *benchMockAdapter) GetResourcePool(_ context.Context, poolID string) (*adapter.ResourcePool, error) {
	return &adapter.ResourcePool{
		ResourcePoolID: poolID,
		Name:           "Test Pool",
		Description:    "Benchmark test pool",
		Location:       "datacenter-1",
	}, nil
}

func (m *benchMockAdapter) CreateResourcePool(_ context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return pool, nil
}

func (m *benchMockAdapter) UpdateResourcePool(_ context.Context, _ string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return pool, nil
}

func (m *benchMockAdapter) DeleteResourcePool(_ context.Context, _ string) error {
	return nil
}

func (m *benchMockAdapter) ListResources(_ context.Context, _ *adapter.Filter) ([]*adapter.Resource, error) {
	resources := make([]*adapter.Resource, 100)
	for i := 0; i < 100; i++ {
		resources[i] = &adapter.Resource{
			ResourceID:     fmt.Sprintf("resource-%d", i),
			ResourceTypeID: "node",
			ResourcePoolID: "pool-1",
		}
	}
	return resources, nil
}

func (m *benchMockAdapter) GetResource(_ context.Context, resourceID string) (*adapter.Resource, error) {
	return &adapter.Resource{
		ResourceID:     resourceID,
		ResourceTypeID: "node",
		ResourcePoolID: "pool-1",
	}, nil
}

func (m *benchMockAdapter) CreateResource(_ context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	return resource, nil
}

func (m *benchMockAdapter) UpdateResource(_ context.Context, _ string, resource *adapter.Resource) (*adapter.Resource, error) {
	return resource, nil
}

func (m *benchMockAdapter) DeleteResource(_ context.Context, _ string) error {
	return nil
}

func (m *benchMockAdapter) ListResourceTypes(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourceType, error) {
	return []*adapter.ResourceType{
		{ResourceTypeID: "node", Name: "Compute Node"},
		{ResourceTypeID: "network", Name: "Network Device"},
	}, nil
}

func (m *benchMockAdapter) GetResourceType(_ context.Context, typeID string) (*adapter.ResourceType, error) {
	return &adapter.ResourceType{
		ResourceTypeID: typeID,
		Name:           "Test Type",
	}, nil
}

func (m *benchMockAdapter) CreateSubscription(_ context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	return sub, nil
}

func (m *benchMockAdapter) GetSubscription(_ context.Context, id string) (*adapter.Subscription, error) {
	return &adapter.Subscription{SubscriptionID: id}, nil
}

func (m *benchMockAdapter) UpdateSubscription(_ context.Context, _ string, sub *adapter.Subscription) (*adapter.Subscription, error) {
	return sub, nil
}

func (m *benchMockAdapter) DeleteSubscription(_ context.Context, _ string) error {
	return nil
}

func (m *benchMockAdapter) Health(_ context.Context) error {
	return nil
}

func (m *benchMockAdapter) Close() error {
	return nil
}

// Ensure benchMockAdapter implements adapter.Adapter.
var _ adapter.Adapter = (*benchMockAdapter)(nil)

// Mock store implementation for benchmarks.

type benchMockStore struct{}

func (m *benchMockStore) Create(_ context.Context, _ *storage.Subscription) error {
	return nil
}

func (m *benchMockStore) Get(_ context.Context, _ string) (*storage.Subscription, error) {
	return &storage.Subscription{
		ID:                     "sub-1",
		Callback:               "https://smo.example.com/notify",
		ConsumerSubscriptionID: "consumer-1",
	}, nil
}

func (m *benchMockStore) List(_ context.Context) ([]*storage.Subscription, error) {
	return []*storage.Subscription{
		{
			ID:                     "sub-1",
			Callback:               "https://smo.example.com/notify",
			ConsumerSubscriptionID: "consumer-1",
		},
	}, nil
}

func (m *benchMockStore) Update(_ context.Context, _ *storage.Subscription) error {
	return nil
}

func (m *benchMockStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *benchMockStore) ListByResourcePool(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *benchMockStore) ListByResourceType(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *benchMockStore) ListByTenant(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *benchMockStore) Close() error {
	return nil
}

func (m *benchMockStore) Ping(_ context.Context) error {
	return nil
}

// Ensure benchMockStore implements storage.Store.
var _ storage.Store = (*benchMockStore)(nil)
