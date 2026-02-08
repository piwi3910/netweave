package models_test

import (
	"net/url"
	"testing"

	"github.com/piwi3910/netweave/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseQueryParams_ResourceIDs tests parsing of resource ID query parameters.
func TestParseQueryParams_ResourceIDs(t *testing.T) {
	tests := []struct {
		name            string
		query           string
		wantPoolIDs     []string
		wantTypeIDs     []string
		wantResourceIDs []string
	}{
		{
			name:        "resource type IDs",
			query:       "resourceTypeId=type-1&resourceTypeId=type-2",
			wantTypeIDs: []string{"type-1", "type-2"},
		},
		{
			name:            "resource IDs",
			query:           "resourceId=res-1&resourceId=res-2&resourceId=res-3",
			wantResourceIDs: []string{"res-1", "res-2", "res-3"},
		},
		{
			name:            "mixed IDs",
			query:           "resourcePoolId=pool-1&resourceTypeId=type-1&resourceId=res-1",
			wantPoolIDs:     []string{"pool-1"},
			wantTypeIDs:     []string{"type-1"},
			wantResourceIDs: []string{"res-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := url.ParseQuery(tt.query)
			require.NoError(t, err)

			filter := models.ParseQueryParams(params)

			if tt.wantPoolIDs != nil {
				assert.Equal(t, tt.wantPoolIDs, filter.ResourcePoolID)
			}
			if tt.wantTypeIDs != nil {
				assert.Equal(t, tt.wantTypeIDs, filter.ResourceTypeID)
			}
			if tt.wantResourceIDs != nil {
				assert.Equal(t, tt.wantResourceIDs, filter.ResourceID)
			}
		})
	}
}

// TestParseQueryParams_OCloudID tests parsing of O-Cloud ID parameter.
func TestParseQueryParams_OCloudID(t *testing.T) {
	params, err := url.ParseQuery("oCloudId=ocloud-123")
	require.NoError(t, err)

	filter := models.ParseQueryParams(params)
	assert.Equal(t, "ocloud-123", filter.OCloudID)
}

// TestFilter_MatchesResource_AllConditions tests resource matching with all condition types.
func TestFilter_MatchesResource_AllConditions(t *testing.T) {
	tests := []struct {
		name     string
		filter   *models.Filter
		resource *models.Resource
		expected bool
	}{
		{
			name: "non-matching resource type ID",
			filter: &models.Filter{
				ResourceTypeID: []string{"compute-node"},
			},
			resource: &models.Resource{
				ResourceID:     "res-1",
				ResourceTypeID: "storage-node",
			},
			expected: false,
		},
		{
			name: "non-matching resource pool ID",
			filter: &models.Filter{
				ResourcePoolID: []string{"pool-1"},
			},
			resource: &models.Resource{
				ResourceID:     "res-1",
				ResourcePoolID: "pool-2",
			},
			expected: false,
		},
		{
			name: "multiple resource IDs - match second",
			filter: &models.Filter{
				ResourceID: []string{"res-1", "res-2"},
			},
			resource: &models.Resource{
				ResourceID: "res-2",
			},
			expected: true,
		},
		{
			name: "combined filters all match",
			filter: &models.Filter{
				ResourceID:     []string{"res-1"},
				ResourceTypeID: []string{"compute-node"},
				ResourcePoolID: []string{"pool-1"},
			},
			resource: &models.Resource{
				ResourceID:     "res-1",
				ResourceTypeID: "compute-node",
				ResourcePoolID: "pool-1",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.MatchesResource(tt.resource)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFilter_MatchesSubscription_EdgeCases tests subscription matching edge cases.
func TestFilter_MatchesSubscription_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		filter   *models.Filter
		sub      *models.Subscription
		expected bool
	}{
		{
			name: "filter with pool IDs and subscription with nil filter",
			filter: &models.Filter{
				ResourcePoolID: []string{"pool-1"},
			},
			sub: &models.Subscription{
				SubscriptionID: "sub-1",
				Filter:         nil,
			},
			expected: true,
		},
		{
			name: "filter with pool IDs and subscription with empty pool IDs",
			filter: &models.Filter{
				ResourcePoolID: []string{"pool-1"},
			},
			sub: &models.Subscription{
				SubscriptionID: "sub-1",
				Filter: &models.SubscriptionFilter{
					ResourcePoolID: []string{},
				},
			},
			expected: true,
		},
		{
			name: "multiple filter pool IDs - one matches",
			filter: &models.Filter{
				ResourcePoolID: []string{"pool-1", "pool-2"},
			},
			sub: &models.Subscription{
				SubscriptionID: "sub-1",
				Filter: &models.SubscriptionFilter{
					ResourcePoolID: []string{"pool-2", "pool-3"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.MatchesSubscription(tt.sub)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFilter_ToQueryParams_AllIDTypes tests ToQueryParams with resource type and resource IDs.
func TestFilter_ToQueryParams_AllIDTypes(t *testing.T) {
	filter := &models.Filter{
		ResourcePoolID: []string{"pool-1"},
		ResourceTypeID: []string{"type-1", "type-2"},
		ResourceID:     []string{"res-1"},
	}

	result := filter.ToQueryParams()

	assert.Equal(t, []string{"pool-1"}, result["resourcePoolId"])
	assert.Equal(t, []string{"type-1", "type-2"}, result["resourceTypeId"])
	assert.Equal(t, []string{"res-1"}, result["resourceId"])
}

// TestFilter_ToQueryParams_AllStringFields tests all string filter params.
func TestFilter_ToQueryParams_AllStringFields(t *testing.T) {
	filter := &models.Filter{
		Location:      "us-east",
		OCloudID:      "ocloud-1",
		ResourceClass: "compute",
		ResourceKind:  "physical",
		Vendor:        "AWS",
		Model:         "m5.xlarge",
	}

	result := filter.ToQueryParams()

	assert.Equal(t, "us-east", result.Get("location"))
	assert.Equal(t, "ocloud-1", result.Get("oCloudId"))
	assert.Equal(t, "compute", result.Get("resourceClass"))
	assert.Equal(t, "physical", result.Get("resourceKind"))
	assert.Equal(t, "AWS", result.Get("vendor"))
	assert.Equal(t, "m5.xlarge", result.Get("model"))
}

// TestFilter_IsEmpty_AllFields tests IsEmpty with various non-empty fields.
func TestFilter_IsEmpty_AllFields(t *testing.T) {
	tests := []struct {
		name     string
		filter   *models.Filter
		expected bool
	}{
		{
			name:     "empty filter",
			filter:   &models.Filter{},
			expected: true,
		},
		{
			name:     "resource type ID makes non-empty",
			filter:   &models.Filter{ResourceTypeID: []string{"type-1"}},
			expected: false,
		},
		{
			name:     "resource ID makes non-empty",
			filter:   &models.Filter{ResourceID: []string{"res-1"}},
			expected: false,
		},
		{
			name:     "oCloudID makes non-empty",
			filter:   &models.Filter{OCloudID: "ocloud-1"},
			expected: false,
		},
		{
			name:     "resource class makes non-empty",
			filter:   &models.Filter{ResourceClass: "compute"},
			expected: false,
		},
		{
			name:     "resource kind makes non-empty",
			filter:   &models.Filter{ResourceKind: "physical"},
			expected: false,
		},
		{
			name:     "vendor makes non-empty",
			filter:   &models.Filter{Vendor: "AWS"},
			expected: false,
		},
		{
			name:     "model makes non-empty",
			filter:   &models.Filter{Model: "m5.xlarge"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.IsEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDeepCopyValue_AllTypes tests deep copy for various value types.
func TestDeepCopyValue_AllTypes(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{
			name:  "nil value",
			value: nil,
		},
		{
			name:  "string value",
			value: "hello",
		},
		{
			name:  "int value",
			value: 42,
		},
		{
			name:  "bool value",
			value: true,
		},
		{
			name:  "float64 value",
			value: 3.14,
		},
		{
			name:  "map value",
			value: map[string]interface{}{"key": "value", "nested": map[string]interface{}{"inner": "data"}},
		},
		{
			name:  "interface slice",
			value: []interface{}{"a", "b", map[string]interface{}{"c": "d"}},
		},
		{
			name:  "map slice",
			value: []map[string]interface{}{{"id": "1"}, {"id": "2"}},
		},
		{
			name:  "string slice",
			value: []string{"a", "b", "c"},
		},
		{
			name:  "int slice",
			value: []int{1, 2, 3},
		},
		{
			name:  "int64 slice",
			value: []int64{1, 2, 3},
		},
		{
			name:  "float64 slice",
			value: []float64{1.1, 2.2, 3.3},
		},
		{
			name:  "bool slice",
			value: []bool{true, false, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := models.DeepCopyValue(tt.value)

			if tt.value == nil {
				assert.Nil(t, result)
				return
			}

			assert.Equal(t, tt.value, result)
		})
	}
}

// TestDeepCopyValue_MapMutationSafety tests that deep copy prevents mutation.
func TestDeepCopyValue_MapMutationSafety(t *testing.T) {
	original := map[string]interface{}{
		"key": "original-value",
		"nested": map[string]interface{}{
			"inner": "original-inner",
		},
	}

	copied := models.DeepCopyValue(original)
	copiedMap, ok := copied.(map[string]interface{})
	require.True(t, ok)

	// Mutate the copy
	copiedMap["key"] = "modified"
	nestedCopy, ok := copiedMap["nested"].(map[string]interface{})
	require.True(t, ok)
	nestedCopy["inner"] = "modified-inner"

	// Original should be unchanged
	assert.Equal(t, "original-value", original["key"])
	nestedOrig, ok := original["nested"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "original-inner", nestedOrig["inner"])
}

// TestDeepCopyValue_SliceMutationSafety tests that deep copy prevents slice mutation.
func TestDeepCopyValue_SliceMutationSafety(t *testing.T) {
	original := []string{"a", "b", "c"}

	copied := models.DeepCopyValue(original)
	copiedSlice, ok := copied.([]string)
	require.True(t, ok)

	// Mutate the copy
	copiedSlice[0] = "modified"

	// Original should be unchanged
	assert.Equal(t, "a", original[0])
}

// TestFilter_SelectFields_NestedMerge tests field selection with multiple nested fields.
func TestFilter_SelectFields_NestedMerge(t *testing.T) {
	filter := &models.Filter{Fields: []string{"extensions.cpu", "extensions.memory"}}
	data := map[string]interface{}{
		"id": "123",
		"extensions": map[string]interface{}{
			"cpu":    "4 cores",
			"memory": "16GB",
			"disk":   "100GB",
		},
	}

	result := filter.SelectFields(data)

	extensions, ok := result["extensions"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "4 cores", extensions["cpu"])
	assert.Equal(t, "16GB", extensions["memory"])
	_, hasDisk := extensions["disk"]
	assert.False(t, hasDisk, "disk should not be included")
}

// TestFilter_SelectFields_NestedNonMap tests field selection where nested path hits non-map.
func TestFilter_SelectFields_NestedNonMap(t *testing.T) {
	filter := &models.Filter{Fields: []string{"data.nested.field"}}
	data := map[string]interface{}{
		"data": "not-a-map",
	}

	result := filter.SelectFields(data)
	assert.Empty(t, result)
}

// TestFilter_MatchesResourceType_AllFilters tests resource type matching with all filter fields.
func TestFilter_MatchesResourceType_AllFilters(t *testing.T) {
	tests := []struct {
		name     string
		filter   *models.Filter
		rt       *models.ResourceType
		expected bool
	}{
		{
			name:     "non-matching resource kind",
			filter:   &models.Filter{ResourceKind: "virtual"},
			rt:       &models.ResourceType{ResourceKind: "physical"},
			expected: false,
		},
		{
			name:     "non-matching vendor",
			filter:   &models.Filter{Vendor: "AWS"},
			rt:       &models.ResourceType{Vendor: "GCP"},
			expected: false,
		},
		{
			name:     "non-matching model",
			filter:   &models.Filter{Model: "m5.xlarge"},
			rt:       &models.ResourceType{Model: "m5.2xlarge"},
			expected: false,
		},
		{
			name:     "non-matching resource type ID",
			filter:   &models.Filter{ResourceTypeID: []string{"type-1"}},
			rt:       &models.ResourceType{ResourceTypeID: "type-2"},
			expected: false,
		},
		{
			name: "all filters match",
			filter: &models.Filter{
				ResourceTypeID: []string{"compute-node"},
				ResourceClass:  "compute",
				ResourceKind:   "physical",
				Vendor:         "AWS",
				Model:          "m5.xlarge",
			},
			rt: &models.ResourceType{
				ResourceTypeID: "compute-node",
				ResourceClass:  "compute",
				ResourceKind:   "physical",
				Vendor:         "AWS",
				Model:          "m5.xlarge",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.MatchesResourceType(tt.rt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseQueryParams_InvalidLimit tests that invalid limit values are handled.
func TestParseQueryParams_InvalidLimit(t *testing.T) {
	params, err := url.ParseQuery("limit=invalid")
	require.NoError(t, err)

	filter := models.ParseQueryParams(params)
	assert.Equal(t, 100, filter.Limit) // Falls back to default
}

// TestParseQueryParams_NegativeLimit tests that negative limit values are handled.
func TestParseQueryParams_NegativeLimit(t *testing.T) {
	params, err := url.ParseQuery("limit=-5")
	require.NoError(t, err)

	filter := models.ParseQueryParams(params)
	assert.Equal(t, 100, filter.Limit) // Falls back to default
}

// TestParseQueryParams_NegativeOffset tests that negative offset values are handled.
func TestParseQueryParams_NegativeOffset(t *testing.T) {
	params, err := url.ParseQuery("offset=-5")
	require.NoError(t, err)

	filter := models.ParseQueryParams(params)
	assert.Equal(t, 0, filter.Offset) // Negative offset ignored
}

// TestParseQueryParams_InvalidOffset tests that invalid offset values are handled.
func TestParseQueryParams_InvalidOffset(t *testing.T) {
	params, err := url.ParseQuery("offset=invalid")
	require.NoError(t, err)

	filter := models.ParseQueryParams(params)
	assert.Equal(t, 0, filter.Offset) // Invalid offset ignored
}
