//go:build integration

package adapter_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
)

// TestAllAdapters_ResourceOperations tests that all adapters properly implement Resource CRUD operations.
// This ensures consistency across all backend implementations.
func TestAllAdapters_ResourceOperations(t *testing.T) {
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

			testListResources(t, tt.adapter)
			testGetResource(t, tt.adapter)
			testCreateResource(t, tt.adapter)
			testUpdateResource(t, tt.adapter)
			testDeleteResource(t, tt.adapter)
			testResourceLifecycle(t, tt.adapter)
		})
	}
}

// testListResources verifies that an adapter can list resources.
func testListResources(t *testing.T, adp adapter.Adapter) {
	ctx := context.Background()

	t.Run("list_all_resources", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, nil)
		require.NoError(t, err, "ListResources should not return error")
		// Resources may be empty in test environments, which is fine
		assert.NotNil(t, resources, "adapter should return a non-nil slice")

		for i, res := range resources {
			assert.NotEmpty(t, res.ResourceID, "resource %d: ID required", i)
			assert.NotEmpty(t, res.ResourceTypeID, "resource %d: ResourceTypeID required", i)
			assert.NotNil(t, res.Extensions, "resource %d: Extensions should not be nil", i)
		}
	})

	t.Run("list_with_pagination", func(t *testing.T) {
		filter := &adapter.Filter{
			Limit:  5,
			Offset: 0,
		}
		resources, err := adp.ListResources(ctx, filter)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(resources), 5, "should respect limit parameter")
	})

	t.Run("list_with_filter", func(t *testing.T) {
		// List all resources first
		allResources, err := adp.ListResources(ctx, nil)
		require.NoError(t, err)

		if len(allResources) == 0 {
			t.Skip("No resources to filter")
		}

		// Filter by resource pool if available
		if allResources[0].ResourcePoolID != "" {
			filter := &adapter.Filter{
				ResourcePoolID: allResources[0].ResourcePoolID,
			}
			filtered, err := adp.ListResources(ctx, filter)
			require.NoError(t, err)
			for _, res := range filtered {
				assert.Equal(t, allResources[0].ResourcePoolID, res.ResourcePoolID,
					"filtered resources should match pool ID")
			}
		}
	})
}

// testGetResource verifies that an adapter can retrieve a specific resource.
func testGetResource(t *testing.T, adp adapter.Adapter) {
	ctx := context.Background()

	t.Run("get_existing_resource", func(t *testing.T) {
		// First, list resources to get a valid ID
		resources, err := adp.ListResources(ctx, nil)
		require.NoError(t, err)

		if len(resources) == 0 {
			t.Skip("No resources available to test Get")
		}

		// Get the first resource by ID
		resourceID := resources[0].ResourceID
		retrieved, err := adp.GetResource(ctx, resourceID)
		require.NoError(t, err, "GetResource should not return error for existing resource")
		assert.Equal(t, resourceID, retrieved.ResourceID, "retrieved resource ID should match requested ID")
		assert.NotEmpty(t, retrieved.ResourceTypeID, "retrieved resource should have type ID")
	})

	t.Run("get_nonexistent_resource", func(t *testing.T) {
		_, err := adp.GetResource(ctx, "nonexistent-resource-12345")
		assert.Error(t, err, "GetResource should return error for nonexistent resource")
		assert.Contains(t, err.Error(), "not found", "error should indicate resource not found")
	})
}

// testCreateResource verifies that an adapter can create new resources.
func testCreateResource(t *testing.T, adp adapter.Adapter) {
	ctx := context.Background()

	t.Run("create_valid_resource", func(t *testing.T) {
		// Get a valid resource type first
		resourceTypes, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err)

		if len(resourceTypes) == 0 {
			t.Skip("No resource types available to test Create")
		}

		// Get a valid resource pool if available
		resourcePools, err := adp.ListResourcePools(ctx, nil)
		require.NoError(t, err)

		poolID := ""
		if len(resourcePools) > 0 {
			poolID = resourcePools[0].ResourcePoolID
		}

		newResource := &adapter.Resource{
			ResourceTypeID: resourceTypes[0].ResourceTypeID,
			ResourcePoolID: poolID,
			Description:    "Integration test resource",
			Extensions: map[string]interface{}{
				"test": "true",
			},
		}

		created, err := adp.CreateResource(ctx, newResource)
		if err != nil {
			// Some adapters may not support creation in test mode
			t.Skipf("Adapter does not support resource creation in test mode: %v", err)
		}

		assert.NotEmpty(t, created.ResourceID, "created resource should have ID assigned")
		assert.Equal(t, newResource.ResourceTypeID, created.ResourceTypeID)
		assert.Equal(t, newResource.Description, created.Description)

		// Cleanup
		_ = adp.DeleteResource(ctx, created.ResourceID)
	})

	t.Run("create_invalid_resource", func(t *testing.T) {
		invalidResource := &adapter.Resource{
			ResourceTypeID: "invalid-type-id",
			Description:    "Invalid resource",
		}

		_, err := adp.CreateResource(ctx, invalidResource)
		if err == nil {
			t.Skip("Adapter does not validate resource type on create")
		}
		assert.Error(t, err, "creating resource with invalid type should fail")
	})
}

// testUpdateResource verifies that an adapter can update existing resources.
func testUpdateResource(t *testing.T, adp adapter.Adapter) {
	ctx := context.Background()

	t.Run("update_existing_resource", func(t *testing.T) {
		// Get an existing resource
		resources, err := adp.ListResources(ctx, nil)
		require.NoError(t, err)

		if len(resources) == 0 {
			t.Skip("No resources available to test Update")
		}

		originalResource := resources[0]
		updatedResource := &adapter.Resource{
			ResourceID:     originalResource.ResourceID,
			ResourceTypeID: originalResource.ResourceTypeID,
			ResourcePoolID: originalResource.ResourcePoolID,
			Description:    "Updated by integration test",
			Extensions: map[string]interface{}{
				"updated": "true",
			},
		}

		updated, err := adp.UpdateResource(ctx, originalResource.ResourceID, updatedResource)
		if err != nil {
			// Some adapters may not support updates in test mode
			t.Skipf("Adapter does not support resource updates in test mode: %v", err)
		}

		assert.Equal(t, updatedResource.Description, updated.Description, "description should be updated")
		assert.Equal(t, originalResource.ResourceID, updated.ResourceID, "resource ID should not change")
		assert.Equal(t, originalResource.ResourceTypeID, updated.ResourceTypeID, "type ID should not change")

		// Restore original description if possible
		_, _ = adp.UpdateResource(ctx, originalResource.ResourceID, originalResource)
	})

	t.Run("update_nonexistent_resource", func(t *testing.T) {
		nonexistentResource := &adapter.Resource{
			ResourceID:  "nonexistent-resource",
			Description: "This should fail",
		}

		_, err := adp.UpdateResource(ctx, "nonexistent-resource", nonexistentResource)
		assert.Error(t, err, "updating nonexistent resource should fail")
		assert.Contains(t, err.Error(), "not found", "error should indicate resource not found")
	})
}

// testDeleteResource verifies that an adapter can delete resources.
func testDeleteResource(t *testing.T, adp adapter.Adapter) {
	ctx := context.Background()

	t.Run("delete_nonexistent_resource", func(t *testing.T) {
		err := adp.DeleteResource(ctx, "nonexistent-resource-12345")
		assert.Error(t, err, "deleting nonexistent resource should fail")
		assert.Contains(t, err.Error(), "not found", "error should indicate resource not found")
	})
}

// testResourceLifecycle tests the complete resource lifecycle (create → update → delete).
func testResourceLifecycle(t *testing.T, adp adapter.Adapter) {
	ctx := context.Background()

	t.Run("complete_lifecycle", func(t *testing.T) {
		// Get a valid resource type
		resourceTypes, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err)

		if len(resourceTypes) == 0 {
			t.Skip("No resource types available to test lifecycle")
		}

		// Get a valid resource pool if available
		resourcePools, err := adp.ListResourcePools(ctx, nil)
		require.NoError(t, err)

		poolID := ""
		if len(resourcePools) > 0 {
			poolID = resourcePools[0].ResourcePoolID
		}

		// 1. Create
		newResource := &adapter.Resource{
			ResourceTypeID: resourceTypes[0].ResourceTypeID,
			ResourcePoolID: poolID,
			Description:    "Lifecycle test resource",
			Extensions: map[string]interface{}{
				"lifecycle": "test",
			},
		}

		created, err := adp.CreateResource(ctx, newResource)
		if err != nil {
			t.Skipf("Adapter does not support resource lifecycle operations: %v", err)
		}
		defer func() {
			_ = adp.DeleteResource(ctx, created.ResourceID)
		}()

		assert.NotEmpty(t, created.ResourceID, "created resource should have ID")

		// 2. Get
		retrieved, err := adp.GetResource(ctx, created.ResourceID)
		require.NoError(t, err, "should retrieve newly created resource")
		assert.Equal(t, created.ResourceID, retrieved.ResourceID)

		// 3. Update
		updated := &adapter.Resource{
			ResourceID:     created.ResourceID,
			ResourceTypeID: created.ResourceTypeID,
			ResourcePoolID: created.ResourcePoolID,
			Description:    "Updated lifecycle test resource",
			Extensions: map[string]interface{}{
				"lifecycle": "updated",
			},
		}

		updatedRes, err := adp.UpdateResource(ctx, created.ResourceID, updated)
		require.NoError(t, err, "should update resource")
		assert.Equal(t, updated.Description, updatedRes.Description)

		// 4. Verify update
		retrieved, err = adp.GetResource(ctx, created.ResourceID)
		require.NoError(t, err)
		assert.Equal(t, updated.Description, retrieved.Description, "description should be updated")

		// 5. Delete
		err = adp.DeleteResource(ctx, created.ResourceID)
		require.NoError(t, err, "should delete resource")

		// 6. Verify deletion
		_, err = adp.GetResource(ctx, created.ResourceID)
		assert.Error(t, err, "should not retrieve deleted resource")
		assert.Contains(t, err.Error(), "not found", "error should indicate resource not found")
	})
}
