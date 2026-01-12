package models_test

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
		{
			name: "sorting parameters",
			filter: &Filter{
				SortBy:    "name",
				SortOrder: "desc",
			},
			expected: map[string][]string{
				"sortBy":    {"name"},
				"sortOrder": {"desc"},
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

	// Verify all fields are copied correctly
	verifyClonedFields(t, original, clone)

	// Verify it's a deep copy (modify clone shouldn't affect original)
	verifyDeepCopy(t, original, clone)
}

// verifyClonedFields checks that all fields in clone match original.
func verifyClonedFields(t *testing.T, original, clone *Filter) {
	t.Helper()

	if len(clone.ResourcePoolID) != len(original.ResourcePoolID) {
		t.Errorf("ResourcePoolID length mismatch: got %d, want %d", len(clone.ResourcePoolID), len(original.ResourcePoolID))
	}
	if len(clone.ResourceTypeID) != len(original.ResourceTypeID) {
		t.Errorf("ResourceTypeID length mismatch: got %d, want %d", len(clone.ResourceTypeID), len(original.ResourceTypeID))
	}
	if len(clone.ResourceID) != len(original.ResourceID) {
		t.Errorf("ResourceID length mismatch: got %d, want %d", len(clone.ResourceID), len(original.ResourceID))
	}
	if clone.Location != original.Location {
		t.Errorf("Location mismatch: got %s, want %s", clone.Location, original.Location)
	}
	if clone.OCloudID != original.OCloudID {
		t.Errorf("OCloudID mismatch: got %s, want %s", clone.OCloudID, original.OCloudID)
	}
	if len(clone.Labels) != len(original.Labels) {
		t.Errorf("Labels length mismatch: got %d, want %d", len(clone.Labels), len(original.Labels))
	}
	if clone.ResourceClass != original.ResourceClass {
		t.Errorf("ResourceClass mismatch: got %s, want %s", clone.ResourceClass, original.ResourceClass)
	}
	if clone.Limit != original.Limit {
		t.Errorf("Limit mismatch: got %d, want %d", clone.Limit, original.Limit)
	}
}

// verifyDeepCopy checks that modifications to clone don't affect original.
func verifyDeepCopy(t *testing.T, original, clone *Filter) {
	t.Helper()

	const modifiedValue = "modified"

	clone.ResourcePoolID[0] = modifiedValue
	if original.ResourcePoolID[0] == modifiedValue {
		t.Errorf("Clone is not a deep copy - original.ResourcePoolID was modified")
	}

	clone.Labels["env"] = "dev"
	if original.Labels["env"] == "dev" {
		t.Errorf("Clone is not a deep copy - original.Labels was modified")
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

// Tests for Issue #171: Field Selection

func TestParseQueryParams_FieldSelection(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedFields []string
	}{
		{
			name:           "single field",
			query:          "fields=resourceId",
			expectedFields: []string{"resourceId"},
		},
		{
			name:           "multiple fields",
			query:          "fields=resourceId,name,extensions",
			expectedFields: []string{"resourceId", "name", "extensions"},
		},
		{
			name:           "fields with spaces",
			query:          "fields=resourceId, name, extensions",
			expectedFields: []string{"resourceId", "name", "extensions"},
		},
		{
			name:           "nested fields",
			query:          "fields=resourceId,extensions.cpu,extensions.memory",
			expectedFields: []string{"resourceId", "extensions.cpu", "extensions.memory"},
		},
		{
			name:           "empty fields value returns nil",
			query:          "fields=",
			expectedFields: nil,
		},
		{
			name:           "fields with empty entries",
			query:          "fields=resourceId,,name",
			expectedFields: []string{"resourceId", "name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("Failed to parse query: %v", err)
			}

			filter := ParseQueryParams(params)

			if !reflect.DeepEqual(filter.Fields, tt.expectedFields) {
				t.Errorf("ParseQueryParams().Fields = %v, want %v", filter.Fields, tt.expectedFields)
			}
		})
	}
}

func TestFilter_HasFieldSelection(t *testing.T) {
	tests := []struct {
		name     string
		filter   *Filter
		expected bool
	}{
		{
			name:     "nil fields",
			filter:   &Filter{Fields: nil},
			expected: false,
		},
		{
			name:     "empty fields",
			filter:   &Filter{Fields: []string{}},
			expected: false,
		},
		{
			name:     "has fields",
			filter:   &Filter{Fields: []string{"resourceId", "name"}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.HasFieldSelection()
			if result != tt.expected {
				t.Errorf("Filter.HasFieldSelection() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilter_ShouldIncludeField(t *testing.T) {
	tests := []struct {
		name      string
		filter    *Filter
		fieldName string
		expected  bool
	}{
		{
			name:      "no field selection includes all",
			filter:    &Filter{Fields: nil},
			fieldName: "anyField",
			expected:  true,
		},
		{
			name:      "exact match",
			filter:    &Filter{Fields: []string{"resourceId", "name"}},
			fieldName: "resourceId",
			expected:  true,
		},
		{
			name:      "field not in list",
			filter:    &Filter{Fields: []string{"resourceId", "name"}},
			fieldName: "description",
			expected:  false,
		},
		{
			name:      "nested field prefix match",
			filter:    &Filter{Fields: []string{"extensions.cpu"}},
			fieldName: "extensions.cpu.cores",
			expected:  true,
		},
		{
			name:      "parent field includes nested",
			filter:    &Filter{Fields: []string{"extensions.cpu"}},
			fieldName: "extensions",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.ShouldIncludeField(tt.fieldName)
			if result != tt.expected {
				t.Errorf("Filter.ShouldIncludeField(%q) = %v, want %v", tt.fieldName, result, tt.expected)
			}
		})
	}
}

func TestFilter_SelectFields(t *testing.T) {
	tests := []struct {
		name     string
		filter   *Filter
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:   "no field selection returns all",
			filter: &Filter{Fields: nil},
			input: map[string]interface{}{
				"id":   "123",
				"name": "test",
			},
			expected: map[string]interface{}{
				"id":   "123",
				"name": "test",
			},
		},
		{
			name:   "select specific fields",
			filter: &Filter{Fields: []string{"id"}},
			input: map[string]interface{}{
				"id":   "123",
				"name": "test",
			},
			expected: map[string]interface{}{
				"id": "123",
			},
		},
		{
			name:   "select multiple fields",
			filter: &Filter{Fields: []string{"id", "name"}},
			input: map[string]interface{}{
				"id":          "123",
				"name":        "test",
				"description": "desc",
			},
			expected: map[string]interface{}{
				"id":   "123",
				"name": "test",
			},
		},
		{
			name:   "select nested field",
			filter: &Filter{Fields: []string{"extensions.cpu"}},
			input: map[string]interface{}{
				"id": "123",
				"extensions": map[string]interface{}{
					"cpu":    "4 cores",
					"memory": "16GB",
				},
			},
			expected: map[string]interface{}{
				"extensions": map[string]interface{}{
					"cpu": "4 cores",
				},
			},
		},
		{
			name:   "field not found is ignored",
			filter: &Filter{Fields: []string{"nonexistent"}},
			input: map[string]interface{}{
				"id":   "123",
				"name": "test",
			},
			expected: map[string]interface{}{},
		},
		{
			name:   "deeply nested field selection (5 levels)",
			filter: &Filter{Fields: []string{"level1.level2.level3.level4.level5"}},
			input: map[string]interface{}{
				"id": "root",
				"level1": map[string]interface{}{
					"data": "level1-data",
					"level2": map[string]interface{}{
						"data": "level2-data",
						"level3": map[string]interface{}{
							"data": "level3-data",
							"level4": map[string]interface{}{
								"data": "level4-data",
								"level5": map[string]interface{}{
									"target": "found",
									"other":  "ignored",
								},
								"sibling": "ignored",
							},
							"sibling": "ignored",
						},
						"sibling": "ignored",
					},
					"sibling": "ignored",
				},
			},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"level4": map[string]interface{}{
								"level5": map[string]interface{}{
									"target": "found",
									"other":  "ignored",
								},
							},
						},
					},
				},
			},
		},
		{
			name:   "multiple deeply nested fields",
			filter: &Filter{Fields: []string{"a.b.c.value", "x.y.z.value"}},
			input: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": map[string]interface{}{
							"value": "abc-value",
							"other": "ignored",
						},
					},
				},
				"x": map[string]interface{}{
					"y": map[string]interface{}{
						"z": map[string]interface{}{
							"value": "xyz-value",
							"other": "ignored",
						},
					},
				},
				"top": "ignored",
			},
			expected: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": map[string]interface{}{
							"value": "abc-value",
						},
					},
				},
				"x": map[string]interface{}{
					"y": map[string]interface{}{
						"z": map[string]interface{}{
							"value": "xyz-value",
						},
					},
				},
			},
		},
		{
			name:   "deeply nested with arrays",
			filter: &Filter{Fields: []string{"data.items"}},
			input: map[string]interface{}{
				"id": "123",
				"data": map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{"id": "1", "name": "first"},
						map[string]interface{}{"id": "2", "name": "second"},
					},
					"metadata": "ignored",
				},
			},
			expected: map[string]interface{}{
				"data": map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{"id": "1", "name": "first"},
						map[string]interface{}{"id": "2", "name": "second"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.SelectFields(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Filter.SelectFields() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilter_ToQueryParams_WithFields(t *testing.T) {
	filter := &Filter{
		ResourcePoolID: []string{"pool-1"},
		Limit:          50,
		Offset:         10,
		SortBy:         "name",
		SortOrder:      "desc",
		Fields:         []string{"id", "name", "extensions.cpu"},
	}

	result := filter.ToQueryParams()

	// Check fields parameter
	fields, ok := result["fields"]
	if !ok {
		t.Fatal("Expected 'fields' key in result")
	}
	if len(fields) != 1 {
		t.Errorf("Expected single fields value, got %d", len(fields))
	}

	expectedFields := "id,name,extensions.cpu"
	if fields[0] != expectedFields {
		t.Errorf("fields = %q, want %q", fields[0], expectedFields)
	}
}

func TestFilter_Clone_WithFields(t *testing.T) {
	original := &Filter{
		ResourcePoolID: []string{"pool-1"},
		Location:       "us-east-1a",
		Fields:         []string{"id", "name", "extensions"},
	}

	clone := original.Clone()

	// Verify fields are equal
	if !reflect.DeepEqual(clone.Fields, original.Fields) {
		t.Errorf("Clone Fields = %v, want %v", clone.Fields, original.Fields)
	}

	// Verify it's a deep copy
	clone.Fields[0] = "modified"
	if original.Fields[0] == "modified" {
		t.Error("Clone is not a deep copy - original Fields were modified")
	}
}
