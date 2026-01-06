// Package models contains the O2-IMS data models for the netweave gateway.
package models

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQueryParams_ResourcePoolID(t *testing.T) {
	tests := []struct {
		name     string
		params   url.Values
		expected []string
	}{
		{
			name:     "single resource pool ID",
			params:   url.Values{"resourcePoolId": {"pool-1"}},
			expected: []string{"pool-1"},
		},
		{
			name:     "multiple resource pool IDs",
			params:   url.Values{"resourcePoolId": {"pool-1", "pool-2", "pool-3"}},
			expected: []string{"pool-1", "pool-2", "pool-3"},
		},
		{
			name:     "no resource pool ID",
			params:   url.Values{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := ParseQueryParams(tt.params)
			assert.Equal(t, tt.expected, filter.ResourcePoolID)
		})
	}
}

func TestParseQueryParams_ResourceTypeID(t *testing.T) {
	tests := []struct {
		name     string
		params   url.Values
		expected []string
	}{
		{
			name:     "single resource type ID",
			params:   url.Values{"resourceTypeId": {"compute-node"}},
			expected: []string{"compute-node"},
		},
		{
			name:     "multiple resource type IDs",
			params:   url.Values{"resourceTypeId": {"compute-node", "storage-node"}},
			expected: []string{"compute-node", "storage-node"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := ParseQueryParams(tt.params)
			assert.Equal(t, tt.expected, filter.ResourceTypeID)
		})
	}
}

func TestParseQueryParams_ResourceID(t *testing.T) {
	params := url.Values{"resourceId": {"node-1", "node-2"}}
	filter := ParseQueryParams(params)
	assert.Equal(t, []string{"node-1", "node-2"}, filter.ResourceID)
}

func TestParseQueryParams_Location(t *testing.T) {
	params := url.Values{"location": {"us-east-1a"}}
	filter := ParseQueryParams(params)
	assert.Equal(t, "us-east-1a", filter.Location)
}

func TestParseQueryParams_OCloudID(t *testing.T) {
	params := url.Values{"oCloudId": {"ocloud-1"}}
	filter := ParseQueryParams(params)
	assert.Equal(t, "ocloud-1", filter.OCloudID)
}

func TestParseQueryParams_ResourceClass(t *testing.T) {
	params := url.Values{"resourceClass": {"compute"}}
	filter := ParseQueryParams(params)
	assert.Equal(t, "compute", filter.ResourceClass)
}

func TestParseQueryParams_ResourceKind(t *testing.T) {
	params := url.Values{"resourceKind": {"physical"}}
	filter := ParseQueryParams(params)
	assert.Equal(t, "physical", filter.ResourceKind)
}

func TestParseQueryParams_VendorAndModel(t *testing.T) {
	params := url.Values{
		"vendor": {"AWS"},
		"model":  {"m5.4xlarge"},
	}
	filter := ParseQueryParams(params)
	assert.Equal(t, "AWS", filter.Vendor)
	assert.Equal(t, "m5.4xlarge", filter.Model)
}

func TestParseQueryParams_Labels(t *testing.T) {
	tests := []struct {
		name     string
		params   url.Values
		expected map[string]string
	}{
		{
			name:   "single label",
			params: url.Values{"labels": {"env:production"}},
			expected: map[string]string{
				"env": "production",
			},
		},
		{
			name:   "multiple labels",
			params: url.Values{"labels": {"env:production,tier:frontend,app:web"}},
			expected: map[string]string{
				"env":  "production",
				"tier": "frontend",
				"app":  "web",
			},
		},
		{
			name:   "labels with spaces",
			params: url.Values{"labels": {"env : production , tier : frontend"}},
			expected: map[string]string{
				"env":  "production",
				"tier": "frontend",
			},
		},
		{
			name:     "empty labels",
			params:   url.Values{},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := ParseQueryParams(tt.params)
			assert.Equal(t, tt.expected, filter.Labels)
		})
	}
}

func TestParseQueryParams_Limit(t *testing.T) {
	tests := []struct {
		name     string
		params   url.Values
		expected int
	}{
		{
			name:     "default limit",
			params:   url.Values{},
			expected: 100,
		},
		{
			name:     "custom limit",
			params:   url.Values{"limit": {"50"}},
			expected: 50,
		},
		{
			name:     "limit exceeds max",
			params:   url.Values{"limit": {"2000"}},
			expected: 1000, // Max limit
		},
		{
			name:     "invalid limit",
			params:   url.Values{"limit": {"invalid"}},
			expected: 100, // Default
		},
		{
			name:     "negative limit",
			params:   url.Values{"limit": {"-5"}},
			expected: 100, // Default (negative not allowed)
		},
		{
			name:     "zero limit",
			params:   url.Values{"limit": {"0"}},
			expected: 100, // Default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := ParseQueryParams(tt.params)
			assert.Equal(t, tt.expected, filter.Limit)
		})
	}
}

func TestParseQueryParams_Offset(t *testing.T) {
	tests := []struct {
		name     string
		params   url.Values
		expected int
	}{
		{
			name:     "default offset",
			params:   url.Values{},
			expected: 0,
		},
		{
			name:     "custom offset",
			params:   url.Values{"offset": {"100"}},
			expected: 100,
		},
		{
			name:     "invalid offset",
			params:   url.Values{"offset": {"invalid"}},
			expected: 0,
		},
		{
			name:     "negative offset",
			params:   url.Values{"offset": {"-5"}},
			expected: 0, // Negative not allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := ParseQueryParams(tt.params)
			assert.Equal(t, tt.expected, filter.Offset)
		})
	}
}

func TestParseQueryParams_Sorting(t *testing.T) {
	tests := []struct {
		name          string
		params        url.Values
		expectedBy    string
		expectedOrder string
	}{
		{
			name:          "default sort order",
			params:        url.Values{},
			expectedBy:    "",
			expectedOrder: "asc",
		},
		{
			name:          "sort by name ascending",
			params:        url.Values{"sortBy": {"name"}, "sortOrder": {"asc"}},
			expectedBy:    "name",
			expectedOrder: "asc",
		},
		{
			name:          "sort by createdAt descending",
			params:        url.Values{"sortBy": {"createdAt"}, "sortOrder": {"desc"}},
			expectedBy:    "createdAt",
			expectedOrder: "desc",
		},
		{
			name:          "invalid sort order defaults to asc",
			params:        url.Values{"sortBy": {"name"}, "sortOrder": {"invalid"}},
			expectedBy:    "name",
			expectedOrder: "asc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := ParseQueryParams(tt.params)
			assert.Equal(t, tt.expectedBy, filter.SortBy)
			assert.Equal(t, tt.expectedOrder, filter.SortOrder)
		})
	}
}

func TestParseQueryParams_CompleteExample(t *testing.T) {
	params := url.Values{
		"resourcePoolId": {"pool-1", "pool-2"},
		"resourceTypeId": {"compute-node"},
		"location":       {"us-east"},
		"oCloudId":       {"ocloud-1"},
		"labels":         {"env:prod,tier:frontend"},
		"limit":          {"50"},
		"offset":         {"10"},
		"sortBy":         {"name"},
		"sortOrder":      {"desc"},
	}

	filter := ParseQueryParams(params)

	assert.Equal(t, []string{"pool-1", "pool-2"}, filter.ResourcePoolID)
	assert.Equal(t, []string{"compute-node"}, filter.ResourceTypeID)
	assert.Equal(t, "us-east", filter.Location)
	assert.Equal(t, "ocloud-1", filter.OCloudID)
	assert.Equal(t, map[string]string{"env": "prod", "tier": "frontend"}, filter.Labels)
	assert.Equal(t, 50, filter.Limit)
	assert.Equal(t, 10, filter.Offset)
	assert.Equal(t, "name", filter.SortBy)
	assert.Equal(t, "desc", filter.SortOrder)
}

func TestFilter_ToQueryParams(t *testing.T) {
	filter := &Filter{
		ResourcePoolID: []string{"pool-1", "pool-2"},
		ResourceTypeID: []string{"compute-node"},
		ResourceID:     []string{"node-1"},
		Location:       "us-east-1a",
		OCloudID:       "ocloud-1",
		ResourceClass:  "compute",
		ResourceKind:   "physical",
		Vendor:         "AWS",
		Model:          "m5.4xlarge",
		Labels: map[string]string{
			"env": "production",
		},
		Limit:     50,
		Offset:    10,
		SortBy:    "name",
		SortOrder: "desc",
	}

	params := filter.ToQueryParams()

	assert.Equal(t, []string{"pool-1", "pool-2"}, params["resourcePoolId"])
	assert.Equal(t, []string{"compute-node"}, params["resourceTypeId"])
	assert.Equal(t, []string{"node-1"}, params["resourceId"])
	assert.Equal(t, "us-east-1a", params.Get("location"))
	assert.Equal(t, "ocloud-1", params.Get("oCloudId"))
	assert.Equal(t, "compute", params.Get("resourceClass"))
	assert.Equal(t, "physical", params.Get("resourceKind"))
	assert.Equal(t, "AWS", params.Get("vendor"))
	assert.Equal(t, "m5.4xlarge", params.Get("model"))
	assert.Equal(t, "50", params.Get("limit"))
	assert.Equal(t, "10", params.Get("offset"))
	assert.Equal(t, "name", params.Get("sortBy"))
	assert.Equal(t, "desc", params.Get("sortOrder"))
	assert.Contains(t, params.Get("labels"), "env:production")
}

func TestFilter_ToQueryParams_Empty(t *testing.T) {
	filter := &Filter{}
	params := filter.ToQueryParams()

	// Should not contain any of the optional fields
	assert.Empty(t, params.Get("location"))
	assert.Empty(t, params.Get("oCloudId"))
	assert.Empty(t, params.Get("resourceClass"))
	assert.Empty(t, params.Get("vendor"))
}

func TestFilter_MatchesResourcePool(t *testing.T) {
	tests := []struct {
		name     string
		filter   *Filter
		pool     *ResourcePool
		expected bool
	}{
		{
			name:   "empty filter matches all",
			filter: &Filter{},
			pool: &ResourcePool{
				ResourcePoolID: "pool-1",
				Location:       "us-east-1a",
				OCloudID:       "ocloud-1",
			},
			expected: true,
		},
		{
			name: "matching resource pool ID",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1", "pool-2"},
			},
			pool: &ResourcePool{
				ResourcePoolID: "pool-1",
			},
			expected: true,
		},
		{
			name: "non-matching resource pool ID",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1", "pool-2"},
			},
			pool: &ResourcePool{
				ResourcePoolID: "pool-3",
			},
			expected: false,
		},
		{
			name: "matching location prefix",
			filter: &Filter{
				Location: "us-east",
			},
			pool: &ResourcePool{
				ResourcePoolID: "pool-1",
				Location:       "us-east-1a",
			},
			expected: true,
		},
		{
			name: "non-matching location prefix",
			filter: &Filter{
				Location: "us-west",
			},
			pool: &ResourcePool{
				ResourcePoolID: "pool-1",
				Location:       "us-east-1a",
			},
			expected: false,
		},
		{
			name: "matching O-Cloud ID",
			filter: &Filter{
				OCloudID: "ocloud-1",
			},
			pool: &ResourcePool{
				ResourcePoolID: "pool-1",
				OCloudID:       "ocloud-1",
			},
			expected: true,
		},
		{
			name: "non-matching O-Cloud ID",
			filter: &Filter{
				OCloudID: "ocloud-1",
			},
			pool: &ResourcePool{
				ResourcePoolID: "pool-1",
				OCloudID:       "ocloud-2",
			},
			expected: false,
		},
		{
			name: "multiple criteria all match",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1"},
				Location:       "us-east",
				OCloudID:       "ocloud-1",
			},
			pool: &ResourcePool{
				ResourcePoolID: "pool-1",
				Location:       "us-east-1a",
				OCloudID:       "ocloud-1",
			},
			expected: true,
		},
		{
			name: "multiple criteria one fails",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1"},
				Location:       "us-west", // This won't match
				OCloudID:       "ocloud-1",
			},
			pool: &ResourcePool{
				ResourcePoolID: "pool-1",
				Location:       "us-east-1a",
				OCloudID:       "ocloud-1",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.MatchesResourcePool(tt.pool)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilter_MatchesResource(t *testing.T) {
	tests := []struct {
		name     string
		filter   *Filter
		resource *Resource
		expected bool
	}{
		{
			name:   "empty filter matches all",
			filter: &Filter{},
			resource: &Resource{
				ResourceID:     "node-1",
				ResourceTypeID: "compute-node",
				ResourcePoolID: "pool-1",
			},
			expected: true,
		},
		{
			name: "matching resource ID",
			filter: &Filter{
				ResourceID: []string{"node-1", "node-2"},
			},
			resource: &Resource{
				ResourceID: "node-1",
			},
			expected: true,
		},
		{
			name: "non-matching resource ID",
			filter: &Filter{
				ResourceID: []string{"node-1", "node-2"},
			},
			resource: &Resource{
				ResourceID: "node-3",
			},
			expected: false,
		},
		{
			name: "matching resource type ID",
			filter: &Filter{
				ResourceTypeID: []string{"compute-node"},
			},
			resource: &Resource{
				ResourceID:     "node-1",
				ResourceTypeID: "compute-node",
			},
			expected: true,
		},
		{
			name: "non-matching resource type ID",
			filter: &Filter{
				ResourceTypeID: []string{"storage-node"},
			},
			resource: &Resource{
				ResourceID:     "node-1",
				ResourceTypeID: "compute-node",
			},
			expected: false,
		},
		{
			name: "matching resource pool ID",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1"},
			},
			resource: &Resource{
				ResourceID:     "node-1",
				ResourcePoolID: "pool-1",
			},
			expected: true,
		},
		{
			name: "multiple criteria all match",
			filter: &Filter{
				ResourceID:     []string{"node-1"},
				ResourceTypeID: []string{"compute-node"},
				ResourcePoolID: []string{"pool-1"},
			},
			resource: &Resource{
				ResourceID:     "node-1",
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

func TestFilter_MatchesResourceType(t *testing.T) {
	tests := []struct {
		name     string
		filter   *Filter
		rt       *ResourceType
		expected bool
	}{
		{
			name:   "empty filter matches all",
			filter: &Filter{},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
				ResourceClass:  "compute",
				ResourceKind:   "physical",
				Vendor:         "AWS",
				Model:          "m5.4xlarge",
			},
			expected: true,
		},
		{
			name: "matching resource type ID",
			filter: &Filter{
				ResourceTypeID: []string{"compute-node"},
			},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
			},
			expected: true,
		},
		{
			name: "non-matching resource type ID",
			filter: &Filter{
				ResourceTypeID: []string{"storage-node"},
			},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
			},
			expected: false,
		},
		{
			name: "matching resource class",
			filter: &Filter{
				ResourceClass: "compute",
			},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
				ResourceClass:  "compute",
			},
			expected: true,
		},
		{
			name: "non-matching resource class",
			filter: &Filter{
				ResourceClass: "storage",
			},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
				ResourceClass:  "compute",
			},
			expected: false,
		},
		{
			name: "matching resource kind",
			filter: &Filter{
				ResourceKind: "physical",
			},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
				ResourceKind:   "physical",
			},
			expected: true,
		},
		{
			name: "matching vendor",
			filter: &Filter{
				Vendor: "AWS",
			},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
				Vendor:         "AWS",
			},
			expected: true,
		},
		{
			name: "non-matching vendor",
			filter: &Filter{
				Vendor: "Azure",
			},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
				Vendor:         "AWS",
			},
			expected: false,
		},
		{
			name: "matching model",
			filter: &Filter{
				Model: "m5.4xlarge",
			},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
				Model:          "m5.4xlarge",
			},
			expected: true,
		},
		{
			name: "multiple criteria all match",
			filter: &Filter{
				ResourceClass: "compute",
				ResourceKind:  "physical",
				Vendor:        "AWS",
				Model:         "m5.4xlarge",
			},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
				ResourceClass:  "compute",
				ResourceKind:   "physical",
				Vendor:         "AWS",
				Model:          "m5.4xlarge",
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

func TestFilter_MatchesSubscription(t *testing.T) {
	tests := []struct {
		name     string
		filter   *Filter
		sub      *Subscription
		expected bool
	}{
		{
			name:   "empty filter matches all",
			filter: &Filter{},
			sub: &Subscription{
				SubscriptionID: "sub-1",
				Filter: &SubscriptionFilter{
					ResourcePoolID: []string{"pool-1"},
				},
			},
			expected: true,
		},
		{
			name: "filter with resource pool matches subscription with same pool",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1"},
			},
			sub: &Subscription{
				SubscriptionID: "sub-1",
				Filter: &SubscriptionFilter{
					ResourcePoolID: []string{"pool-1", "pool-2"},
				},
			},
			expected: true,
		},
		{
			name: "filter with resource pool does not match different pool",
			filter: &Filter{
				ResourcePoolID: []string{"pool-3"},
			},
			sub: &Subscription{
				SubscriptionID: "sub-1",
				Filter: &SubscriptionFilter{
					ResourcePoolID: []string{"pool-1", "pool-2"},
				},
			},
			expected: false,
		},
		{
			name: "subscription without filter matches",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1"},
			},
			sub: &Subscription{
				SubscriptionID: "sub-1",
			},
			expected: true,
		},
		{
			name: "subscription with empty pool filter matches",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1"},
			},
			sub: &Subscription{
				SubscriptionID: "sub-1",
				Filter:         &SubscriptionFilter{},
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

func TestFilter_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		filter   *Filter
		expected bool
	}{
		{
			name:     "empty filter",
			filter:   &Filter{},
			expected: true,
		},
		{
			name: "filter with empty collections",
			filter: &Filter{
				ResourcePoolID: []string{},
				Labels:         map[string]string{},
			},
			expected: true,
		},
		{
			name: "filter with resource pool ID",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1"},
			},
			expected: false,
		},
		{
			name: "filter with location",
			filter: &Filter{
				Location: "us-east",
			},
			expected: false,
		},
		{
			name: "filter with labels",
			filter: &Filter{
				Labels: map[string]string{"env": "prod"},
			},
			expected: false,
		},
		{
			name: "filter with resource class",
			filter: &Filter{
				ResourceClass: "compute",
			},
			expected: false,
		},
		{
			name: "filter with pagination only is empty",
			filter: &Filter{
				Limit:  100,
				Offset: 10,
			},
			expected: true, // Pagination doesn't count as filter criteria
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.IsEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilter_Clone(t *testing.T) {
	original := &Filter{
		ResourcePoolID: []string{"pool-1", "pool-2"},
		ResourceTypeID: []string{"compute-node"},
		ResourceID:     []string{"node-1"},
		Location:       "us-east-1a",
		OCloudID:       "ocloud-1",
		Labels: map[string]string{
			"env":  "production",
			"tier": "frontend",
		},
		ResourceClass: "compute",
		ResourceKind:  "physical",
		Vendor:        "AWS",
		Model:         "m5.4xlarge",
		Limit:         50,
		Offset:        10,
		SortBy:        "name",
		SortOrder:     "desc",
		Extensions: map[string]interface{}{
			"custom": "value",
		},
	}

	clone := original.Clone()

	// Verify all fields are copied
	assert.Equal(t, original.ResourcePoolID, clone.ResourcePoolID)
	assert.Equal(t, original.ResourceTypeID, clone.ResourceTypeID)
	assert.Equal(t, original.ResourceID, clone.ResourceID)
	assert.Equal(t, original.Location, clone.Location)
	assert.Equal(t, original.OCloudID, clone.OCloudID)
	assert.Equal(t, original.Labels, clone.Labels)
	assert.Equal(t, original.ResourceClass, clone.ResourceClass)
	assert.Equal(t, original.ResourceKind, clone.ResourceKind)
	assert.Equal(t, original.Vendor, clone.Vendor)
	assert.Equal(t, original.Model, clone.Model)
	assert.Equal(t, original.Limit, clone.Limit)
	assert.Equal(t, original.Offset, clone.Offset)
	assert.Equal(t, original.SortBy, clone.SortBy)
	assert.Equal(t, original.SortOrder, clone.SortOrder)
	assert.Equal(t, original.Extensions, clone.Extensions)

	// Verify it's a deep copy (modifying clone doesn't affect original)
	clone.ResourcePoolID[0] = "modified-pool"
	assert.NotEqual(t, original.ResourcePoolID[0], clone.ResourcePoolID[0])

	clone.Labels["new-key"] = "new-value"
	_, exists := original.Labels["new-key"]
	assert.False(t, exists)
}

func TestFilter_Clone_Empty(t *testing.T) {
	original := &Filter{}
	clone := original.Clone()

	require.NotNil(t, clone)
	assert.NotNil(t, clone.Labels)
	assert.NotNil(t, clone.Extensions)
	assert.Empty(t, clone.ResourcePoolID)
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "item exists",
			slice:    []string{"a", "b", "c"},
			item:     "b",
			expected: true,
		},
		{
			name:     "item does not exist",
			slice:    []string{"a", "b", "c"},
			item:     "d",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "a",
			expected: false,
		},
		{
			name:     "first item",
			slice:    []string{"a", "b", "c"},
			item:     "a",
			expected: true,
		},
		{
			name:     "last item",
			slice:    []string{"a", "b", "c"},
			item:     "c",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.item)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseQueryParams_RoundTrip(t *testing.T) {
	// Create a filter with all fields
	original := &Filter{
		ResourcePoolID: []string{"pool-1"},
		ResourceTypeID: []string{"compute-node"},
		ResourceID:     []string{"node-1"},
		Location:       "us-east-1a",
		OCloudID:       "ocloud-1",
		ResourceClass:  "compute",
		ResourceKind:   "physical",
		Vendor:         "AWS",
		Model:          "m5.4xlarge",
		Labels:         map[string]string{"env": "prod"},
		Limit:          50,
		Offset:         10,
		SortBy:         "name",
		SortOrder:      "desc",
	}

	// Convert to query params and back
	params := original.ToQueryParams()
	parsed := ParseQueryParams(params)

	// Verify round-trip
	assert.Equal(t, original.ResourcePoolID, parsed.ResourcePoolID)
	assert.Equal(t, original.ResourceTypeID, parsed.ResourceTypeID)
	assert.Equal(t, original.ResourceID, parsed.ResourceID)
	assert.Equal(t, original.Location, parsed.Location)
	assert.Equal(t, original.OCloudID, parsed.OCloudID)
	assert.Equal(t, original.ResourceClass, parsed.ResourceClass)
	assert.Equal(t, original.ResourceKind, parsed.ResourceKind)
	assert.Equal(t, original.Vendor, parsed.Vendor)
	assert.Equal(t, original.Model, parsed.Model)
	assert.Equal(t, original.Labels, parsed.Labels)
	assert.Equal(t, original.Limit, parsed.Limit)
	assert.Equal(t, original.Offset, parsed.Offset)
	assert.Equal(t, original.SortBy, parsed.SortBy)
	assert.Equal(t, original.SortOrder, parsed.SortOrder)
}
