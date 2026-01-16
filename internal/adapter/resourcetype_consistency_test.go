//go:build integration

package adapter_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
)

// TestAllAdapters_ResourceTypeConsistency verifies that all adapters
// return consistent data structures and follow the same conventions.
func TestAllAdapters_ResourceTypeConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	adapters := []struct {
		name    string
		adapter adapter.Adapter
		skip    bool
		reason  string
	}{
		{
			name:    "kubernetes",
			adapter: setupKubernetesAdapter(t),
			skip:    false,
		},
		{
			name:    "aws",
			adapter: setupAWSAdapter(t),
			skip:    true,
			reason:  "Requires AWS credentials",
		},
		{
			name:    "azure",
			adapter: setupAzureAdapter(t),
			skip:    true,
			reason:  "Requires Azure credentials",
		},
		{
			name:    "gcp",
			adapter: setupGCPAdapter(t),
			skip:    true,
			reason:  "Requires GCP credentials",
		},
		{
			name:    "openstack",
			adapter: setupOpenStackAdapter(t),
			skip:    true,
			reason:  "Requires OpenStack environment",
		},
		{
			name:    "vmware",
			adapter: setupVMwareAdapter(t),
			skip:    true,
			reason:  "Requires VMware environment",
		},
		{
			name:    "dtias",
			adapter: setupDTIASAdapter(t),
			skip:    true,
			reason:  "Requires DTIAS environment",
		},
	}

	for _, tt := range adapters {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip(tt.reason)
			}

			if tt.adapter == nil {
				t.Skip("Adapter not available")
			}

			testResourceTypeStructure(t, tt.adapter)
			testResourceTypeIDFormat(t, tt.adapter)
			testResourceTypeExtensions(t, tt.adapter)
		})
	}
}

// testResourceTypeStructure verifies that all resource types have required fields.
func testResourceTypeStructure(t *testing.T, adp adapter.Adapter) {
	t.Helper()
	ctx := context.Background()

	types, err := adp.ListResourceTypes(ctx, nil)
	require.NoError(t, err, "ListResourceTypes should not fail")
	require.NotEmpty(t, types, "adapter should return at least one resource type")

	for i, rt := range types {
		// Verify required fields
		assert.NotEmpty(t, rt.ResourceTypeID, "type %d: ResourceTypeID is required", i)
		assert.NotEmpty(t, rt.Name, "type %d: Name is required", i)

		// ResourceClass must be valid if set
		if rt.ResourceClass != "" {
			validClasses := []string{"compute", "storage", "network"}
			assert.Contains(t, validClasses, rt.ResourceClass,
				"type %d: ResourceClass must be compute, storage, or network", i)
		}

		// ResourceKind must be valid if set
		if rt.ResourceKind != "" {
			validKinds := []string{"physical", "virtual"}
			assert.Contains(t, validKinds, rt.ResourceKind,
				"type %d: ResourceKind must be physical or virtual", i)
		}

		// Extensions should not be nil
		assert.NotNil(t, rt.Extensions, "type %d: Extensions must not be nil", i)
	}
}

// testResourceTypeIDFormat verifies that resource type IDs follow consistent naming.
func testResourceTypeIDFormat(t *testing.T, adp adapter.Adapter) {
	t.Helper()
	ctx := context.Background()

	types, err := adp.ListResourceTypes(ctx, nil)
	require.NoError(t, err)

	for i, rt := range types {
		// ID should not be empty
		assert.NotEmpty(t, rt.ResourceTypeID, "type %d: ID cannot be empty", i)

		// ID should not contain spaces
		assert.NotContains(t, rt.ResourceTypeID, " ",
			"type %d: ID should not contain spaces", i)

		// ID should be unique within the list
		for j, other := range types {
			if i != j {
				assert.NotEqual(t, rt.ResourceTypeID, other.ResourceTypeID,
					"types %d and %d have duplicate IDs", i, j)
			}
		}
	}
}

// testResourceTypeExtensions verifies that extensions are properly structured.
func testResourceTypeExtensions(t *testing.T, adp adapter.Adapter) {
	t.Helper()
	ctx := context.Background()

	types, err := adp.ListResourceTypes(ctx, nil)
	require.NoError(t, err)

	for i, rt := range types {
		// Extensions should not be nil
		require.NotNil(t, rt.Extensions, "type %d: Extensions cannot be nil", i)

		// Extensions should be a valid map structure
		assert.IsType(t, map[string]interface{}{}, rt.Extensions,
			"type %d: Extensions should be a map", i)
	}
}

// TestResourceType_GetConsistency verifies that Get returns the same
// data as List for the same resource type.
func TestResourceType_GetConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	adp := setupKubernetesAdapter(t)
	if adp == nil {
		t.Skip("Kubernetes adapter not available")
	}

	ctx := context.Background()

	// Get resource types from List
	types, err := adp.ListResourceTypes(ctx, nil)
	require.NoError(t, err)
	require.NotEmpty(t, types)

	// Get the same type using Get
	typeID := types[0].ResourceTypeID
	retrieved, err := adp.GetResourceType(ctx, typeID)
	require.NoError(t, err)

	// Verify consistency
	assert.Equal(t, types[0].ResourceTypeID, retrieved.ResourceTypeID,
		"Get should return same ResourceTypeID as List")
	assert.Equal(t, types[0].Name, retrieved.Name,
		"Get should return same Name as List")
	assert.Equal(t, types[0].ResourceClass, retrieved.ResourceClass,
		"Get should return same ResourceClass as List")
	assert.Equal(t, types[0].ResourceKind, retrieved.ResourceKind,
		"Get should return same ResourceKind as List")
}

// TestResourceType_PaginationConsistency verifies that pagination
// returns consistent results.
func TestResourceType_PaginationConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	adp := setupKubernetesAdapter(t)
	if adp == nil {
		t.Skip("Kubernetes adapter not available")
	}

	ctx := context.Background()

	// Get all types
	allTypes, err := adp.ListResourceTypes(ctx, nil)
	require.NoError(t, err)

	if len(allTypes) < 2 {
		t.Skip("Need at least 2 resource types to test pagination")
	}

	// Get first page
	page1, err := adp.ListResourceTypes(ctx, &adapter.Filter{Limit: 1, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 1, "first page should have 1 item")

	// Get second page
	page2, err := adp.ListResourceTypes(ctx, &adapter.Filter{Limit: 1, Offset: 1})
	require.NoError(t, err)
	assert.Len(t, page2, 1, "second page should have 1 item")

	// Verify pages contain different types
	assert.NotEqual(t, page1[0].ResourceTypeID, page2[0].ResourceTypeID,
		"pages should contain different resource types")

	// Verify page results match full list
	assert.Equal(t, allTypes[0].ResourceTypeID, page1[0].ResourceTypeID,
		"first page should match first item from full list")
	assert.Equal(t, allTypes[1].ResourceTypeID, page2[0].ResourceTypeID,
		"second page should match second item from full list")
}
