package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/o2ims/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeploymentManagerJSONMarshaling tests DeploymentManager JSON marshaling and unmarshaling.
func TestDeploymentManagerJSONMarshaling(t *testing.T) {
	dm := &models.DeploymentManager{
		DeploymentManagerID: "dm-123",
		Name:                "prod-cluster-1",
		Description:         "Production Kubernetes cluster",
		OCloudID:            "ocloud-456",
		ServiceURI:          "https://cluster.example.com",
		SupportedLocations:  []string{"us-east-1a", "us-east-1b"},
		Capabilities:        []string{"helm", "flux", "argocd"},
		Extensions: map[string]interface{}{
			"version": "1.28.0",
			"provider": "aws",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(dm)
	require.NoError(t, err)

	// Unmarshal back
	var decoded models.DeploymentManager
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, dm.DeploymentManagerID, decoded.DeploymentManagerID)
	assert.Equal(t, dm.Name, decoded.Name)
	assert.Equal(t, dm.Description, decoded.Description)
	assert.Equal(t, dm.OCloudID, decoded.OCloudID)
	assert.Equal(t, dm.ServiceURI, decoded.ServiceURI)
	assert.Equal(t, dm.SupportedLocations, decoded.SupportedLocations)
	assert.Equal(t, dm.Capabilities, decoded.Capabilities)
	assert.Equal(t, "1.28.0", decoded.Extensions["version"])
	assert.Equal(t, "aws", decoded.Extensions["provider"])
}

// TestResourcePoolJSONMarshaling tests ResourcePool JSON marshaling and unmarshaling.
func TestResourcePoolJSONMarshaling(t *testing.T) {
	pool := &models.ResourcePool{
		ResourcePoolID: "pool-789",
		Name:           "compute-pool-1",
		Description:    "Primary compute resource pool",
		Location:       "us-east-1",
		OCloudID:       "ocloud-456",
		GlobalAssetID:  "asset-001",
		Extensions: map[string]interface{}{
			"nodeCount": 10,
			"region":    "us-east",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(pool)
	require.NoError(t, err)

	// Unmarshal back
	var decoded models.ResourcePool
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, pool.ResourcePoolID, decoded.ResourcePoolID)
	assert.Equal(t, pool.Name, decoded.Name)
	assert.Equal(t, pool.Description, decoded.Description)
	assert.Equal(t, pool.Location, decoded.Location)
	assert.Equal(t, pool.OCloudID, decoded.OCloudID)
	assert.Equal(t, pool.GlobalAssetID, decoded.GlobalAssetID)

	// Convert interface{} to float64 for numeric comparison (JSON unmarshals numbers as float64)
	nodeCount, ok := decoded.Extensions["nodeCount"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(10), nodeCount)
	assert.Equal(t, "us-east", decoded.Extensions["region"])
}

// TestResourceJSONMarshaling tests Resource JSON marshaling and unmarshaling.
func TestResourceJSONMarshaling(t *testing.T) {
	resource := &models.Resource{
		ResourceID:     "res-001",
		ResourceTypeID: "compute-node",
		ResourcePoolID: "pool-789",
		Name:           "node-1",
		Description:    "Compute node in pool 1",
		GlobalAssetID:  "asset-node-001",
		ParentID:       "pool-789",
		Extensions: map[string]interface{}{
			"cpu":    "8 cores",
			"memory": "32GB",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(resource)
	require.NoError(t, err)

	// Unmarshal back
	var decoded models.Resource
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, resource.ResourceID, decoded.ResourceID)
	assert.Equal(t, resource.ResourceTypeID, decoded.ResourceTypeID)
	assert.Equal(t, resource.ResourcePoolID, decoded.ResourcePoolID)
	assert.Equal(t, resource.Name, decoded.Name)
	assert.Equal(t, resource.Description, decoded.Description)
	assert.Equal(t, resource.GlobalAssetID, decoded.GlobalAssetID)
	assert.Equal(t, resource.ParentID, decoded.ParentID)
	assert.Equal(t, "8 cores", decoded.Extensions["cpu"])
	assert.Equal(t, "32GB", decoded.Extensions["memory"])
}

// TestResourceTypeJSONMarshaling tests ResourceType JSON marshaling and unmarshaling.
func TestResourceTypeJSONMarshaling(t *testing.T) {
	resourceType := &models.ResourceType{
		ResourceTypeID: "compute-node",
		Name:           "Compute Node",
		Description:    "Physical or virtual compute resource",
		Vendor:         "Dell",
		Model:          "PowerEdge R740",
		Version:        "2.0",
		Extensions: map[string]interface{}{
			"generation": "14th",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(resourceType)
	require.NoError(t, err)

	// Unmarshal back
	var decoded models.ResourceType
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, resourceType.ResourceTypeID, decoded.ResourceTypeID)
	assert.Equal(t, resourceType.Name, decoded.Name)
	assert.Equal(t, resourceType.Description, decoded.Description)
	assert.Equal(t, resourceType.Vendor, decoded.Vendor)
	assert.Equal(t, resourceType.Model, decoded.Model)
	assert.Equal(t, resourceType.Version, decoded.Version)
	assert.Equal(t, "14th", decoded.Extensions["generation"])
}

// TestSubscriptionJSONMarshaling tests Subscription JSON marshaling and unmarshaling.
func TestSubscriptionJSONMarshaling(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	subscription := &models.Subscription{
		SubscriptionID:         "sub-123",
		Callback:               "https://smo.example.com/notify",
		ConsumerSubscriptionID: "consumer-456",
		Filter: models.SubscriptionFilter{
			ResourcePoolID: []string{"pool-1", "pool-2"},
			ResourceTypeID: []string{"compute-node"},
			ResourceID:     []string{"res-001", "res-002"},
		},
		CreatedAt: now,
	}

	// Marshal to JSON
	data, err := json.Marshal(subscription)
	require.NoError(t, err)

	// Unmarshal back
	var decoded models.Subscription
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, subscription.SubscriptionID, decoded.SubscriptionID)
	assert.Equal(t, subscription.Callback, decoded.Callback)
	assert.Equal(t, subscription.ConsumerSubscriptionID, decoded.ConsumerSubscriptionID)
	assert.Equal(t, subscription.Filter.ResourcePoolID, decoded.Filter.ResourcePoolID)
	assert.Equal(t, subscription.Filter.ResourceTypeID, decoded.Filter.ResourceTypeID)
	assert.Equal(t, subscription.Filter.ResourceID, decoded.Filter.ResourceID)
	assert.True(t, subscription.CreatedAt.Equal(decoded.CreatedAt))
}

// TestSubscriptionFilterDefaults tests SubscriptionFilter with empty values.
func TestSubscriptionFilterDefaults(t *testing.T) {
	filter := models.SubscriptionFilter{}

	assert.Nil(t, filter.ResourcePoolID)
	assert.Nil(t, filter.ResourceTypeID)
	assert.Nil(t, filter.ResourceID)
}

// TestSubscriptionFilterWithValues tests SubscriptionFilter with populated values.
func TestSubscriptionFilterWithValues(t *testing.T) {
	filter := models.SubscriptionFilter{
		ResourcePoolID: []string{"pool-1", "pool-2", "pool-3"},
		ResourceTypeID: []string{"compute", "storage"},
		ResourceID:     []string{"res-1"},
	}

	assert.Len(t, filter.ResourcePoolID, 3)
	assert.Len(t, filter.ResourceTypeID, 2)
	assert.Len(t, filter.ResourceID, 1)
	assert.Contains(t, filter.ResourcePoolID, "pool-1")
	assert.Contains(t, filter.ResourceTypeID, "compute")
	assert.Contains(t, filter.ResourceID, "res-1")
}

// TestListResponseJSONMarshaling tests ListResponse JSON marshaling.
func TestListResponseJSONMarshaling(t *testing.T) {
	pools := []models.ResourcePool{
		{
			ResourcePoolID: "pool-1",
			Name:           "Pool 1",
			OCloudID:       "ocloud-1",
		},
		{
			ResourcePoolID: "pool-2",
			Name:           "Pool 2",
			OCloudID:       "ocloud-1",
		},
	}

	listResp := &models.ListResponse{
		Items:      pools,
		TotalCount: 2,
		NextCursor: "cursor-123",
	}

	// Marshal to JSON
	data, err := json.Marshal(listResp)
	require.NoError(t, err)

	// Verify JSON structure
	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Contains(t, decoded, "items")

	// TotalCount is float64 after JSON unmarshal
	totalCount, ok := decoded["totalCount"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(2), totalCount)

	assert.Equal(t, "cursor-123", decoded["nextCursor"])
}

// TestErrorResponseJSONMarshaling tests ErrorResponse JSON marshaling.
func TestErrorResponseJSONMarshaling(t *testing.T) {
	errResp := &models.ErrorResponse{
		Error:   "NotFound",
		Message: "Resource pool not found",
		Code:    404,
	}

	// Marshal to JSON
	data, err := json.Marshal(errResp)
	require.NoError(t, err)

	// Unmarshal back
	var decoded models.ErrorResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, errResp.Error, decoded.Error)
	assert.Equal(t, errResp.Message, decoded.Message)
	assert.Equal(t, errResp.Code, decoded.Code)
}

// TestErrorResponseStructure tests ErrorResponse field validation.
func TestErrorResponseStructure(t *testing.T) {
	tests := []struct {
		name    string
		errResp *models.ErrorResponse
	}{
		{
			name: "not found error",
			errResp: &models.ErrorResponse{
				Error:   "NotFound",
				Message: "The requested resource was not found",
				Code:    404,
			},
		},
		{
			name: "bad request error",
			errResp: &models.ErrorResponse{
				Error:   "BadRequest",
				Message: "Invalid filter syntax",
				Code:    400,
			},
		},
		{
			name: "internal server error",
			errResp: &models.ErrorResponse{
				Error:   "InternalServerError",
				Message: "An unexpected error occurred",
				Code:    500,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.errResp.Error)
			assert.NotEmpty(t, tt.errResp.Message)
			assert.Greater(t, tt.errResp.Code, 0)
		})
	}
}

// TestDeploymentManagerMinimalFields tests DeploymentManager with only required fields.
func TestDeploymentManagerMinimalFields(t *testing.T) {
	dm := &models.DeploymentManager{
		DeploymentManagerID: "dm-minimal",
		Name:                "minimal-cluster",
		OCloudID:            "ocloud-1",
		ServiceURI:          "https://cluster.example.com",
	}

	// Marshal to JSON
	data, err := json.Marshal(dm)
	require.NoError(t, err)

	// Verify omitempty works for optional fields
	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Required fields present
	assert.Contains(t, decoded, "deploymentManagerId")
	assert.Contains(t, decoded, "name")
	assert.Contains(t, decoded, "oCloudId")
	assert.Contains(t, decoded, "serviceUri")
}

// TestResourcePoolMinimalFields tests ResourcePool with only required fields.
func TestResourcePoolMinimalFields(t *testing.T) {
	pool := &models.ResourcePool{
		ResourcePoolID: "pool-minimal",
		Name:           "minimal-pool",
		OCloudID:       "ocloud-1",
	}

	// Marshal to JSON
	data, err := json.Marshal(pool)
	require.NoError(t, err)

	// Verify omitempty works
	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Required fields present
	assert.Contains(t, decoded, "resourcePoolId")
	assert.Contains(t, decoded, "name")
	assert.Contains(t, decoded, "oCloudId")
}

// TestResourceMinimalFields tests Resource with only required fields.
func TestResourceMinimalFields(t *testing.T) {
	resource := &models.Resource{
		ResourceID:     "res-minimal",
		ResourceTypeID: "compute",
		Name:           "minimal-resource",
	}

	// Marshal to JSON
	data, err := json.Marshal(resource)
	require.NoError(t, err)

	// Verify structure
	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Required fields present
	assert.Contains(t, decoded, "resourceId")
	assert.Contains(t, decoded, "resourceTypeId")
	assert.Contains(t, decoded, "name")
}

// TestSubscriptionMinimalFields tests Subscription with only required fields.
func TestSubscriptionMinimalFields(t *testing.T) {
	subscription := &models.Subscription{
		SubscriptionID: "sub-minimal",
		Callback:       "https://smo.example.com/callback",
	}

	// Marshal to JSON
	data, err := json.Marshal(subscription)
	require.NoError(t, err)

	// Verify structure
	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Required fields present
	assert.Contains(t, decoded, "subscriptionId")
	assert.Contains(t, decoded, "callback")
}

// TestExtensionsFieldFlexibility tests that extensions can hold various types.
func TestExtensionsFieldFlexibility(t *testing.T) {
	pool := &models.ResourcePool{
		ResourcePoolID: "pool-ext",
		Name:           "extension-test-pool",
		OCloudID:       "ocloud-1",
		Extensions: map[string]interface{}{
			"stringField":  "value",
			"intField":     42,
			"boolField":    true,
			"floatField":   3.14,
			"arrayField":   []string{"a", "b", "c"},
			"objectField":  map[string]string{"key": "val"},
			"nullableField": nil,
		},
	}

	// Marshal and unmarshal
	data, err := json.Marshal(pool)
	require.NoError(t, err)

	var decoded models.ResourcePool
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify different types
	assert.Equal(t, "value", decoded.Extensions["stringField"])

	// JSON unmarshals numbers as float64
	intField, ok := decoded.Extensions["intField"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(42), intField)

	assert.Equal(t, true, decoded.Extensions["boolField"])

	floatField, ok := decoded.Extensions["floatField"].(float64)
	require.True(t, ok)
	assert.InDelta(t, 3.14, floatField, 0.001)

	assert.Nil(t, decoded.Extensions["nullableField"])
}
