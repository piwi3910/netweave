package starlingx_test

import (
	"context"
	"testing"
	"github.com/piwi3910/netweave/internal/adapters/starlingx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/piwi3910/netweave/internal/adapter"
)

func TestListResources(t *testing.T) {
	hosts := []starlingx.IHost{
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
		},
	}

	labels := []starlingx.Label{
		{UUID: "label-1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "pool-a"},
	}

	cpus := map[string][]starlingx.ICPU{
		"host-1": {
			{UUID: "cpu-1", CPU: 0, HostUUID: "host-1"},
		},
	}

	memory := map[string][]starlingx.IMemory{
		"host-1": {
			{UUID: "mem-1", MemTotalMiB: 131072, HostUUID: "host-1"},
		},
	}

	disks := map[string][]starlingx.IDisk{
		"host-1": {
			{UUID: "disk-1", SizeMiB: 476940, HostUUID: "host-1"},
		},
	}

	adp, cleanup := starlingx.CreateTestAdapter(t, &starlingx.MockServerConfig{
		Hosts:  hosts,
		Labels: labels,
		CPUs:   cpus,
		Memory: memory,
		Disks:  disks,
	})
	defer cleanup()

	ctx := context.Background()

	t.Run("list all resources", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, resources, 2)

		// Verify first resource has extensions
		assert.Equal(t, "host-1", resources[0].ResourceID)
		assert.Equal(t, "compute-0", resources[0].Extensions["hostname"])
	})

	t.Run("filter by location", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, &adapter.Filter{
			Location: "Ottawa",
		})
		require.NoError(t, err)
		assert.Len(t, resources, 1)
		assert.Equal(t, "host-1", resources[0].ResourceID)
	})

	t.Run("filter by pool", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, &adapter.Filter{
			ResourcePoolID: "starlingx-pool-pool-a",
		})
		require.NoError(t, err)
		assert.Len(t, resources, 1)
		assert.Equal(t, "host-1", resources[0].ResourceID)
	})

	t.Run("pagination with offset and limit", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, &adapter.Filter{
			Offset: 1,
			Limit:  1,
		})
		require.NoError(t, err)
		assert.Len(t, resources, 1)
		assert.Equal(t, "host-2", resources[0].ResourceID)
	})
}

func TestGetResource(t *testing.T) {
	hosts := []starlingx.IHost{
		{
			UUID:        "host-1",
			Hostname:    "compute-0",
			Personality: "compute",
		},
	}

	adp, cleanup := starlingx.CreateTestAdapter(t, &starlingx.MockServerConfig{
		Hosts: hosts,
		CPUs: map[string][]starlingx.ICPU{
			"host-1": {},
		},
		Memory: map[string][]starlingx.IMemory{
			"host-1": {},
		},
		Disks: map[string][]starlingx.IDisk{
			"host-1": {},
		},
	})
	defer cleanup()

	ctx := context.Background()

	t.Run("existing resource", func(t *testing.T) {
		resource, err := adp.GetResource(ctx, "host-1")
		require.NoError(t, err)
		assert.Equal(t, "host-1", resource.ResourceID)
		assert.Equal(t, "compute-0", resource.Extensions["hostname"])
	})

	t.Run("non-existing resource", func(t *testing.T) {
		_, err := adp.GetResource(ctx, "non-existing")
		require.Error(t, err)
		require.ErrorIs(t, err, adapter.ErrResourceNotFound)
	})
}
