package models

import (
	"net/url"
	"reflect"
	"testing"
)

func TestParseQueryParams(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected *Filter
	}{
		{
			name:  "empty query",
			query: "",
			expected: &Filter{
				Limit:      100,
				SortOrder:  "asc",
				Labels:     map[string]string{},
				Extensions: map[string]interface{}{},
			},
		},
		{
			name:  "single resource pool ID",
			query: "resourcePoolId=pool-1",
			expected: &Filter{
				ResourcePoolID: []string{"pool-1"},
				Limit:          100,
				SortOrder:      "asc",
				Labels:         map[string]string{},
				Extensions:     map[string]interface{}{},
			},
		},
		{
			name:  "multiple resource pool IDs",
			query: "resourcePoolId=pool-1&resourcePoolId=pool-2",
			expected: &Filter{
				ResourcePoolID: []string{"pool-1", "pool-2"},
				Limit:          100,
				SortOrder:      "asc",
				Labels:         map[string]string{},
				Extensions:     map[string]interface{}{},
			},
		},
		{
			name:  "location filter",
			query: "location=us-east-1a",
			expected: &Filter{
				Location:   "us-east-1a",
				Limit:      100,
				SortOrder:  "asc",
				Labels:     map[string]string{},
				Extensions: map[string]interface{}{},
			},
		},
		{
			name:  "labels filter",
			query: "labels=env:prod,tier:gold",
			expected: &Filter{
				Limit:     100,
				SortOrder: "asc",
				Labels: map[string]string{
					"env":  "prod",
					"tier": "gold",
				},
				Extensions: map[string]interface{}{},
			},
		},
		{
			name:  "custom limit",
			query: "limit=50",
			expected: &Filter{
				Limit:      50,
				SortOrder:  "asc",
				Labels:     map[string]string{},
				Extensions: map[string]interface{}{},
			},
		},
		{
			name:  "limit exceeds max",
			query: "limit=2000",
			expected: &Filter{
				Limit:      1000, // Should be capped at max
				SortOrder:  "asc",
				Labels:     map[string]string{},
				Extensions: map[string]interface{}{},
			},
		},
		{
			name:  "offset and sorting",
			query: "offset=100&sortBy=name&sortOrder=desc",
			expected: &Filter{
				Limit:      100,
				Offset:     100,
				SortBy:     "name",
				SortOrder:  "desc",
				Labels:     map[string]string{},
				Extensions: map[string]interface{}{},
			},
		},
		{
			name:  "resource class and kind",
			query: "resourceClass=compute&resourceKind=physical",
			expected: &Filter{
				ResourceClass: "compute",
				ResourceKind:  "physical",
				Limit:         100,
				SortOrder:     "asc",
				Labels:        map[string]string{},
				Extensions:    map[string]interface{}{},
			},
		},
		{
			name:  "vendor and model",
			query: "vendor=AWS&model=m5.4xlarge",
			expected: &Filter{
				Vendor:     "AWS",
				Model:      "m5.4xlarge",
				Limit:      100,
				SortOrder:  "asc",
				Labels:     map[string]string{},
				Extensions: map[string]interface{}{},
			},
		},
		{
			name:  "invalid sort order falls back to asc",
			query: "sortOrder=invalid",
			expected: &Filter{
				Limit:      100,
				SortOrder:  "asc",
				Labels:     map[string]string{},
				Extensions: map[string]interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("Failed to parse query: %v", err)
			}

			result := ParseQueryParams(params)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseQueryParams() = %+v, want %+v", result, tt.expected)
			}
		})
	}
}

func TestFilter_ToQueryParams(t *testing.T) {
	tests := []struct {
		name     string
		filter   *Filter
		expected map[string][]string
	}{
		{
			name:     "empty filter",
			filter:   &Filter{},
			expected: map[string][]string{},
		},
		{
			name: "resource pool IDs",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1", "pool-2"},
			},
			expected: map[string][]string{
				"resourcePoolId": {"pool-1", "pool-2"},
			},
		},
		{
			name: "location and limit",
			filter: &Filter{
				Location: "us-east-1a",
				Limit:    50,
			},
			expected: map[string][]string{
				"location": {"us-east-1a"},
				"limit":    {"50"},
			},
		},
		{
			name: "labels",
			filter: &Filter{
				Labels: map[string]string{
					"env":  "prod",
					"tier": "gold",
				},
			},
			expected: map[string][]string{
				"labels": {"env:prod,tier:gold", "tier:gold,env:prod"}, // Order may vary
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.ToQueryParams()

			for key, expectedValues := range tt.expected {
				resultValues, ok := result[key]
				if !ok {
					t.Errorf("Expected key %s not found in result", key)
					continue
				}

				// For labels, check if result matches any expected permutation
				if key == "labels" {
					matchFound := false
					for _, expectedValue := range expectedValues {
						if len(resultValues) == 1 && resultValues[0] == expectedValue {
							matchFound = true
							break
						}
					}
					if !matchFound {
						t.Errorf("Result value %v for key %s doesn't match any expected permutation %v",
							resultValues, key, expectedValues)
					}
				} else if !reflect.DeepEqual(resultValues, expectedValues) {
					t.Errorf("Result value %v for key %s doesn't match expected %v",
						resultValues, key, expectedValues)
				}
			}
		})
	}
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
			name: "matching pool ID",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1", "pool-2"},
			},
			pool: &ResourcePool{
				ResourcePoolID: "pool-1",
			},
			expected: true,
		},
		{
			name: "non-matching pool ID",
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
			name: "non-matching location",
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
				OCloudID: "ocloud-2",
			},
			pool: &ResourcePool{
				ResourcePoolID: "pool-1",
				OCloudID:       "ocloud-1",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.MatchesResourcePool(tt.pool)
			if result != tt.expected {
				t.Errorf("Filter.MatchesResourcePool() = %v, want %v", result, tt.expected)
			}
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
				ResourceID:     "resource-1",
				ResourceTypeID: "compute-node",
				ResourcePoolID: "pool-1",
			},
			expected: true,
		},
		{
			name: "matching resource ID",
			filter: &Filter{
				ResourceID: []string{"resource-1"},
			},
			resource: &Resource{
				ResourceID: "resource-1",
			},
			expected: true,
		},
		{
			name: "non-matching resource ID",
			filter: &Filter{
				ResourceID: []string{"resource-2"},
			},
			resource: &Resource{
				ResourceID: "resource-1",
			},
			expected: false,
		},
		{
			name: "matching resource type ID",
			filter: &Filter{
				ResourceTypeID: []string{"compute-node"},
			},
			resource: &Resource{
				ResourceID:     "resource-1",
				ResourceTypeID: "compute-node",
			},
			expected: true,
		},
		{
			name: "matching resource pool ID",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1"},
			},
			resource: &Resource{
				ResourceID:     "resource-1",
				ResourcePoolID: "pool-1",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.MatchesResource(tt.resource)
			if result != tt.expected {
				t.Errorf("Filter.MatchesResource() = %v, want %v", result, tt.expected)
			}
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
			name: "matching vendor and model",
			filter: &Filter{
				Vendor: "AWS",
				Model:  "m5.4xlarge",
			},
			rt: &ResourceType{
				ResourceTypeID: "compute-node",
				Vendor:         "AWS",
				Model:          "m5.4xlarge",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.MatchesResourceType(tt.rt)
			if result != tt.expected {
				t.Errorf("Filter.MatchesResourceType() = %v, want %v", result, tt.expected)
			}
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
			},
			expected: true,
		},
		{
			name: "matching resource pool ID in subscription filter",
			filter: &Filter{
				ResourcePoolID: []string{"pool-1"},
			},
			sub: &Subscription{
				SubscriptionID: "sub-1",
				Filter: &SubscriptionFilter{
					ResourcePoolID: []string{"pool-1"},
				},
			},
			expected: true,
		},
		{
			name: "non-matching resource pool ID",
			filter: &Filter{
				ResourcePoolID: []string{"pool-2"},
			},
			sub: &Subscription{
				SubscriptionID: "sub-1",
				Filter: &SubscriptionFilter{
					ResourcePoolID: []string{"pool-1"},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.MatchesSubscription(tt.sub)
			if result != tt.expected {
				t.Errorf("Filter.MatchesSubscription() = %v, want %v", result, tt.expected)
			}
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
			name:     "completely empty filter",
			filter:   &Filter{},
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
				Location: "us-east-1a",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.IsEmpty()
			if result != tt.expected {
				t.Errorf("Filter.IsEmpty() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilter_Clone(t *testing.T) {
	original := &Filter{
		ResourcePoolID: []string{"pool-1", "pool-2"},
		ResourceTypeID: []string{"type-1"},
		ResourceID:     []string{"res-1"},
		Location:       "us-east-1a",
		OCloudID:       "ocloud-1",
		Labels: map[string]string{
			"env": "prod",
		},
		ResourceClass: "compute",
		ResourceKind:  "physical",
		Vendor:        "AWS",
		Model:         "m5.4xlarge",
		Limit:         50,
		Offset:        100,
		SortBy:        "name",
		SortOrder:     "desc",
		Extensions: map[string]interface{}{
			"custom": "value",
		},
	}

	clone := original.Clone()

	// Verify all fields are equal
	if len(clone.ResourcePoolID) != len(original.ResourcePoolID) {
		t.Errorf("ResourcePoolID length mismatch")
	}
	if len(clone.ResourceTypeID) != len(original.ResourceTypeID) {
		t.Errorf("ResourceTypeID length mismatch")
	}
	if len(clone.ResourceID) != len(original.ResourceID) {
		t.Errorf("ResourceID length mismatch")
	}
	if clone.Location != original.Location {
		t.Errorf("Location mismatch")
	}
	if clone.OCloudID != original.OCloudID {
		t.Errorf("OCloudID mismatch")
	}
	if len(clone.Labels) != len(original.Labels) {
		t.Errorf("Labels length mismatch")
	}
	if clone.ResourceClass != original.ResourceClass {
		t.Errorf("ResourceClass mismatch")
	}
	if clone.Limit != original.Limit {
		t.Errorf("Limit mismatch")
	}

	// Verify it's a deep copy (modify clone shouldn't affect original)
	clone.ResourcePoolID[0] = "modified"
	if original.ResourcePoolID[0] == "modified" {
		t.Errorf("Clone is not a deep copy - original was modified")
	}

	clone.Labels["env"] = "dev"
	if original.Labels["env"] == "dev" {
		t.Errorf("Clone is not a deep copy - original labels were modified")
	}
}

func Test_contains(t *testing.T) {
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
			name:     "item doesn't exist",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("contains() = %v, want %v", result, tt.expected)
			}
		})
	}
}
