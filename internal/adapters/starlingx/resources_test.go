package starlingx_test

import (
	"context"
	"testing"

	"github.com/piwi3910/netweave/internal/adapters/starlingx"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestListResources_EmptyHosts(t *testing.T) {
	adp, cleanup := starlingx.CreateTestAdapter(t, &starlingx.MockServerConfig{
		Hosts: []starlingx.IHost{},
	})
	defer cleanup()

	ctx := context.Background()
	resources, err := adp.ListResources(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, resources)
}

func TestListResources_FilterByResourceType(t *testing.T) {
	hosts := []starlingx.IHost{
		{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
		{UUID: "host-2", Hostname: "controller-0", Personality: "controller"},
		{UUID: "host-3", Hostname: "storage-0", Personality: "storage"},
	}

	adp, cleanup := starlingx.CreateTestAdapter(t, &starlingx.MockServerConfig{
		Hosts: hosts,
	})
	defer cleanup()

	ctx := context.Background()

	t.Run("filter by compute type", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, &adapter.Filter{
			ResourceTypeID: "starlingx-compute",
		})
		require.NoError(t, err)
		assert.Len(t, resources, 1)
		assert.Equal(t, "host-1", resources[0].ResourceID)
	})

	t.Run("filter by controller type", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, &adapter.Filter{
			ResourceTypeID: "starlingx-controller",
		})
		require.NoError(t, err)
		assert.Len(t, resources, 1)
		assert.Equal(t, "host-2", resources[0].ResourceID)
	})

	t.Run("filter by storage type", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, &adapter.Filter{
			ResourceTypeID: "starlingx-storage",
		})
		require.NoError(t, err)
		assert.Len(t, resources, 1)
		assert.Equal(t, "host-3", resources[0].ResourceID)
	})

	t.Run("filter by non-existing type", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, &adapter.Filter{
			ResourceTypeID: "starlingx-nonexistent",
		})
		require.NoError(t, err)
		assert.Empty(t, resources)
	})
}

func TestListResources_FilterByPool(t *testing.T) {
	hosts := []starlingx.IHost{
		{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
		{UUID: "host-2", Hostname: "compute-1", Personality: "compute"},
	}

	labels := []starlingx.Label{
		{UUID: "label-1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "pool-a"},
		{UUID: "label-2", HostUUID: "host-2", LabelKey: "pool", LabelValue: "pool-b"},
	}

	adp, cleanup := starlingx.CreateTestAdapter(t, &starlingx.MockServerConfig{
		Hosts:  hosts,
		Labels: labels,
	})
	defer cleanup()

	ctx := context.Background()

	t.Run("filter by matching pool", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, &adapter.Filter{
			ResourcePoolID: "starlingx-pool-pool-a",
		})
		require.NoError(t, err)
		assert.Len(t, resources, 1)
		assert.Equal(t, "host-1", resources[0].ResourceID)
		assert.Equal(t, "starlingx-pool-pool-a", resources[0].ResourcePoolID)
	})

	t.Run("filter by different pool", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, &adapter.Filter{
			ResourcePoolID: "starlingx-pool-pool-b",
		})
		require.NoError(t, err)
		assert.Len(t, resources, 1)
		assert.Equal(t, "host-2", resources[0].ResourceID)
	})

	t.Run("filter by non-existing pool returns default matches", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, &adapter.Filter{
			ResourcePoolID: "starlingx-pool-nonexistent",
		})
		require.NoError(t, err)
		assert.Empty(t, resources)
	})
}

func TestListResources_WithHardwareDetails(t *testing.T) {
	hosts := []starlingx.IHost{
		{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
	}

	cpus := map[string][]starlingx.ICPU{
		"host-1": {
			{UUID: "cpu-1", CPU: 0, HostUUID: "host-1"},
			{UUID: "cpu-2", CPU: 1, HostUUID: "host-1"},
			{UUID: "cpu-3", CPU: 2, HostUUID: "host-1"},
			{UUID: "cpu-4", CPU: 3, HostUUID: "host-1"},
		},
	}

	memory := map[string][]starlingx.IMemory{
		"host-1": {
			{UUID: "mem-1", MemTotalMiB: 65536, HostUUID: "host-1"},
			{UUID: "mem-2", MemTotalMiB: 65536, HostUUID: "host-1"},
		},
	}

	disks := map[string][]starlingx.IDisk{
		"host-1": {
			{UUID: "disk-1", SizeMiB: 476940, HostUUID: "host-1"},
			{UUID: "disk-2", SizeMiB: 953870, HostUUID: "host-1"},
		},
	}

	adp, cleanup := starlingx.CreateTestAdapter(t, &starlingx.MockServerConfig{
		Hosts:  hosts,
		CPUs:   cpus,
		Memory: memory,
		Disks:  disks,
	})
	defer cleanup()

	ctx := context.Background()

	resources, err := adp.ListResources(ctx, nil)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	res := resources[0]
	assert.Equal(t, "host-1", res.ResourceID)
	assert.Equal(t, "compute-0", res.Extensions["hostname"])
	assert.NotNil(t, res.Extensions)

	// Verify hardware details are present (key names from mapping.go)
	assert.NotNil(t, res.Extensions["cpus"])
	assert.Equal(t, 4, res.Extensions["cpu_count"])
	assert.NotNil(t, res.Extensions["memory_total_mib"])
	assert.Equal(t, 131072, res.Extensions["memory_total_mib"])
	assert.NotNil(t, res.Extensions["disks"])
	assert.Equal(t, 1430810, res.Extensions["storage_total_mib"])
}

func TestGetResource_WithHardwareDetails(t *testing.T) {
	hosts := []starlingx.IHost{
		{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
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
		CPUs:   cpus,
		Memory: memory,
		Disks:  disks,
	})
	defer cleanup()

	ctx := context.Background()

	resource, err := adp.GetResource(ctx, "host-1")
	require.NoError(t, err)
	require.NotNil(t, resource)
	assert.Equal(t, "host-1", resource.ResourceID)
	assert.NotNil(t, resource.Extensions["cpus"])
	assert.Equal(t, 1, resource.Extensions["cpu_count"])
	assert.NotNil(t, resource.Extensions["memory_total_mib"])
	assert.Equal(t, 131072, resource.Extensions["memory_total_mib"])
	assert.NotNil(t, resource.Extensions["disks"])
	assert.Equal(t, 476940, resource.Extensions["storage_total_mib"])
}
