package starlingx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
)

func TestMapHostToResource(t *testing.T) {
	host := &IHost{
		UUID:           "host-uuid-1",
		Hostname:       "compute-0",
		Personality:    "compute",
		Administrative: "unlocked",
		Operational:    "enabled",
		Availability:   "available",
		Uptime:         3600,
		Location: map[string]interface{}{
			"name": "Ottawa",
		},
		Capabilities: map[string]interface{}{
			"cpu_model": "Intel Xeon",
		},
	}

	cpus := []ICPU{
		{
			UUID:      "cpu-1",
			CPU:       0,
			Core:      0,
			Thread:    0,
			CPUModel:  "Intel(R) Xeon(R) Gold 6140",
			CPUFamily: "6",
			HostUUID:  "host-uuid-1",
		},
		{
			UUID:      "cpu-2",
			CPU:       1,
			Core:      1,
			Thread:    0,
			CPUModel:  "Intel(R) Xeon(R) Gold 6140",
			CPUFamily: "6",
			HostUUID:  "host-uuid-1",
		},
	}

	memories := []IMemory{
		{
			UUID:                "mem-1",
			MemTotalMiB:         131072,
			MemAvailMiB:         126976,
			PlatformReservedMiB: 4096,
			Node:                0,
			HostUUID:            "host-uuid-1",
		},
	}

	disks := []IDisk{
		{
			UUID:       "disk-1",
			DevicePath: "/dev/sda",
			DeviceNode: "/dev/sda",
			SizeMiB:    476940,
			HostUUID:   "host-uuid-1",
		},
	}

	resource := mapHostToResource(host, cpus, memories, disks)

	assert.NotNil(t, resource)
	assert.Equal(t, "host-uuid-1", resource.ResourceID)
	assert.Equal(t, "starlingx-compute", resource.ResourceTypeID)
	assert.Contains(t, resource.Description, "compute")
	assert.Contains(t, resource.Description, "compute-0")

	// Check extensions
	assert.Equal(t, "compute-0", resource.Extensions["hostname"])
	assert.Equal(t, "compute", resource.Extensions["personality"])
	assert.Equal(t, "unlocked", resource.Extensions["administrative"])
	assert.Equal(t, "enabled", resource.Extensions["operational"])
	assert.Equal(t, "available", resource.Extensions["availability"])
	assert.Equal(t, 3600, resource.Extensions["uptime"])

	// Check CPU info
	assert.Equal(t, 2, resource.Extensions["cpu_count"])
	assert.Equal(t, "Intel(R) Xeon(R) Gold 6140", resource.Extensions["cpu_model"])

	// Check memory info
	assert.Equal(t, 131072, resource.Extensions["memory_total_mib"])
	assert.Equal(t, 126976, resource.Extensions["memory_available_mib"])

	// Check disk info
	assert.Equal(t, 476940, resource.Extensions["storage_total_mib"])

	// Check location and capabilities
	assert.NotNil(t, resource.Extensions["location"])
	assert.NotNil(t, resource.Extensions["capabilities"])
}

func TestGenerateResourceTypeID(t *testing.T) {
	tests := []struct {
		name     string
		host     *IHost
		expected string
	}{
		{
			name: "compute host",
			host: &IHost{
				Personality: "compute",
			},
			expected: "starlingx-compute",
		},
		{
			name: "controller host",
			host: &IHost{
				Personality: "controller",
			},
			expected: "starlingx-controller",
		},
		{
			name: "storage host",
			host: &IHost{
				Personality: "storage",
			},
			expected: "starlingx-storage",
		},
		{
			name: "compute with subfunctions",
			host: &IHost{
				Personality:  "compute",
				SubFunctions: "lowlatency",
			},
			expected: "starlingx-compute-lowlatency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateResourceTypeID(tt.host)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapSystemToDeploymentManager(t *testing.T) {
	system := &ISystem{
		UUID:            "system-uuid-1",
		Name:            "starlingx-system",
		SystemType:      "All-in-one",
		SystemMode:      "simplex",
		Description:     "Test StarlingX System",
		Location:        "Ottawa",
		Latitude:        "45.4215",
		Longitude:       "-75.6972",
		Timezone:        "UTC",
		SoftwareVersion: "8.0",
		Capabilities: map[string]interface{}{
			"feature1": "enabled",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	dm := mapSystemToDeploymentManager(
		system,
		"test-dm-1",
		"test-ocloud-1",
		"http://localhost:8080/o2ims",
	)

	require.NotNil(t, dm)
	assert.Equal(t, "test-dm-1", dm.DeploymentManagerID)
	assert.Equal(t, "starlingx-system", dm.Name)
	assert.Equal(t, "Test StarlingX System", dm.Description)
	assert.Equal(t, "test-ocloud-1", dm.OCloudID)
	assert.Equal(t, "http://localhost:8080/o2ims", dm.ServiceURI)
	assert.Contains(t, dm.SupportedLocations, "Ottawa")
	assert.Contains(t, dm.Capabilities, "compute-provisioning")
	assert.Contains(t, dm.Capabilities, "label-based-pooling")

	// Check extensions
	assert.Equal(t, "All-in-one", dm.Extensions["system_type"])
	assert.Equal(t, "simplex", dm.Extensions["system_mode"])
	assert.Equal(t, "8.0", dm.Extensions["software_version"])
	assert.Equal(t, "UTC", dm.Extensions["timezone"])
	assert.Equal(t, "Ottawa", dm.Extensions["location_description"])

	coords, ok := dm.Extensions["coordinates"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "45.4215", coords["latitude"])
	assert.Equal(t, "-75.6972", coords["longitude"])
}

func TestExtractPoolNameFromLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   []Label
		expected string
	}{
		{
			name: "pool label",
			labels: []Label{
				{LabelKey: "pool", LabelValue: "high-memory"},
			},
			expected: "high-memory",
		},
		{
			name: "resource-pool label",
			labels: []Label{
				{LabelKey: "resource-pool", LabelValue: "compute-pool"},
			},
			expected: "compute-pool",
		},
		{
			name:     "no pool label",
			labels:   []Label{},
			expected: "",
		},
		{
			name: "mixed labels",
			labels: []Label{
				{LabelKey: "zone", LabelValue: "az1"},
				{LabelKey: "pool", LabelValue: "default"},
				{LabelKey: "env", LabelValue: "prod"},
			},
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPoolNameFromLabels(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGroupHostsByPool(t *testing.T) {
	hosts := []IHost{
		{UUID: "host-1", Hostname: "compute-0"},
		{UUID: "host-2", Hostname: "compute-1"},
		{UUID: "host-3", Hostname: "compute-2"},
	}

	labels := []Label{
		{HostUUID: "host-1", LabelKey: "pool", LabelValue: "pool-a"},
		{HostUUID: "host-2", LabelKey: "pool", LabelValue: "pool-a"},
		{HostUUID: "host-3", LabelKey: "pool", LabelValue: "pool-b"},
	}

	poolGroups := groupHostsByPool(hosts, labels)

	assert.Len(t, poolGroups, 2)
	assert.Len(t, poolGroups["pool-a"], 2)
	assert.Len(t, poolGroups["pool-b"], 1)
}

func TestMapLabelsToResourcePool(t *testing.T) {
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
		},
	}

	pool := mapLabelsToResourcePool("test-pool", hosts, "test-ocloud")

	require.NotNil(t, pool)
	assert.Equal(t, "starlingx-pool-test-pool", pool.ResourcePoolID)
	assert.Equal(t, "test-pool", pool.Name)
	assert.Equal(t, "Ottawa", pool.Location)
	assert.Equal(t, "test-ocloud", pool.OCloudID)
	assert.Contains(t, pool.Description, "test-pool")
	assert.Contains(t, pool.Description, "2 hosts")

	// Check extensions
	assert.Equal(t, 2, pool.Extensions["host_count"])

	personalities, ok := pool.Extensions["personalities"].(map[string]int)
	require.True(t, ok)
	assert.Equal(t, 2, personalities["compute"])
}

func TestGenerateResourceTypesFromHosts(t *testing.T) {
	hosts := []IHost{
		{
			UUID:         "host-1",
			Personality:  "compute",
			SubFunctions: "",
		},
		{
			UUID:         "host-2",
			Personality:  "compute",
			SubFunctions: "",
		},
		{
			UUID:         "host-3",
			Personality:  "controller",
			SubFunctions: "",
		},
		{
			UUID:         "host-4",
			Personality:  "storage",
			SubFunctions: "",
		},
	}

	types := generateResourceTypesFromHosts(hosts)

	require.Len(t, types, 3) // compute, controller, storage

	typeMap := make(map[string]*adapter.ResourceType)
	for _, rt := range types {
		typeMap[rt.ResourceTypeID] = rt
	}

	// Check compute type
	computeType, ok := typeMap["starlingx-compute"]
	require.True(t, ok)
	assert.Equal(t, "StarlingX compute", computeType.Name)
	assert.Equal(t, "compute", computeType.ResourceClass)
	assert.Equal(t, "physical", computeType.ResourceKind)
	assert.Equal(t, "Wind River", computeType.Vendor)

	// Check controller type
	controllerType, ok := typeMap["starlingx-controller"]
	require.True(t, ok)
	assert.Equal(t, "StarlingX controller", controllerType.Name)
	assert.Equal(t, "control-plane", controllerType.ResourceClass)

	// Check storage type
	storageType, ok := typeMap["starlingx-storage"]
	require.True(t, ok)
	assert.Equal(t, "StarlingX storage", storageType.Name)
	assert.Equal(t, "storage", storageType.ResourceClass)
}
