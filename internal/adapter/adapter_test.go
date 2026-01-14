// Package adapter provides tests for the adapter interface and types.
package adapter_test

import (
	"testing"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilityConstants(t *testing.T) {
	tests := []struct {
		name       string
		capability adapter.Capability
		expected   string
	}{
		{
			name:       "resource pools capability",
			capability: adapter.CapabilityResourcePools,
			expected:   "resource-pools",
		},
		{
			name:       "resources capability",
			capability: adapter.CapabilityResources,
			expected:   "resources",
		},
		{
			name:       "resource types capability",
			capability: adapter.CapabilityResourceTypes,
			expected:   "resource-types",
		},
		{
			name:       "deployment managers capability",
			capability: adapter.CapabilityDeploymentManagers,
			expected:   "deployment-managers",
		},
		{
			name:       "subscriptions capability",
			capability: adapter.CapabilitySubscriptions,
			expected:   "subscriptions",
		},
		{
			name:       "metrics capability",
			capability: adapter.CapabilityMetrics,
			expected:   "metrics",
		},
		{
			name:       "health checks capability",
			capability: adapter.CapabilityHealthChecks,
			expected:   "health-checks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.capability))
		})
	}
}

func TestFilterCreation(t *testing.T) {
	tests := []struct {
		name           string
		filter         *adapter.Filter
		expectedPoolID string
		expectedTypeID string
		expectedLoc    string
		expectedLimit  int
		expectedOffset int
		hasLabels      bool
		hasExtensions  bool
	}{
		{
			name:           "empty filter",
			filter:         &adapter.Filter{},
			expectedPoolID: "",
			expectedTypeID: "",
			expectedLoc:    "",
			expectedLimit:  0,
			expectedOffset: 0,
			hasLabels:      false,
			hasExtensions:  false,
		},
		{
			name: "filter with resource pool ID",
			filter: &adapter.Filter{
				ResourcePoolID: "pool-123",
			},
			expectedPoolID: "pool-123",
			expectedTypeID: "",
			expectedLoc:    "",
			expectedLimit:  0,
			expectedOffset: 0,
			hasLabels:      false,
			hasExtensions:  false,
		},
		{
			name: "filter with resource type ID",
			filter: &adapter.Filter{
				ResourceTypeID: "type-compute",
			},
			expectedPoolID: "",
			expectedTypeID: "type-compute",
			expectedLoc:    "",
			expectedLimit:  0,
			expectedOffset: 0,
			hasLabels:      false,
			hasExtensions:  false,
		},
		{
			name: "filter with location",
			filter: &adapter.Filter{
				Location: "dc-west-1",
			},
			expectedPoolID: "",
			expectedTypeID: "",
			expectedLoc:    "dc-west-1",
			expectedLimit:  0,
			expectedOffset: 0,
			hasLabels:      false,
			hasExtensions:  false,
		},
		{
			name: "filter with labels",
			filter: &adapter.Filter{
				Labels: map[string]string{
					"environment": "production",
					"tier":        "backend",
				},
			},
			expectedPoolID: "",
			expectedTypeID: "",
			expectedLoc:    "",
			expectedLimit:  0,
			expectedOffset: 0,
			hasLabels:      true,
			hasExtensions:  false,
		},
		{
			name: "filter with extensions",
			filter: &adapter.Filter{
				Extensions: map[string]interface{}{
					"vendor.customField": "value",
				},
			},
			expectedPoolID: "",
			expectedTypeID: "",
			expectedLoc:    "",
			expectedLimit:  0,
			expectedOffset: 0,
			hasLabels:      false,
			hasExtensions:  true,
		},
		{
			name: "filter with pagination",
			filter: &adapter.Filter{
				Limit:  100,
				Offset: 50,
			},
			expectedPoolID: "",
			expectedTypeID: "",
			expectedLoc:    "",
			expectedLimit:  100,
			expectedOffset: 50,
			hasLabels:      false,
			hasExtensions:  false,
		},
		{
			name: "complete filter with all fields",
			filter: &adapter.Filter{
				ResourcePoolID: "pool-456",
				ResourceTypeID: "type-storage",
				Location:       "dc-east-2",
				Labels: map[string]string{
					"app": "netweave",
				},
				Extensions: map[string]interface{}{
					"custom": true,
				},
				Limit:  25,
				Offset: 0,
			},
			expectedPoolID: "pool-456",
			expectedTypeID: "type-storage",
			expectedLoc:    "dc-east-2",
			expectedLimit:  25,
			expectedOffset: 0,
			hasLabels:      true,
			hasExtensions:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.filter)
			assert.Equal(t, tt.expectedPoolID, tt.filter.ResourcePoolID)
			assert.Equal(t, tt.expectedTypeID, tt.filter.ResourceTypeID)
			assert.Equal(t, tt.expectedLoc, tt.filter.Location)
			assert.Equal(t, tt.expectedLimit, tt.filter.Limit)
			assert.Equal(t, tt.expectedOffset, tt.filter.Offset)

			if tt.hasLabels {
				assert.NotNil(t, tt.filter.Labels)
				assert.NotEmpty(t, tt.filter.Labels)
			} else {
				assert.Nil(t, tt.filter.Labels)
			}

			if tt.hasExtensions {
				assert.NotNil(t, tt.filter.Extensions)
				assert.NotEmpty(t, tt.filter.Extensions)
			} else {
				assert.Nil(t, tt.filter.Extensions)
			}
		})
	}
}

func TestDeploymentManagerModel(t *testing.T) {
	tests := []struct {
		name string
		dm   *adapter.DeploymentManager
	}{
		{
			name: "minimal deployment manager",
			dm: &adapter.DeploymentManager{
				DeploymentManagerID: "dm-1",
				Name:                "Primary DM",
				OCloudID:            "ocloud-1",
				ServiceURI:          "https://api.example.com/o2ims",
			},
		},
		{
			name: "complete deployment manager",
			dm: &adapter.DeploymentManager{
				DeploymentManagerID: "dm-2",
				Name:                "Full DM",
				Description:         "Production deployment manager",
				OCloudID:            "ocloud-2",
				ServiceURI:          "https://prod.example.com/o2ims",
				SupportedLocations:  []string{"dc-west-1", "dc-east-1"},
				Capabilities:        []string{"compute", "storage", "network"},
				Extensions: map[string]interface{}{
					"vendor": "kubernetes",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.dm)
			assert.NotEmpty(t, tt.dm.DeploymentManagerID)
			assert.NotEmpty(t, tt.dm.Name)
			assert.NotEmpty(t, tt.dm.OCloudID)
			assert.NotEmpty(t, tt.dm.ServiceURI)
		})
	}
}

func TestResourcePoolModel(t *testing.T) {
	tests := []struct {
		name string
		pool *adapter.ResourcePool
	}{
		{
			name: "minimal resource pool",
			pool: &adapter.ResourcePool{
				ResourcePoolID: "pool-1",
				Name:           "Default Pool",
				OCloudID:       "ocloud-1",
			},
		},
		{
			name: "complete resource pool",
			pool: &adapter.ResourcePool{
				ResourcePoolID:   "pool-2",
				Name:             "Production Pool",
				Description:      "High-performance compute pool",
				Location:         "dc-west-1",
				OCloudID:         "ocloud-1",
				GlobalLocationID: "geo:37.7749,-122.4194",
				Extensions: map[string]interface{}{
					"machineType": "n1-standard-4",
					"replicas":    3,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.pool)
			assert.NotEmpty(t, tt.pool.ResourcePoolID)
			assert.NotEmpty(t, tt.pool.Name)
			assert.NotEmpty(t, tt.pool.OCloudID)
		})
	}
}

func TestResourceModel(t *testing.T) {
	tests := []struct {
		name     string
		resource *adapter.Resource
	}{
		{
			name: "minimal resource",
			resource: &adapter.Resource{
				ResourceID:     "res-1",
				ResourceTypeID: "type-compute",
			},
		},
		{
			name: "complete resource",
			resource: &adapter.Resource{
				ResourceID:     "res-2",
				ResourceTypeID: "type-compute",
				ResourcePoolID: "pool-1",
				GlobalAssetID:  "urn:o-ran:resource:node-01",
				Description:    "Worker node 01",
				Extensions: map[string]interface{}{
					"nodeName": "worker-01",
					"status":   "Ready",
					"cpu":      "8",
					"memory":   "32Gi",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.resource)
			assert.NotEmpty(t, tt.resource.ResourceID)
			assert.NotEmpty(t, tt.resource.ResourceTypeID)
		})
	}
}

func TestResourceTypeModel(t *testing.T) {
	tests := []struct {
		name         string
		resourceType *adapter.ResourceType
	}{
		{
			name: "minimal resource type",
			resourceType: &adapter.ResourceType{
				ResourceTypeID: "type-1",
				Name:           "Compute Node",
			},
		},
		{
			name: "complete resource type",
			resourceType: &adapter.ResourceType{
				ResourceTypeID: "type-2",
				Name:           "High Memory Compute",
				Description:    "High memory compute nodes for data processing",
				Vendor:         "Dell",
				Model:          "PowerEdge R740",
				Version:        "1.0",
				ResourceClass:  "compute",
				ResourceKind:   "physical",
				Extensions: map[string]interface{}{
					"cores":  64,
					"memory": "512Gi",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.resourceType)
			assert.NotEmpty(t, tt.resourceType.ResourceTypeID)
			assert.NotEmpty(t, tt.resourceType.Name)
		})
	}
}

func TestSubscriptionModel(t *testing.T) {
	tests := []struct {
		name string
		sub  *adapter.Subscription
	}{
		{
			name: "minimal subscription",
			sub: &adapter.Subscription{
				SubscriptionID: "sub-1",
				Callback:       "https://smo.example.com/notify",
			},
		},
		{
			name: "subscription with consumer ID",
			sub: &adapter.Subscription{
				SubscriptionID:         "sub-2",
				Callback:               "https://smo.example.com/notify",
				ConsumerSubscriptionID: "consumer-sub-123",
			},
		},
		{
			name: "subscription with filter",
			sub: &adapter.Subscription{
				SubscriptionID: "sub-3",
				Callback:       "https://smo.example.com/notify",
				Filter: &adapter.SubscriptionFilter{
					ResourcePoolID: "pool-1",
					ResourceTypeID: "type-compute",
				},
			},
		},
		{
			name: "complete subscription",
			sub: &adapter.Subscription{
				SubscriptionID:         "sub-4",
				Callback:               "https://smo.example.com/notify",
				ConsumerSubscriptionID: "consumer-sub-456",
				Filter: &adapter.SubscriptionFilter{
					ResourcePoolID: "pool-2",
					ResourceTypeID: "type-storage",
					ResourceID:     "res-specific",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.sub)
			assert.NotEmpty(t, tt.sub.SubscriptionID)
			assert.NotEmpty(t, tt.sub.Callback)
		})
	}
}

func TestSubscriptionFilterModel(t *testing.T) {
	tests := []struct {
		name           string
		filter         *adapter.SubscriptionFilter
		expectedPoolID string
		expectedTypeID string
		expectedResID  string
	}{
		{
			name:           "empty filter",
			filter:         &adapter.SubscriptionFilter{},
			expectedPoolID: "",
			expectedTypeID: "",
			expectedResID:  "",
		},
		{
			name: "filter by resource pool",
			filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-1",
			},
			expectedPoolID: "pool-1",
			expectedTypeID: "",
			expectedResID:  "",
		},
		{
			name: "filter by resource type",
			filter: &adapter.SubscriptionFilter{
				ResourceTypeID: "type-compute",
			},
			expectedPoolID: "",
			expectedTypeID: "type-compute",
			expectedResID:  "",
		},
		{
			name: "filter by resource",
			filter: &adapter.SubscriptionFilter{
				ResourceID: "res-123",
			},
			expectedPoolID: "",
			expectedTypeID: "",
			expectedResID:  "res-123",
		},
		{
			name: "combined filter",
			filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-1",
				ResourceTypeID: "type-compute",
				ResourceID:     "res-456",
			},
			expectedPoolID: "pool-1",
			expectedTypeID: "type-compute",
			expectedResID:  "res-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.filter)
			assert.Equal(t, tt.expectedPoolID, tt.filter.ResourcePoolID)
			assert.Equal(t, tt.expectedTypeID, tt.filter.ResourceTypeID)
			assert.Equal(t, tt.expectedResID, tt.filter.ResourceID)
		})
	}
}

func TestCapabilityUniqueness(t *testing.T) {
	capabilities := []adapter.Capability{
		adapter.CapabilityResourcePools,
		adapter.CapabilityResources,
		adapter.CapabilityResourceTypes,
		adapter.CapabilityDeploymentManagers,
		adapter.CapabilitySubscriptions,
		adapter.CapabilityMetrics,
		adapter.CapabilityHealthChecks,
	}

	seen := make(map[adapter.Capability]bool)
	for _, cap := range capabilities {
		if seen[cap] {
			t.Errorf("duplicate capability found: %s", cap)
		}
		seen[cap] = true
	}

	assert.Len(t, seen, 7, "expected 7 unique capabilities")
}
