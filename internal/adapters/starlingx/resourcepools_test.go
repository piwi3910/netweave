package starlingx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
)

func TestListResourcePools(t *testing.T) {
	hosts := []IHost{
		{
			UUID:        "host-1",
			Hostname:    "compute-0",
			Personality: "compute",
			Location: map[string]interface{}{
				"name": "Ottawa",
			},
		},
		{
			UUID:        "host-2",
			Hostname:    "compute-1",
			Personality: "compute",
			Location: map[string]interface{}{
				"name": "Ottawa",
			},
		},
		{
			UUID:        "host-3",
			Hostname:    "compute-2",
			Personality: "compute",
			Location: map[string]interface{}{
				"name": "Toronto",
			},
		},
	}

	labels := []Label{
		{UUID: "label-1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "pool-a"},
		{UUID: "label-2", HostUUID: "host-2", LabelKey: "pool", LabelValue: "pool-a"},
		{UUID: "label-3", HostUUID: "host-3", LabelKey: "pool", LabelValue: "pool-b"},
	}

	adp, cleanup := createTestAdapter(t, &mockServerConfig{
		Hosts:  hosts,
		Labels: labels,
	})
	defer cleanup()

	ctx := context.Background()

	t.Run("list all pools", func(t *testing.T) {
		pools, err := adp.ListResourcePools(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, pools, 2) // pool-a and pool-b

		poolMap := make(map[string]*adapter.ResourcePool)
		for _, pool := range pools {
			poolMap[pool.Name] = pool
		}

		// Verify pool-a
		poolA, ok := poolMap["pool-a"]
		require.True(t, ok)
		assert.Equal(t, "starlingx-pool-pool-a", poolA.ResourcePoolID)
		assert.Equal(t, 2, poolA.Extensions["host_count"])

		// Verify pool-b
		poolB, ok := poolMap["pool-b"]
		require.True(t, ok)
		assert.Equal(t, "starlingx-pool-pool-b", poolB.ResourcePoolID)
		assert.Equal(t, 1, poolB.Extensions["host_count"])
	})

	t.Run("filter by location", func(t *testing.T) {
		pools, err := adp.ListResourcePools(ctx, &adapter.Filter{
			Location: "Ottawa",
		})
		require.NoError(t, err)
		assert.Len(t, pools, 1) // Only pool-a has Ottawa hosts
		assert.Equal(t, "pool-a", pools[0].Name)
	})

	t.Run("pagination", func(t *testing.T) {
		pools, err := adp.ListResourcePools(ctx, &adapter.Filter{
			Offset: 1,
			Limit:  1,
		})
		require.NoError(t, err)
		assert.Len(t, pools, 1)
	})
}

func TestGetResourcePool(t *testing.T) {
	hosts := []IHost{
		{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
		{UUID: "host-2", Hostname: "compute-1", Personality: "compute"},
	}

	labels := []Label{
		{UUID: "label-1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "test-pool"},
		{UUID: "label-2", HostUUID: "host-2", LabelKey: "pool", LabelValue: "test-pool"},
	}

	adp, cleanup := createTestAdapter(t, &mockServerConfig{
		Hosts:  hosts,
		Labels: labels,
	})
	defer cleanup()

	ctx := context.Background()

	t.Run("existing pool", func(t *testing.T) {
		pool, err := adp.GetResourcePool(ctx, "starlingx-pool-test-pool")
		require.NoError(t, err)
		assert.Equal(t, "starlingx-pool-test-pool", pool.ResourcePoolID)
		assert.Equal(t, "test-pool", pool.Name)
		assert.Equal(t, 2, pool.Extensions["host_count"])
	})

	t.Run("non-existing pool", func(t *testing.T) {
		_, err := adp.GetResourcePool(ctx, "starlingx-pool-non-existing")
		require.Error(t, err)
		require.ErrorIs(t, err, adapter.ErrResourcePoolNotFound)
	})
}
