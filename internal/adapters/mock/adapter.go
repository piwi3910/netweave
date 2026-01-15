// Package mock provides a mock O2-IMS adapter with realistic sample data.
// This adapter is designed for:
// - Local development and testing without real infrastructure
// - E2E testing in CI pipelines
// - API demonstrations and documentation
//
// The mock adapter stores all data in memory and includes realistic sample data
// for resource pools, resources, resource types, and subscriptions.
package mock

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/adapter"
)

// Adapter is a mock implementation of the O2-IMS adapter interface.
// It stores all data in memory and provides deterministic responses for testing.
type Adapter struct {
	mu                sync.RWMutex
	resourcePools     map[string]*adapter.ResourcePool
	resources         map[string]*adapter.Resource
	resourceTypes     map[string]*adapter.ResourceType
	subscriptions     map[string]*adapter.Subscription
	deploymentManager *adapter.DeploymentManager
}

// NewAdapter creates a new mock adapter with sample data.
// Pass populateSampleData=true to pre-populate with realistic test data.
func NewAdapter(populateSampleData bool) *Adapter {
	a := &Adapter{
		resourcePools:     make(map[string]*adapter.ResourcePool),
		resources:         make(map[string]*adapter.Resource),
		resourceTypes:     make(map[string]*adapter.ResourceType),
		subscriptions:     make(map[string]*adapter.Subscription),
		deploymentManager: createMockDeploymentManager(),
	}

	if populateSampleData {
		a.populateSampleData()
	}

	return a
}

// createMockDeploymentManager creates a mock deployment manager.
func createMockDeploymentManager() *adapter.DeploymentManager {
	return &adapter.DeploymentManager{
		DeploymentManagerID: "mock-dm-001",
		Name:                "Mock O2-IMS Deployment Manager",
		Description:         "Mock deployment manager for development and testing",
		OCloudID:            "mock-ocloud-01",
		ServiceURI:          "http://localhost:8080/o2ims",
		SupportedLocations:  []string{"us-east-1", "us-west-2", "eu-central-1"},
		Capabilities: []string{
			string(adapter.CapabilityResourcePools),
			string(adapter.CapabilityResources),
			string(adapter.CapabilityResourceTypes),
			string(adapter.CapabilitySubscriptions),
		},
		Extensions: map[string]interface{}{
			"vendor":      "Mock Corp",
			"version":     "1.0.0",
			"environment": "test",
		},
	}
}

// populateSampleData adds realistic sample data for testing.
func (a *Adapter) populateSampleData() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Create resource types first
	cpuType := &adapter.ResourceType{
		ResourceTypeID: "rt-cpu-001",
		Name:           "CPU",
		Description:    "CPU cores for compute workloads",
		Vendor:         "Intel",
		Model:          "Xeon Gold 6248R",
		Version:        "1.0",
		ResourceClass:  "compute",
		ResourceKind:   "physical",
		Extensions: map[string]interface{}{
			"cores":       48,
			"threads":     96,
			"base_clock":  "3.0 GHz",
			"turbo_clock": "4.0 GHz",
		},
	}
	a.resourceTypes[cpuType.ResourceTypeID] = cpuType

	gpuType := &adapter.ResourceType{
		ResourceTypeID: "rt-gpu-001",
		Name:           "GPU",
		Description:    "NVIDIA GPU for AI/ML workloads",
		Vendor:         "NVIDIA",
		Model:          "A100",
		Version:        "1.0",
		ResourceClass:  "accelerator",
		ResourceKind:   "physical",
		Extensions: map[string]interface{}{
			"memory":       "40GB",
			"cuda_cores":   6912,
			"tensor_cores": 432,
			"bandwidth":    "1555 GB/s",
		},
	}
	a.resourceTypes[gpuType.ResourceTypeID] = gpuType

	memoryType := &adapter.ResourceType{
		ResourceTypeID: "rt-memory-001",
		Name:           "Memory",
		Description:    "System memory for compute nodes",
		Vendor:         "Samsung",
		Model:          "DDR4 ECC",
		Version:        "1.0",
		ResourceClass:  "memory",
		ResourceKind:   "physical",
		Extensions: map[string]interface{}{
			"capacity": "512GB",
			"speed":    "3200 MHz",
			"type":     "DDR4",
			"ecc":      true,
		},
	}
	a.resourceTypes[memoryType.ResourceTypeID] = memoryType

	storageType := &adapter.ResourceType{
		ResourceTypeID: "rt-storage-001",
		Name:           "NVMe Storage",
		Description:    "High-speed NVMe storage",
		Vendor:         "Samsung",
		Model:          "PM9A3",
		Version:        "1.0",
		ResourceClass:  "storage",
		ResourceKind:   "physical",
		Extensions: map[string]interface{}{
			"capacity":   "3.84TB",
			"interface":  "NVMe",
			"read_iops":  "800000",
			"write_iops": "120000",
		},
	}
	a.resourceTypes[storageType.ResourceTypeID] = storageType

	// Create resource pools
	usEast1Pool := &adapter.ResourcePool{
		ResourcePoolID:   "pool-us-east-1",
		OCloudID:         "mock-ocloud-01",
		Name:             "US East Compute Pool",
		Description:      "Compute resources in US East region",
		Location:         "us-east-1",
		GlobalLocationID: "us-east-1a",
		Extensions: map[string]interface{}{
			"datacenter": "DC-US-EAST-1A",
			"rack":       "R01",
		},
	}
	a.resourcePools[usEast1Pool.ResourcePoolID] = usEast1Pool

	usWest2Pool := &adapter.ResourcePool{
		ResourcePoolID:   "pool-us-west-2",
		OCloudID:         "mock-ocloud-01",
		Name:             "US West GPU Pool",
		Description:      "GPU resources for AI/ML workloads",
		Location:         "us-west-2",
		GlobalLocationID: "us-west-2b",
		Extensions: map[string]interface{}{
			"datacenter": "DC-US-WEST-2B",
			"rack":       "R05",
			"gpu_count":  32,
		},
	}
	a.resourcePools[usWest2Pool.ResourcePoolID] = usWest2Pool

	euCentralPool := &adapter.ResourcePool{
		ResourcePoolID:   "pool-eu-central-1",
		OCloudID:         "mock-ocloud-01",
		Name:             "EU Central Pool",
		Description:      "European compute and storage resources",
		Location:         "eu-central-1",
		GlobalLocationID: "eu-central-1c",
		Extensions: map[string]interface{}{
			"datacenter": "DC-EU-CENTRAL-1C",
			"rack":       "R03",
		},
	}
	a.resourcePools[euCentralPool.ResourcePoolID] = euCentralPool

	// Create resources
	// US East resources
	for i := 1; i <= 5; i++ {
		resource := &adapter.Resource{
			ResourceID:     fmt.Sprintf("res-cpu-us-east-%03d", i),
			ResourceTypeID: cpuType.ResourceTypeID,
			ResourcePoolID: usEast1Pool.ResourcePoolID,
			GlobalAssetID:  uuid.New().String(),
			Description:    fmt.Sprintf("CPU server node %d in US East", i),
			Extensions: map[string]interface{}{
				"hostname": fmt.Sprintf("node-us-east-%03d", i),
				"status":   "active",
			},
		}
		a.resources[resource.ResourceID] = resource
	}

	// US West GPU resources
	for i := 1; i <= 8; i++ {
		resource := &adapter.Resource{
			ResourceID:     fmt.Sprintf("res-gpu-us-west-%03d", i),
			ResourceTypeID: gpuType.ResourceTypeID,
			ResourcePoolID: usWest2Pool.ResourcePoolID,
			GlobalAssetID:  uuid.New().String(),
			Description:    fmt.Sprintf("NVIDIA A100 GPU %d in US West", i),
			Extensions: map[string]interface{}{
				"hostname":    fmt.Sprintf("gpu-node-us-west-%03d", i),
				"status":      "active",
				"temperature": 65.0 + float64(i),
				"utilization": 45.0 + float64(i)*5,
			},
		}
		a.resources[resource.ResourceID] = resource
	}

	// EU Central mixed resources
	for i := 1; i <= 3; i++ {
		cpuResource := &adapter.Resource{
			ResourceID:     fmt.Sprintf("res-cpu-eu-central-%03d", i),
			ResourceTypeID: cpuType.ResourceTypeID,
			ResourcePoolID: euCentralPool.ResourcePoolID,
			GlobalAssetID:  uuid.New().String(),
			Description:    fmt.Sprintf("CPU server node %d in EU Central", i),
			Extensions: map[string]interface{}{
				"hostname": fmt.Sprintf("node-eu-central-%03d", i),
				"status":   "active",
			},
		}
		a.resources[cpuResource.ResourceID] = cpuResource

		storageResource := &adapter.Resource{
			ResourceID:     fmt.Sprintf("res-storage-eu-central-%03d", i),
			ResourceTypeID: storageType.ResourceTypeID,
			ResourcePoolID: euCentralPool.ResourcePoolID,
			GlobalAssetID:  uuid.New().String(),
			Description:    fmt.Sprintf("NVMe storage array %d in EU Central", i),
			Extensions: map[string]interface{}{
				"hostname":     fmt.Sprintf("storage-eu-central-%03d", i),
				"status":       "active",
				"used_space":   "1.5TB",
				"free_space":   "2.34TB",
				"health_score": 98,
			},
		}
		a.resources[storageResource.ResourceID] = storageResource
	}
}

// Metadata implementation

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "mock"
}

// Version returns the adapter version.
func (a *Adapter) Version() string {
	return "1.0.0"
}

// Capabilities returns the list of supported capabilities.
func (a *Adapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityResourcePools,
		adapter.CapabilityResources,
		adapter.CapabilityResourceTypes,
		adapter.CapabilitySubscriptions,
	}
}

// DeploymentManagerClient implementation

// GetDeploymentManager retrieves the mock deployment manager.
func (a *Adapter) GetDeploymentManager(ctx context.Context, id string) (*adapter.DeploymentManager, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if id != a.deploymentManager.DeploymentManagerID && id != "default" {
		return nil, fmt.Errorf("deployment manager not found: %s", id)
	}

	return a.deploymentManager, nil
}

// ResourcePoolClient implementation

// ListResourcePools retrieves all resource pools matching the filter.
func (a *Adapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var pools []*adapter.ResourcePool
	for _, pool := range a.resourcePools {
		if a.matchesPoolFilter(pool, filter) {
			pools = append(pools, pool)
		}
	}

	return a.applyPagination(pools, filter), nil
}

// GetResourcePool retrieves a specific resource pool by ID.
func (a *Adapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	pool, ok := a.resourcePools[id]
	if !ok {
		return nil, fmt.Errorf("resource pool not found: %s", id)
	}

	return pool, nil
}

// CreateResourcePool creates a new resource pool.
func (a *Adapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if pool.ResourcePoolID == "" {
		pool.ResourcePoolID = fmt.Sprintf("pool-%s", uuid.New().String()[:8])
	}

	if pool.OCloudID == "" {
		pool.OCloudID = a.deploymentManager.OCloudID
	}

	a.resourcePools[pool.ResourcePoolID] = pool
	return pool, nil
}

// UpdateResourcePool updates an existing resource pool.
func (a *Adapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.resourcePools[id]; !ok {
		return nil, fmt.Errorf("resource pool not found: %s", id)
	}

	pool.ResourcePoolID = id
	a.resourcePools[id] = pool
	return pool, nil
}

// DeleteResourcePool deletes a resource pool by ID.
func (a *Adapter) DeleteResourcePool(ctx context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.resourcePools[id]; !ok {
		return fmt.Errorf("resource pool not found: %s", id)
	}

	// Check for dependent resources
	for _, resource := range a.resources {
		if resource.ResourcePoolID == id {
			return fmt.Errorf("cannot delete pool with existing resources")
		}
	}

	delete(a.resourcePools, id)
	return nil
}

// ResourceClient implementation

// ListResources retrieves all resources matching the filter.
func (a *Adapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var resources []*adapter.Resource
	for _, resource := range a.resources {
		if a.matchesResourceFilter(resource, filter) {
			resources = append(resources, resource)
		}
	}

	return a.applyPaginationResources(resources, filter), nil
}

// GetResource retrieves a specific resource by ID.
func (a *Adapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	resource, ok := a.resources[id]
	if !ok {
		return nil, fmt.Errorf("resource not found: %s", id)
	}

	return resource, nil
}

// CreateResource creates a new resource.
func (a *Adapter) CreateResource(ctx context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if resource.ResourceID == "" {
		resource.ResourceID = fmt.Sprintf("res-%s", uuid.New().String()[:8])
	}

	if resource.GlobalAssetID == "" {
		resource.GlobalAssetID = uuid.New().String()
	}

	a.resources[resource.ResourceID] = resource
	return resource, nil
}

// UpdateResource updates an existing resource.
func (a *Adapter) UpdateResource(ctx context.Context, id string, resource *adapter.Resource) (*adapter.Resource, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.resources[id]; !ok {
		return nil, fmt.Errorf("resource not found: %s", id)
	}

	resource.ResourceID = id
	a.resources[id] = resource
	return resource, nil
}

// DeleteResource deletes a resource by ID.
func (a *Adapter) DeleteResource(ctx context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.resources[id]; !ok {
		return fmt.Errorf("resource not found: %s", id)
	}

	delete(a.resources, id)
	return nil
}

// ResourceTypeClient implementation

// ListResourceTypes retrieves all resource types matching the filter.
func (a *Adapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourceType, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var resourceTypes []*adapter.ResourceType
	for _, rt := range a.resourceTypes {
		resourceTypes = append(resourceTypes, rt)
	}

	return a.applyPaginationResourceTypes(resourceTypes, filter), nil
}

// GetResourceType retrieves a specific resource type by ID.
func (a *Adapter) GetResourceType(ctx context.Context, id string) (*adapter.ResourceType, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	rt, ok := a.resourceTypes[id]
	if !ok {
		return nil, fmt.Errorf("resource type not found: %s", id)
	}

	return rt, nil
}

// SubscriptionClient implementation

// ListSubscriptions retrieves all subscriptions matching the filter.
func (a *Adapter) ListSubscriptions(ctx context.Context, filter *adapter.SubscriptionFilter) ([]*adapter.Subscription, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var subscriptions []*adapter.Subscription
	for _, sub := range a.subscriptions {
		subscriptions = append(subscriptions, sub)
	}

	return subscriptions, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *Adapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	sub, ok := a.subscriptions[id]
	if !ok {
		return nil, fmt.Errorf("subscription not found: %s", id)
	}

	return sub, nil
}

// CreateSubscription creates a new subscription.
func (a *Adapter) CreateSubscription(ctx context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if sub.SubscriptionID == "" {
		sub.SubscriptionID = uuid.New().String()
	}

	a.subscriptions[sub.SubscriptionID] = sub
	return sub, nil
}

// UpdateSubscription updates an existing subscription.
func (a *Adapter) UpdateSubscription(ctx context.Context, id string, sub *adapter.Subscription) (*adapter.Subscription, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.subscriptions[id]; !ok {
		return nil, fmt.Errorf("subscription not found: %s", id)
	}

	sub.SubscriptionID = id
	a.subscriptions[id] = sub
	return sub, nil
}

// DeleteSubscription deletes a subscription by ID.
func (a *Adapter) DeleteSubscription(ctx context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.subscriptions[id]; !ok {
		return fmt.Errorf("subscription not found: %s", id)
	}

	delete(a.subscriptions, id)
	return nil
}

// Lifecycle implementation

// Initialize performs any necessary initialization.
func (a *Adapter) Initialize(ctx context.Context) error {
	// Mock adapter requires no initialization
	return nil
}

// Health checks the health of the adapter.
func (a *Adapter) Health(ctx context.Context) error {
	// Mock adapter is always healthy
	return nil
}

// Shutdown performs cleanup when the adapter is shutting down.
func (a *Adapter) Shutdown(ctx context.Context) error {
	// Mock adapter requires no cleanup
	return nil
}

// Helper methods for filtering and pagination

func (a *Adapter) matchesPoolFilter(pool *adapter.ResourcePool, filter *adapter.Filter) bool {
	if filter == nil {
		return true
	}

	if filter.Location != "" && pool.Location != filter.Location {
		return false
	}

	if filter.TenantID != "" && pool.TenantID != filter.TenantID {
		return false
	}

	return true
}

func (a *Adapter) matchesResourceFilter(resource *adapter.Resource, filter *adapter.Filter) bool {
	if filter == nil {
		return true
	}

	if filter.ResourcePoolID != "" && resource.ResourcePoolID != filter.ResourcePoolID {
		return false
	}

	if filter.ResourceTypeID != "" && resource.ResourceTypeID != filter.ResourceTypeID {
		return false
	}

	if filter.TenantID != "" && resource.TenantID != filter.TenantID {
		return false
	}

	return true
}

func (a *Adapter) applyPagination(pools []*adapter.ResourcePool, filter *adapter.Filter) []*adapter.ResourcePool {
	if filter == nil {
		return pools
	}

	offset := filter.Offset
	limit := filter.Limit

	if limit == 0 {
		limit = len(pools)
	}

	if offset >= len(pools) {
		return []*adapter.ResourcePool{}
	}

	end := offset + limit
	if end > len(pools) {
		end = len(pools)
	}

	return pools[offset:end]
}

func (a *Adapter) applyPaginationResources(resources []*adapter.Resource, filter *adapter.Filter) []*adapter.Resource {
	if filter == nil {
		return resources
	}

	offset := filter.Offset
	limit := filter.Limit

	if limit == 0 {
		limit = len(resources)
	}

	if offset >= len(resources) {
		return []*adapter.Resource{}
	}

	end := offset + limit
	if end > len(resources) {
		end = len(resources)
	}

	return resources[offset:end]
}

func (a *Adapter) applyPaginationResourceTypes(resourceTypes []*adapter.ResourceType, filter *adapter.Filter) []*adapter.ResourceType {
	if filter == nil {
		return resourceTypes
	}

	offset := filter.Offset
	limit := filter.Limit

	if limit == 0 {
		limit = len(resourceTypes)
	}

	if offset >= len(resourceTypes) {
		return []*adapter.ResourceType{}
	}

	end := offset + limit
	if end > len(resourceTypes) {
		end = len(resourceTypes)
	}

	return resourceTypes[offset:end]
}

// Close cleanly shuts down the adapter and releases resources.
// For the mock adapter, this is a no-op since there are no external connections.
func (a *Adapter) Close() error {
	// Mock adapter has no resources to clean up
	return nil
}
