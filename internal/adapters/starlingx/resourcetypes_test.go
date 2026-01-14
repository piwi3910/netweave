package starlingx_test

import (
	"context"
	"testing"
	"github.com/piwi3910/netweave/internal/adapters/starlingx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/piwi3910/netweave/internal/adapter"
)

func TestListResourceTypes(t *testing.T) {
	hosts := []starlingx.IHost{
		{UUID: "host-1", Personality: "compute"},
		{UUID: "host-2", Personality: "compute"},
		{UUID: "host-3", Personality: "controller"},
		{UUID: "host-4", Personality: "storage"},
	}

	adp, cleanup := starlingx.CreateTestAdapter(t, &starlingx.MockServerConfig{
		Hosts: hosts,
	})
	defer cleanup()

	ctx := context.Background()

	t.Run("list all types", func(t *testing.T) {
		types, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, types, 3) // compute, controller, storage

		typeMap := make(map[string]*adapter.ResourceType)
		for _, rt := range types {
			typeMap[rt.ResourceTypeID] = rt
		}

		// Verify compute type
		computeType, ok := typeMap["starlingx-compute"]
		require.True(t, ok)
		assert.Equal(t, "StarlingX compute", computeType.Name)
		assert.Equal(t, "compute", computeType.ResourceClass)

		// Verify controller type
		controllerType, ok := typeMap["starlingx-controller"]
		require.True(t, ok)
		assert.Equal(t, "StarlingX controller", controllerType.Name)

		// Verify storage type
		storageType, ok := typeMap["starlingx-storage"]
		require.True(t, ok)
		assert.Equal(t, "StarlingX storage", storageType.Name)
	})

	t.Run("pagination", func(t *testing.T) {
		types, err := adp.ListResourceTypes(ctx, &adapter.Filter{
			Offset: 1,
			Limit:  2,
		})
		require.NoError(t, err)
		assert.Len(t, types, 2)
	})
}

func TestGetResourceType(t *testing.T) {
	hosts := []starlingx.IHost{
		{UUID: "host-1", Personality: "compute"},
		{UUID: "host-2", Personality: "controller"},
	}

	adp, cleanup := starlingx.CreateTestAdapter(t, &starlingx.MockServerConfig{
		Hosts: hosts,
	})
	defer cleanup()

	ctx := context.Background()

	t.Run("existing type", func(t *testing.T) {
		rt, err := adp.GetResourceType(ctx, "starlingx-compute")
		require.NoError(t, err)
		assert.Equal(t, "starlingx-compute", rt.ResourceTypeID)
		assert.Equal(t, "StarlingX compute", rt.Name)
	})

	t.Run("non-existing type", func(t *testing.T) {
		_, err := adp.GetResourceType(ctx, "non-existing")
		require.Error(t, err)
		require.ErrorIs(t, err, adapter.ErrResourceTypeNotFound)
	})
}
