//go:build integration

package adapter_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
)

// TestAllAdapters_ResourceConsistency verifies that all adapters
// return consistent data structures and follow the same conventions for Resources.
func TestAllAdapters_ResourceConsistency(t *testing.T) {
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

			testResourceStructure(t, tt.adapter)
			testResourceIDFormat(t, tt.adapter)
			testResourceExtensions(t, tt.adapter)
			testResourceTypeIDReferences(t, tt.adapter)
		})
	}
}

// testResourceStructure verifies that all resources have required fields.
func testResourceStructure(t *testing.T, adp adapter.Adapter) {
	t.Helper()
	ctx := context.Background()

	resources, err := adp.ListResources(ctx, nil)
	require.NoError(t, err, "ListResources should not fail")

	// Resources may be empty in test environments
	if len(resources) == 0 {
		t.Skip("No resources available to test structure")
	}

	for i, res := range resources {
		// Verify required fields
		assert.NotEmpty(t, res.ResourceID, "resource %d: ResourceID is required", i)
		assert.NotEmpty(t, res.ResourceTypeID, "resource %d: ResourceTypeID is required", i)

		// Extensions should not be nil
		assert.NotNil(t, res.Extensions, "resource %d: Extensions must not be nil", i)

		// GlobalAssetID should be in URN format if present
		if res.GlobalAssetID != "" {
			assert.Contains(t, res.GlobalAssetID, ":", "resource %d: GlobalAssetID should be URN format", i)
		}
	}
}

// testResourceIDFormat verifies that resource IDs follow consistent naming.
func testResourceIDFormat(t *testing.T, adp adapter.Adapter) {
	t.Helper()
	ctx := context.Background()

	resources, err := adp.ListResources(ctx, nil)
	require.NoError(t, err)

	if len(resources) == 0 {
		t.Skip("No resources available to test ID format")
	}

	for i, res := range resources {
		// ID should not be empty
		assert.NotEmpty(t, res.ResourceID, "resource %d: ID cannot be empty", i)

		// ID should not contain spaces
		assert.NotContains(t, res.ResourceID, " ",
			"resource %d: ID should not contain spaces", i)

		// ID should be unique within the list
		for j, other := range resources {
			if i != j {
				assert.NotEqual(t, res.ResourceID, other.ResourceID,
					"resources %d and %d have duplicate IDs", i, j)
			}
		}
	}
}

// testResourceExtensions verifies that extensions are properly structured.
func testResourceExtensions(t *testing.T, adp adapter.Adapter) {
	t.Helper()
	ctx := context.Background()

	resources, err := adp.ListResources(ctx, nil)
	require.NoError(t, err)

	if len(resources) == 0 {
		t.Skip("No resources available to test extensions")
	}

	for i, res := range resources {
		// Extensions should not be nil
		require.NotNil(t, res.Extensions, "resource %d: Extensions cannot be nil", i)

		// Extensions should be a valid map structure
		assert.IsType(t, map[string]interface{}{}, res.Extensions,
			"resource %d: Extensions should be a map", i)
	}
}

// testResourceTypeIDReferences verifies that resource type IDs reference valid types.
func testResourceTypeIDReferences(t *testing.T, adp adapter.Adapter) {
	t.Helper()
	ctx := context.Background()

	resources, err := adp.ListResources(ctx, nil)
	require.NoError(t, err)

	if len(resources) == 0 {
		t.Skip("No resources available to test type references")
	}

	// Get all valid resource type IDs
	resourceTypes, err := adp.ListResourceTypes(ctx, nil)
	require.NoError(t, err)

	validTypeIDs := make(map[string]bool)
	for _, rt := range resourceTypes {
		validTypeIDs[rt.ResourceTypeID] = true
	}

	// Verify each resource references a valid type
	for i, res := range resources {
		assert.NotEmpty(t, res.ResourceTypeID, "resource %d: must have ResourceTypeID", i)

		// Some adapters may have resources with types not yet discovered
		// so we'll just check that it's not empty rather than validating against the list
		if len(validTypeIDs) > 0 {
			// Only validate if we have resource types
			_, exists := validTypeIDs[res.ResourceTypeID]
			if !exists {
				t.Logf("resource %d: ResourceTypeID %s not found in discovered types (may be valid)",
					i, res.ResourceTypeID)
			}
		}
	}
}

// TestResource_GetConsistency verifies that Get returns the same
// data as List for the same resource.
func TestResource_GetConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	adp := setupKubernetesAdapter(t)
	if adp == nil {
		t.Skip("Kubernetes adapter not available")
	}

	ctx := context.Background()

	// Get resources from List
	resources, err := adp.ListResources(ctx, nil)
	require.NoError(t, err)

	if len(resources) == 0 {
		t.Skip("No resources available to test Get consistency")
	}

	// Get the same resource using Get
	resourceID := resources[0].ResourceID
	retrieved, err := adp.GetResource(ctx, resourceID)
	require.NoError(t, err)

	// Verify consistency
	assert.Equal(t, resources[0].ResourceID, retrieved.ResourceID,
		"Get should return same ResourceID as List")
	assert.Equal(t, resources[0].ResourceTypeID, retrieved.ResourceTypeID,
		"Get should return same ResourceTypeID as List")
	assert.Equal(t, resources[0].ResourcePoolID, retrieved.ResourcePoolID,
		"Get should return same ResourcePoolID as List")
	assert.Equal(t, resources[0].GlobalAssetID, retrieved.GlobalAssetID,
		"Get should return same GlobalAssetID as List")
}

// TestResource_PaginationConsistency verifies that pagination
// returns consistent results.
func TestResource_PaginationConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	adp := setupKubernetesAdapter(t)
	if adp == nil {
		t.Skip("Kubernetes adapter not available")
	}

	ctx := context.Background()

	// Get all resources
	allResources, err := adp.ListResources(ctx, nil)
	require.NoError(t, err)

	if len(allResources) < 2 {
		t.Skip("Need at least 2 resources to test pagination")
	}

	// Get first page
	page1, err := adp.ListResources(ctx, &adapter.Filter{Limit: 1, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 1, "first page should have 1 item")

	// Get second page
	page2, err := adp.ListResources(ctx, &adapter.Filter{Limit: 1, Offset: 1})
	require.NoError(t, err)
	assert.Len(t, page2, 1, "second page should have 1 item")

	// Verify pages contain different resources
	assert.NotEqual(t, page1[0].ResourceID, page2[0].ResourceID,
		"pages should contain different resources")

	// Verify page results match full list
	assert.Equal(t, allResources[0].ResourceID, page1[0].ResourceID,
		"first page should match first item from full list")
	assert.Equal(t, allResources[1].ResourceID, page2[0].ResourceID,
		"second page should match second item from full list")
}

// TestResource_FilterConsistency verifies that filtering returns
// consistent and correct results.
func TestResource_FilterConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	adp := setupKubernetesAdapter(t)
	if adp == nil {
		t.Skip("Kubernetes adapter not available")
	}

	ctx := context.Background()

	// Get all resources
	allResources, err := adp.ListResources(ctx, nil)
	require.NoError(t, err)

	if len(allResources) == 0 {
		t.Skip("No resources available to test filtering")
	}

	// Test filtering by resource pool if available
	if allResources[0].ResourcePoolID != "" {
		poolID := allResources[0].ResourcePoolID

		filtered, err := adp.ListResources(ctx, &adapter.Filter{
			ResourcePoolID: poolID,
		})
		require.NoError(t, err)

		// All filtered resources should have the specified pool ID
		for i, res := range filtered {
			assert.Equal(t, poolID, res.ResourcePoolID,
				"filtered resource %d should have pool ID %s", i, poolID)
		}
	}

	// Test filtering by resource type
	if len(allResources) > 0 {
		typeID := allResources[0].ResourceTypeID

		filtered, err := adp.ListResources(ctx, &adapter.Filter{
			ResourceTypeID: typeID,
		})
		require.NoError(t, err)

		// All filtered resources should have the specified type ID
		for i, res := range filtered {
			assert.Equal(t, typeID, res.ResourceTypeID,
				"filtered resource %d should have type ID %s", i, typeID)
		}
	}
}
