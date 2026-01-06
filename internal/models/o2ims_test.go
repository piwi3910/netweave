// Package models contains the O2-IMS data models for the netweave gateway.
package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDeploymentManager_JSONSerialization(t *testing.T) {
	tests := []struct {
		name string
		dm   DeploymentManager
	}{
		{
			name: "full deployment manager",
			dm: DeploymentManager{
				DeploymentManagerID: "ocloud-k8s-1",
				Name:                "Production Kubernetes Cluster",
				Description:         "Main production cluster in US East",
				OCloudID:            "ocloud-1",
				ServiceURI:          "https://api.o2ims.example.com/o2ims/v1",
				SupportedLocations:  []string{"us-east-1a", "us-east-1b"},
				Capabilities:        []string{"compute", "storage"},
				Capacity: &Capacity{
					TotalCPU:           100,
					TotalMemoryMB:      204800,
					TotalStorageGB:     10000,
					AvailableCPU:       50,
					AvailableMemoryMB:  102400,
					AvailableStorageGB: 5000,
				},
				Extensions: map[string]interface{}{
					"clusterVersion": "1.30.0",
					"provider":       "AWS",
				},
			},
		},
		{
			name: "minimal deployment manager",
			dm: DeploymentManager{
				DeploymentManagerID: "dm-minimal",
				Name:                "Minimal DM",
				OCloudID:            "ocloud-1",
				ServiceURI:          "https://api.example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			jsonData, err := json.Marshal(tt.dm)
			require.NoError(t, err)

			// Unmarshal back
			var decoded DeploymentManager
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err)

			// Verify fields
			assert.Equal(t, tt.dm.DeploymentManagerID, decoded.DeploymentManagerID)
			assert.Equal(t, tt.dm.Name, decoded.Name)
			assert.Equal(t, tt.dm.Description, decoded.Description)
			assert.Equal(t, tt.dm.OCloudID, decoded.OCloudID)
			assert.Equal(t, tt.dm.ServiceURI, decoded.ServiceURI)
			assert.Equal(t, tt.dm.SupportedLocations, decoded.SupportedLocations)
			assert.Equal(t, tt.dm.Capabilities, decoded.Capabilities)
		})
	}
}

func TestDeploymentManager_YAMLSerialization(t *testing.T) {
	dm := DeploymentManager{
		DeploymentManagerID: "ocloud-k8s-1",
		Name:                "Test Cluster",
		OCloudID:            "ocloud-1",
		ServiceURI:          "https://api.example.com",
		SupportedLocations:  []string{"us-east-1a"},
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(dm)
	require.NoError(t, err)

	// Unmarshal back
	var decoded DeploymentManager
	err = yaml.Unmarshal(yamlData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, dm.DeploymentManagerID, decoded.DeploymentManagerID)
	assert.Equal(t, dm.Name, decoded.Name)
	assert.Equal(t, dm.OCloudID, decoded.OCloudID)
}

func TestResourcePool_JSONSerialization(t *testing.T) {
	tests := []struct {
		name string
		pool ResourcePool
	}{
		{
			name: "full resource pool",
			pool: ResourcePool{
				ResourcePoolID:   "pool-compute-high-mem",
				Name:             "High Memory Compute Pool",
				Description:      "Nodes with 128GB+ RAM",
				Location:         "us-east-1a",
				OCloudID:         "ocloud-1",
				GlobalLocationID: "geo:37.7749,-122.4194",
				Extensions: map[string]interface{}{
					"machineType": "m5.4xlarge",
					"replicas":    float64(5),
				},
			},
		},
		{
			name: "minimal resource pool",
			pool: ResourcePool{
				ResourcePoolID: "pool-minimal",
				Name:           "Minimal Pool",
				OCloudID:       "ocloud-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			jsonData, err := json.Marshal(tt.pool)
			require.NoError(t, err)

			// Unmarshal back
			var decoded ResourcePool
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err)

			assert.Equal(t, tt.pool.ResourcePoolID, decoded.ResourcePoolID)
			assert.Equal(t, tt.pool.Name, decoded.Name)
			assert.Equal(t, tt.pool.Description, decoded.Description)
			assert.Equal(t, tt.pool.Location, decoded.Location)
			assert.Equal(t, tt.pool.OCloudID, decoded.OCloudID)
			assert.Equal(t, tt.pool.GlobalLocationID, decoded.GlobalLocationID)
		})
	}
}

func TestResourcePool_YAMLSerialization(t *testing.T) {
	pool := ResourcePool{
		ResourcePoolID:   "pool-test",
		Name:             "Test Pool",
		Location:         "us-west-2a",
		OCloudID:         "ocloud-1",
		GlobalLocationID: "geo:47.6062,-122.3321",
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(pool)
	require.NoError(t, err)

	// Unmarshal back
	var decoded ResourcePool
	err = yaml.Unmarshal(yamlData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, pool.ResourcePoolID, decoded.ResourcePoolID)
	assert.Equal(t, pool.Name, decoded.Name)
	assert.Equal(t, pool.Location, decoded.Location)
	assert.Equal(t, pool.GlobalLocationID, decoded.GlobalLocationID)
}

func TestResource_JSONSerialization(t *testing.T) {
	tests := []struct {
		name     string
		resource Resource
	}{
		{
			name: "full resource",
			resource: Resource{
				ResourceID:     "node-worker-1a-abc123",
				ResourceTypeID: "compute-node",
				ResourcePoolID: "pool-compute-high-mem",
				GlobalAssetID:  "urn:o-ran:node:abc123",
				Description:    "Compute node for RAN workloads",
				Extensions: map[string]interface{}{
					"nodeName": "ip-10-0-1-123.ec2.internal",
					"status":   "Ready",
					"cpu":      "16 cores",
				},
			},
		},
		{
			name: "minimal resource",
			resource: Resource{
				ResourceID:     "resource-minimal",
				ResourceTypeID: "compute-node",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			jsonData, err := json.Marshal(tt.resource)
			require.NoError(t, err)

			// Unmarshal back
			var decoded Resource
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err)

			assert.Equal(t, tt.resource.ResourceID, decoded.ResourceID)
			assert.Equal(t, tt.resource.ResourceTypeID, decoded.ResourceTypeID)
			assert.Equal(t, tt.resource.ResourcePoolID, decoded.ResourcePoolID)
			assert.Equal(t, tt.resource.GlobalAssetID, decoded.GlobalAssetID)
			assert.Equal(t, tt.resource.Description, decoded.Description)
		})
	}
}

func TestResource_YAMLSerialization(t *testing.T) {
	resource := Resource{
		ResourceID:     "node-test",
		ResourceTypeID: "compute-node",
		ResourcePoolID: "pool-test",
		GlobalAssetID:  "urn:test:node:123",
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(resource)
	require.NoError(t, err)

	// Unmarshal back
	var decoded Resource
	err = yaml.Unmarshal(yamlData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, resource.ResourceID, decoded.ResourceID)
	assert.Equal(t, resource.ResourceTypeID, decoded.ResourceTypeID)
	assert.Equal(t, resource.ResourcePoolID, decoded.ResourcePoolID)
}

func TestResourceType_JSONSerialization(t *testing.T) {
	tests := []struct {
		name string
		rt   ResourceType
	}{
		{
			name: "full resource type",
			rt: ResourceType{
				ResourceTypeID: "compute-node-highmem",
				Name:           "High Memory Compute Node",
				Description:    "Compute node with 64GB+ RAM",
				Vendor:         "AWS",
				Model:          "m5.4xlarge",
				Version:        "v1",
				ResourceClass:  "compute",
				ResourceKind:   "physical",
				Extensions: map[string]interface{}{
					"cpu":    "16 cores",
					"memory": "64 GB",
				},
			},
		},
		{
			name: "minimal resource type",
			rt: ResourceType{
				ResourceTypeID: "rt-minimal",
				Name:           "Minimal Type",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			jsonData, err := json.Marshal(tt.rt)
			require.NoError(t, err)

			// Unmarshal back
			var decoded ResourceType
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err)

			assert.Equal(t, tt.rt.ResourceTypeID, decoded.ResourceTypeID)
			assert.Equal(t, tt.rt.Name, decoded.Name)
			assert.Equal(t, tt.rt.Description, decoded.Description)
			assert.Equal(t, tt.rt.Vendor, decoded.Vendor)
			assert.Equal(t, tt.rt.Model, decoded.Model)
			assert.Equal(t, tt.rt.Version, decoded.Version)
			assert.Equal(t, tt.rt.ResourceClass, decoded.ResourceClass)
			assert.Equal(t, tt.rt.ResourceKind, decoded.ResourceKind)
		})
	}
}

func TestResourceType_YAMLSerialization(t *testing.T) {
	rt := ResourceType{
		ResourceTypeID: "compute-standard",
		Name:           "Standard Compute",
		Vendor:         "AWS",
		Model:          "m5.xlarge",
		ResourceClass:  "compute",
		ResourceKind:   "virtual",
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(rt)
	require.NoError(t, err)

	// Unmarshal back
	var decoded ResourceType
	err = yaml.Unmarshal(yamlData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, rt.ResourceTypeID, decoded.ResourceTypeID)
	assert.Equal(t, rt.Vendor, decoded.Vendor)
	assert.Equal(t, rt.ResourceClass, decoded.ResourceClass)
}

func TestSubscription_JSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name string
		sub  Subscription
	}{
		{
			name: "full subscription",
			sub: Subscription{
				SubscriptionID:         "550e8400-e29b-41d4-a716-446655440000",
				Callback:               "https://smo.example.com/notifications",
				ConsumerSubscriptionID: "smo-sub-123",
				Filter: &SubscriptionFilter{
					ResourcePoolID: []string{"pool-compute-high-mem"},
					ResourceTypeID: []string{"compute-node"},
					ResourceID:     []string{"node-1", "node-2"},
					Labels: map[string]string{
						"env": "production",
					},
				},
				EventTypes: []string{"ResourceCreated", "ResourceUpdated", "ResourceDeleted"},
				CreatedAt:  now,
				UpdatedAt:  now,
				Extensions: map[string]interface{}{
					"priority": "high",
				},
			},
		},
		{
			name: "minimal subscription",
			sub: Subscription{
				SubscriptionID: "sub-minimal",
				Callback:       "https://example.com/webhook",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			jsonData, err := json.Marshal(tt.sub)
			require.NoError(t, err)

			// Unmarshal back
			var decoded Subscription
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err)

			assert.Equal(t, tt.sub.SubscriptionID, decoded.SubscriptionID)
			assert.Equal(t, tt.sub.Callback, decoded.Callback)
			assert.Equal(t, tt.sub.ConsumerSubscriptionID, decoded.ConsumerSubscriptionID)
			assert.Equal(t, tt.sub.EventTypes, decoded.EventTypes)

			if tt.sub.Filter != nil {
				require.NotNil(t, decoded.Filter)
				assert.Equal(t, tt.sub.Filter.ResourcePoolID, decoded.Filter.ResourcePoolID)
				assert.Equal(t, tt.sub.Filter.ResourceTypeID, decoded.Filter.ResourceTypeID)
				assert.Equal(t, tt.sub.Filter.ResourceID, decoded.Filter.ResourceID)
				assert.Equal(t, tt.sub.Filter.Labels, decoded.Filter.Labels)
			}
		})
	}
}

func TestSubscription_YAMLSerialization(t *testing.T) {
	sub := Subscription{
		SubscriptionID:         "sub-yaml-test",
		Callback:               "https://example.com/webhook",
		ConsumerSubscriptionID: "consumer-123",
		Filter: &SubscriptionFilter{
			ResourcePoolID: []string{"pool-1"},
		},
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(sub)
	require.NoError(t, err)

	// Unmarshal back
	var decoded Subscription
	err = yaml.Unmarshal(yamlData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, sub.SubscriptionID, decoded.SubscriptionID)
	assert.Equal(t, sub.Callback, decoded.Callback)
	assert.Equal(t, sub.ConsumerSubscriptionID, decoded.ConsumerSubscriptionID)
}

func TestSubscriptionFilter_JSONSerialization(t *testing.T) {
	filter := SubscriptionFilter{
		ResourcePoolID: []string{"pool-1", "pool-2"},
		ResourceTypeID: []string{"compute-node"},
		ResourceID:     []string{"node-1"},
		Labels: map[string]string{
			"env":  "production",
			"tier": "frontend",
		},
		Extensions: map[string]interface{}{
			"customField": "customValue",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(filter)
	require.NoError(t, err)

	// Unmarshal back
	var decoded SubscriptionFilter
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, filter.ResourcePoolID, decoded.ResourcePoolID)
	assert.Equal(t, filter.ResourceTypeID, decoded.ResourceTypeID)
	assert.Equal(t, filter.ResourceID, decoded.ResourceID)
	assert.Equal(t, filter.Labels, decoded.Labels)
}

func TestNotification_JSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	notification := Notification{
		SubscriptionID:         "sub-123",
		ConsumerSubscriptionID: "consumer-456",
		EventType:              "ResourceCreated",
		Resource: Resource{
			ResourceID:     "node-new",
			ResourceTypeID: "compute-node",
			ResourcePoolID: "pool-1",
		},
		Timestamp: now,
		Extensions: map[string]interface{}{
			"source": "kubernetes",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(notification)
	require.NoError(t, err)

	// Unmarshal back
	var decoded Notification
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, notification.SubscriptionID, decoded.SubscriptionID)
	assert.Equal(t, notification.ConsumerSubscriptionID, decoded.ConsumerSubscriptionID)
	assert.Equal(t, notification.EventType, decoded.EventType)
}

func TestCapacity_JSONSerialization(t *testing.T) {
	capacity := Capacity{
		TotalCPU:           100,
		TotalMemoryMB:      204800,
		TotalStorageGB:     10000,
		AvailableCPU:       50,
		AvailableMemoryMB:  102400,
		AvailableStorageGB: 5000,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(capacity)
	require.NoError(t, err)

	// Unmarshal back
	var decoded Capacity
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, capacity.TotalCPU, decoded.TotalCPU)
	assert.Equal(t, capacity.TotalMemoryMB, decoded.TotalMemoryMB)
	assert.Equal(t, capacity.TotalStorageGB, decoded.TotalStorageGB)
	assert.Equal(t, capacity.AvailableCPU, decoded.AvailableCPU)
	assert.Equal(t, capacity.AvailableMemoryMB, decoded.AvailableMemoryMB)
	assert.Equal(t, capacity.AvailableStorageGB, decoded.AvailableStorageGB)
}

func TestEventType_String(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  string
	}{
		{EventTypeResourceCreated, "ResourceCreated"},
		{EventTypeResourceUpdated, "ResourceUpdated"},
		{EventTypeResourceDeleted, "ResourceDeleted"},
		{EventTypeResourcePoolCreated, "ResourcePoolCreated"},
		{EventTypeResourcePoolUpdated, "ResourcePoolUpdated"},
		{EventTypeResourcePoolDeleted, "ResourcePoolDeleted"},
		{EventTypeResourceTypeCreated, "ResourceTypeCreated"},
		{EventTypeResourceTypeUpdated, "ResourceTypeUpdated"},
		{EventTypeResourceTypeDeleted, "ResourceTypeDeleted"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.eventType.String())
		})
	}
}

func TestEventType_IsValid(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  bool
	}{
		{EventTypeResourceCreated, true},
		{EventTypeResourceUpdated, true},
		{EventTypeResourceDeleted, true},
		{EventTypeResourcePoolCreated, true},
		{EventTypeResourcePoolUpdated, true},
		{EventTypeResourcePoolDeleted, true},
		{EventTypeResourceTypeCreated, true},
		{EventTypeResourceTypeUpdated, true},
		{EventTypeResourceTypeDeleted, true},
		{EventType("InvalidEvent"), false},
		{EventType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.eventType.IsValid())
		})
	}
}

func TestJSONFieldNames(t *testing.T) {
	// Test that JSON field names match O2-IMS specification
	dm := DeploymentManager{
		DeploymentManagerID: "dm-1",
		Name:                "Test",
		OCloudID:            "ocloud-1",
		ServiceURI:          "https://api.example.com",
	}

	jsonData, err := json.Marshal(dm)
	require.NoError(t, err)

	jsonStr := string(jsonData)
	assert.Contains(t, jsonStr, `"deploymentManagerId"`)
	assert.Contains(t, jsonStr, `"name"`)
	assert.Contains(t, jsonStr, `"oCloudId"`)
	assert.Contains(t, jsonStr, `"serviceUri"`)

	// Test ResourcePool field names
	pool := ResourcePool{
		ResourcePoolID:   "pool-1",
		Name:             "Test",
		OCloudID:         "ocloud-1",
		GlobalLocationID: "geo:0,0",
	}

	jsonData, err = json.Marshal(pool)
	require.NoError(t, err)

	jsonStr = string(jsonData)
	assert.Contains(t, jsonStr, `"resourcePoolId"`)
	assert.Contains(t, jsonStr, `"globalLocationId"`)

	// Test Resource field names
	resource := Resource{
		ResourceID:     "res-1",
		ResourceTypeID: "type-1",
		ResourcePoolID: "pool-1",
		GlobalAssetID:  "urn:test:123",
	}

	jsonData, err = json.Marshal(resource)
	require.NoError(t, err)

	jsonStr = string(jsonData)
	assert.Contains(t, jsonStr, `"resourceId"`)
	assert.Contains(t, jsonStr, `"resourceTypeId"`)
	assert.Contains(t, jsonStr, `"resourcePoolId"`)
	assert.Contains(t, jsonStr, `"globalAssetId"`)

	// Test Subscription field names
	sub := Subscription{
		SubscriptionID:         "sub-1",
		Callback:               "https://example.com",
		ConsumerSubscriptionID: "consumer-1",
	}

	jsonData, err = json.Marshal(sub)
	require.NoError(t, err)

	jsonStr = string(jsonData)
	assert.Contains(t, jsonStr, `"subscriptionId"`)
	assert.Contains(t, jsonStr, `"callback"`)
	assert.Contains(t, jsonStr, `"consumerSubscriptionId"`)
}

func TestOmitEmptyFields(t *testing.T) {
	// Test that optional fields are omitted when empty
	dm := DeploymentManager{
		DeploymentManagerID: "dm-1",
		Name:                "Test",
		OCloudID:            "ocloud-1",
		ServiceURI:          "https://api.example.com",
		// Description, SupportedLocations, Capabilities, Extensions are empty
	}

	jsonData, err := json.Marshal(dm)
	require.NoError(t, err)

	jsonStr := string(jsonData)
	assert.NotContains(t, jsonStr, `"description"`)
	assert.NotContains(t, jsonStr, `"supportedLocations"`)
	assert.NotContains(t, jsonStr, `"capabilities"`)
	assert.NotContains(t, jsonStr, `"extensions"`)
	assert.NotContains(t, jsonStr, `"capacity"`)

	// Test ResourcePool omit empty
	pool := ResourcePool{
		ResourcePoolID: "pool-1",
		Name:           "Test",
		OCloudID:       "ocloud-1",
	}

	jsonData, err = json.Marshal(pool)
	require.NoError(t, err)

	jsonStr = string(jsonData)
	assert.NotContains(t, jsonStr, `"description"`)
	assert.NotContains(t, jsonStr, `"location"`)
	assert.NotContains(t, jsonStr, `"globalLocationId"`)
	assert.NotContains(t, jsonStr, `"extensions"`)
}
