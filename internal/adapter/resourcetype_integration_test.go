//go:build integration

package adapter_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
)

// TestAllAdapters_ResourceTypeOperations tests that all adapters properly implement ResourceType operations.
// This ensures consistency across all backend implementations.
func TestAllAdapters_ResourceTypeOperations(t *testing.T) {
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

			testListResourceTypes(t, tt.adapter)
			testGetResourceType(t, tt.adapter)
			testResourceTypeFields(t, tt.adapter)
		})
	}
}

// testListResourceTypes verifies that an adapter can list resource types.
func testListResourceTypes(t *testing.T, adp adapter.Adapter) {
	ctx := context.Background()

	t.Run("list_all_types", func(t *testing.T) {
		types, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err, "ListResourceTypes should not return error")
		assert.NotEmpty(t, types, "adapter should return at least one resource type")

		for i, rt := range types {
			assert.NotEmpty(t, rt.ResourceTypeID, "resource type %d: ID required", i)
			assert.NotEmpty(t, rt.Name, "resource type %d: Name required", i)
			assert.NotNil(t, rt.Extensions, "resource type %d: Extensions should not be nil", i)
		}
	})

	t.Run("list_with_pagination", func(t *testing.T) {
		// Test with limit
		filter := &adapter.Filter{
			Limit:  5,
			Offset: 0,
		}
		types, err := adp.ListResourceTypes(ctx, filter)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(types), 5, "should respect limit parameter")
	})
}

// testGetResourceType verifies that an adapter can retrieve a specific resource type.
func testGetResourceType(t *testing.T, adp adapter.Adapter) {
	ctx := context.Background()

	t.Run("get_existing_type", func(t *testing.T) {
		// First, list types to get a valid ID
		types, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err)
		require.NotEmpty(t, types, "need at least one resource type to test Get")

		// Get the first type by ID
		typeID := types[0].ResourceTypeID
		retrieved, err := adp.GetResourceType(ctx, typeID)
		require.NoError(t, err, "GetResourceType should not return error for existing type")
		assert.Equal(t, typeID, retrieved.ResourceTypeID, "retrieved type ID should match requested ID")
	})

	t.Run("get_nonexistent_type", func(t *testing.T) {
		_, err := adp.GetResourceType(ctx, "nonexistent-type-12345")
		assert.Error(t, err, "GetResourceType should return error for nonexistent type")
		assert.Contains(t, err.Error(), "not found", "error should indicate type not found")
	})
}

// testResourceTypeFields verifies that resource type fields are properly populated.
func testResourceTypeFields(t *testing.T, adp adapter.Adapter) {
	ctx := context.Background()

	types, err := adp.ListResourceTypes(ctx, nil)
	require.NoError(t, err)
	require.NotEmpty(t, types)

	for i, rt := range types {
		t.Run(rt.ResourceTypeID, func(t *testing.T) {
			// Required fields
			assert.NotEmpty(t, rt.ResourceTypeID, "ResourceTypeID required")
			assert.NotEmpty(t, rt.Name, "Name required")

			// ResourceClass validation (if set)
			if rt.ResourceClass != "" {
				validClasses := []string{"compute", "storage", "network"}
				assert.Contains(t, validClasses, rt.ResourceClass,
					"type %d: ResourceClass must be one of: compute, storage, network", i)
			}

			// ResourceKind validation (if set)
			if rt.ResourceKind != "" {
				validKinds := []string{"physical", "virtual"}
				assert.Contains(t, validKinds, rt.ResourceKind,
					"type %d: ResourceKind must be one of: physical, virtual", i)
			}

			// Extensions should not be nil
			assert.NotNil(t, rt.Extensions, "type %d: Extensions should not be nil", i)
		})
	}
}

