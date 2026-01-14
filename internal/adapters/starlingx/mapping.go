package starlingx

import (
	"fmt"
	"strings"

	"github.com/piwi3910/netweave/internal/adapter"
)

// MapHostToResource converts a StarlingX IHost to an O2-IMS Resource.
func MapHostToResource(host *IHost, cpus []ICPU, memories []IMemory, disks []IDisk) *adapter.Resource {
	// Build extensions with hardware inventory
	extensions := make(map[string]interface{})
	extensions["hostname"] = host.Hostname
	extensions["personality"] = host.Personality
	extensions["administrative"] = host.Administrative
	extensions["operational"] = host.Operational
	extensions["availability"] = host.Availability
	extensions["uptime"] = host.Uptime

	// Add CPU information
	if len(cpus) > 0 {
		cpuInfo := make([]map[string]interface{}, 0, len(cpus))
		for _, cpu := range cpus {
			cpuInfo = append(cpuInfo, map[string]interface{}{
				"cpu":        cpu.CPU,
				"core":       cpu.Core,
				"thread":     cpu.Thread,
				"cpu_model":  cpu.CPUModel,
				"cpu_family": cpu.CPUFamily,
			})
		}
		extensions["cpus"] = cpuInfo
		extensions["cpu_count"] = len(cpus)
		if len(cpus) > 0 {
			extensions["cpu_model"] = cpus[0].CPUModel
		}
	}

	// Add memory information
	if len(memories) > 0 {
		totalMemMiB := 0
		availMemMiB := 0
		for _, mem := range memories {
			totalMemMiB += mem.MemTotalMiB
			availMemMiB += mem.MemAvailMiB
		}
		extensions["memory_total_mib"] = totalMemMiB
		extensions["memory_available_mib"] = availMemMiB
	}

	// Add disk information
	if len(disks) > 0 {
		totalDiskMiB := 0
		diskInfo := make([]map[string]interface{}, 0, len(disks))
		for _, disk := range disks {
			totalDiskMiB += disk.SizeMiB
			diskInfo = append(diskInfo, map[string]interface{}{
				"device_path": disk.DevicePath,
				"device_node": disk.DeviceNode,
				"size_mib":    disk.SizeMiB,
			})
		}
		extensions["disks"] = diskInfo
		extensions["storage_total_mib"] = totalDiskMiB
	}

	// Add location if present
	if host.Location != nil {
		extensions["location"] = host.Location
	}

	// Add capabilities if present
	if host.Capabilities != nil {
		extensions["capabilities"] = host.Capabilities
	}

	// Generate description
	description := fmt.Sprintf("StarlingX %s host: %s (state: %s/%s/%s)",
		host.Personality,
		host.Hostname,
		host.Administrative,
		host.Operational,
		host.Availability,
	)

	return &adapter.Resource{
		ResourceID:     host.UUID,
		ResourceTypeID: GenerateResourceTypeID(host),
		Description:    description,
		Extensions:     extensions,
	}
}

// GenerateResourceTypeID creates a resource type ID based on host personality and capabilities.
func GenerateResourceTypeID(host *IHost) string {
	// Base type on personality
	typeID := fmt.Sprintf("starlingx-%s", strings.ToLower(host.Personality))

	// Add subfunctions if present
	if host.SubFunctions != "" {
		typeID = fmt.Sprintf("%s-%s", typeID, strings.ToLower(host.SubFunctions))
	}

	return typeID
}

// MapSystemToDeploymentManager converts a StarlingX ISystem to an O2-IMS DeploymentManager.
func MapSystemToDeploymentManager(system *ISystem, deploymentManagerID, oCloudID, serviceURI string) *adapter.DeploymentManager {
	extensions := make(map[string]interface{})
	extensions["system_type"] = system.SystemType
	extensions["system_mode"] = system.SystemMode
	extensions["software_version"] = system.SoftwareVersion
	extensions["timezone"] = system.Timezone

	if system.Location != "" {
		extensions["location_description"] = system.Location
	}

	if system.Latitude != "" && system.Longitude != "" {
		extensions["coordinates"] = map[string]string{
			"latitude":  system.Latitude,
			"longitude": system.Longitude,
		}
	}

	if system.Capabilities != nil {
		extensions["capabilities_detail"] = system.Capabilities
	}

	capabilities := []string{
		"compute-provisioning",
		"storage-management",
		"network-configuration",
		"label-based-pooling",
	}

	supportedLocations := []string{}
	if system.Location != "" {
		supportedLocations = append(supportedLocations, system.Location)
	}

	description := system.Description
	if description == "" {
		description = fmt.Sprintf("StarlingX %s deployment: %s", system.SystemType, system.Name)
	}

	return &adapter.DeploymentManager{
		DeploymentManagerID: deploymentManagerID,
		Name:                system.Name,
		Description:         description,
		OCloudID:            oCloudID,
		ServiceURI:          serviceURI,
		SupportedLocations:  supportedLocations,
		Capabilities:        capabilities,
		Extensions:          extensions,
	}
}

// ExtractPoolNameFromLabels extracts resource pool name from host labels.
// Looks for labels with key "pool" or "resource-pool".
func ExtractPoolNameFromLabels(labels []Label) string {
	for _, label := range labels {
		if label.LabelKey == "pool" || label.LabelKey == "resource-pool" {
			return label.LabelValue
		}
	}
	return ""
}

// GroupHostsByPool groups hosts by their pool label.
func GroupHostsByPool(hosts []IHost, allLabels []Label) map[string][]IHost {
	// Build host UUID to labels mapping
	hostLabels := make(map[string][]Label)
	for _, label := range allLabels {
		hostLabels[label.HostUUID] = append(hostLabels[label.HostUUID], label)
	}

	// Group hosts by pool
	poolGroups := make(map[string][]IHost)
	defaultPool := "default"

	for _, host := range hosts {
		labels := hostLabels[host.UUID]
		poolName := ExtractPoolNameFromLabels(labels)
		if poolName == "" {
			poolName = defaultPool
		}
		poolGroups[poolName] = append(poolGroups[poolName], host)
	}

	return poolGroups
}

// MapLabelsToResourcePool creates an O2-IMS ResourcePool from grouped hosts and labels.
func MapLabelsToResourcePool(poolName string, hosts []IHost, oCloudID string) *adapter.ResourcePool {
	extensions := make(map[string]interface{})
	extensions["host_count"] = len(hosts)

	// Aggregate host personalities
	personalityCounts := make(map[string]int)
	for _, host := range hosts {
		personalityCounts[host.Personality]++
	}
	extensions["personalities"] = personalityCounts

	// Extract location from hosts if available
	location := ""
	for _, host := range hosts {
		if host.Location != nil {
			if locName, ok := host.Location["name"].(string); ok && locName != "" {
				location = locName
				break
			}
		}
	}

	description := fmt.Sprintf("StarlingX resource pool '%s' with %d hosts", poolName, len(hosts))
	if len(personalityCounts) > 0 {
		description = fmt.Sprintf("%s (%v)", description, personalityCounts)
	}

	// Generate pool ID
	poolID := fmt.Sprintf("starlingx-pool-%s", poolName)

	return &adapter.ResourcePool{
		ResourcePoolID: poolID,
		Name:           poolName,
		Description:    description,
		Location:       location,
		OCloudID:       oCloudID,
		Extensions:     extensions,
	}
}

// GenerateResourceTypesFromHosts creates O2-IMS ResourceTypes based on host personalities and capabilities.
func GenerateResourceTypesFromHosts(hosts []IHost) []*adapter.ResourceType {
	typeMap := make(map[string]*adapter.ResourceType)

	for _, host := range hosts {
		typeID := GenerateResourceTypeID(&host)

		if _, exists := typeMap[typeID]; !exists {
			// Extract CPU model if available from first host of this type
			vendor := "Wind River"
			model := host.Personality
			version := ""

			if host.Capabilities != nil {
				if cpuModel, ok := host.Capabilities["cpu_model"].(string); ok {
					model = cpuModel
				}
			}

			resourceClass := "compute"
			switch strings.ToLower(host.Personality) {
			case "storage":
				resourceClass = "storage"
			case "controller":
				resourceClass = "control-plane"
			}

			resourceKind := "physical" // StarlingX manages physical infrastructure

			description := fmt.Sprintf("StarlingX %s node type", host.Personality)
			if host.SubFunctions != "" {
				description = fmt.Sprintf("%s with subfunctions: %s", description, host.SubFunctions)
			}

			typeMap[typeID] = &adapter.ResourceType{
				ResourceTypeID: typeID,
				Name:           fmt.Sprintf("StarlingX %s", host.Personality),
				Description:    description,
				Vendor:         vendor,
				Model:          model,
				Version:        version,
				ResourceClass:  resourceClass,
				ResourceKind:   resourceKind,
				Extensions: map[string]interface{}{
					"personality":  host.Personality,
					"subfunctions": host.SubFunctions,
					"source":       "starlingx",
				},
			}
		}
	}

	// Convert map to slice
	types := make([]*adapter.ResourceType, 0, len(typeMap))
	for _, rt := range typeMap {
		types = append(types, rt)
	}

	return types
}
