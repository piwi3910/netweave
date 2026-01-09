package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// ListResources retrieves all resources (GCP instances) matching the provided filter.
func (a *GCPAdapter) ListResources(ctx context.Context, filter *adapter.Filter) (resources []*adapter.Resource, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("gcp", "ListResources", start, err) }()

	a.logger.Debug("ListResources called",
		zap.Any("filter", filter))

	// List instances across all zones in the region
	resources, err = a.listInstancesInRegion(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Apply pagination
	if filter != nil {
		resources = adapter.ApplyPagination(resources, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resources",
		zap.Int("count", len(resources)))

	return resources, nil
}

// listInstancesInRegion lists all instances across zones in the region.
func (a *GCPAdapter) listInstancesInRegion(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	var resources []*adapter.Resource

	zoneIt := a.zonesClient.List(ctx, &computepb.ListZonesRequest{
		Project: a.projectID,
	})

	for {
		zone, err := zoneIt.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list zones: %w", err)
		}

		zoneName := ptrToString(zone.Name)
		if !strings.HasPrefix(zoneName, a.region) {
			continue
		}

		zoneResources, err := a.listInstancesInZone(ctx, zoneName, filter)
		if err != nil {
			return nil, err
		}

		resources = append(resources, zoneResources...)
	}

	return resources, nil
}

// listInstancesInZone lists instances in a specific zone and applies filtering.
func (a *GCPAdapter) listInstancesInZone(ctx context.Context, zoneName string, filter *adapter.Filter) ([]*adapter.Resource, error) {
	var resources []*adapter.Resource

	instanceIt := a.instancesClient.List(ctx, &computepb.ListInstancesRequest{
		Project: a.projectID,
		Zone:    zoneName,
	})

	for {
		instance, err := instanceIt.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list instances: %w", err)
		}

		resource := a.instanceToResource(instance, zoneName)

		// Apply filter
		labels := instance.Labels
		if labels == nil {
			labels = make(map[string]string)
		}
		if !adapter.MatchesFilter(filter, resource.ResourcePoolID, resource.ResourceTypeID, zoneName, labels) {
			continue
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// GetResource retrieves a specific resource (GCP instance) by ID.
func (a *GCPAdapter) GetResource(ctx context.Context, id string) (resource *adapter.Resource, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("gcp", "GetResource", start, err) }()

	a.logger.Debug("GetResource called",
		zap.String("id", id))

	// Parse zone and instance name from the ID
	// Format: gcp-instance-{zone}-{name}
	prefix := "gcp-instance-"
	if !strings.HasPrefix(id, prefix) {
		return nil, fmt.Errorf("invalid resource ID format: %s", id)
	}

	remainder := strings.TrimPrefix(id, prefix)
	parts := strings.SplitN(remainder, "-", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid resource ID format: %s", id)
	}

	// Zone is in format like "us-central1-a", so we need to handle this carefully
	// The zone name contains dashes, so we need a different approach
	// We'll list all instances and find the one with matching ID
	resources, err := a.ListResources(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, resource := range resources {
		if resource.ResourceID == id {
			return resource, nil
		}
	}

	return nil, fmt.Errorf("resource not found: %s", id)
}

// CreateResource creates a new resource (GCP instance).
func (a *GCPAdapter) CreateResource(_ context.Context, resource *adapter.Resource) (result *adapter.Resource, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("gcp", "CreateResource", start, err) }()

	a.logger.Debug("CreateResource called",
		zap.String("resourceTypeId", resource.ResourceTypeID))

	// Creating GCP instances requires extensive configuration
	return nil, fmt.Errorf("creating GCP instances requires additional configuration: use gcloud CLI or console")
}

// UpdateResource updates an existing GCP instance's labels and metadata.
// Note: Core instance properties cannot be modified after creation.
func (a *GCPAdapter) UpdateResource(_ context.Context, _ string, resource *adapter.Resource) (updated *adapter.Resource, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("gcp", "UpdateResource", start, err) }()

	a.logger.Debug("UpdateResource called",
		zap.String("resourceID", resource.ResourceID))

	// TODO(#190): Implement instance metadata updates via GCP API
	// For now, return not supported
	return nil, fmt.Errorf("updating GCP instances is not yet implemented")
}

// DeleteResource deletes a resource (GCP instance) by ID.
func (a *GCPAdapter) DeleteResource(ctx context.Context, id string) (err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("gcp", "DeleteResource", start, err) }()

	a.logger.Debug("DeleteResource called",
		zap.String("id", id))

	// Find the instance to get zone and name
	resource, err := a.GetResource(ctx, id)
	if err != nil {
		return err
	}

	// Extract zone and instance name from extensions
	zone, ok := resource.Extensions["gcp.zone"].(string)
	if !ok {
		return fmt.Errorf("cannot determine zone for resource: %s", id)
	}
	instanceName, ok := resource.Extensions["gcp.name"].(string)
	if !ok {
		return fmt.Errorf("cannot determine instance name for resource: %s", id)
	}

	// Delete the instance
	op, err := a.instancesClient.Delete(ctx, &computepb.DeleteInstanceRequest{
		Project:  a.projectID,
		Zone:     zone,
		Instance: instanceName,
	})
	if err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}

	// Wait for operation to complete
	err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for instance deletion: %w", err)
	}

	a.logger.Info("deleted resource",
		zap.String("resourceId", id))

	return nil
}

// instanceToResource converts a GCP instance to an O2-IMS Resource.
func (a *GCPAdapter) instanceToResource(instance *computepb.Instance, zone string) *adapter.Resource {
	instanceName := ptrToString(instance.Name)
	resourceID := generateInstanceID(instanceName, zone)

	// Extract machine type and resource pool
	machineType := extractMachineTypeName(ptrToString(instance.MachineType))
	resourceTypeID := generateMachineTypeID(machineType)
	resourcePoolID := a.determineResourcePoolID(zone)

	// Build extensions with instance details
	extensions := buildInstanceExtensions(instance, instanceName, zone, machineType)

	// Get description
	description := ptrToString(instance.Description)
	if description == "" {
		description = instanceName
	}

	return &adapter.Resource{
		ResourceID:     resourceID,
		ResourceTypeID: resourceTypeID,
		ResourcePoolID: resourcePoolID,
		GlobalAssetID:  fmt.Sprintf("urn:gcp:compute:%s:%s:%s", a.projectID, zone, instanceName),
		Description:    description,
		Extensions:     extensions,
	}
}

// determineResourcePoolID determines the resource pool ID based on pool mode.
func (a *GCPAdapter) determineResourcePoolID(zone string) string {
	if a.poolMode == "zone" {
		return generateZonePoolID(zone)
	}
	// In IG mode, we would need to look up which IG this instance belongs to
	// For now, use zone as fallback
	return generateZonePoolID(zone)
}

// buildInstanceExtensions builds the extensions map with GCP instance details.
func buildInstanceExtensions(instance *computepb.Instance, instanceName, zone, machineType string) map[string]interface{} {
	var instanceID uint64
	if instance.Id != nil {
		instanceID = *instance.Id
	}

	extensions := map[string]interface{}{
		"gcp.id":          instanceID,
		"gcp.name":        instanceName,
		"gcp.zone":        zone,
		"gcp.machineType": machineType,
		"gcp.status":      ptrToString(instance.Status),
		"gcp.selfLink":    ptrToString(instance.SelfLink),
		"gcp.labels":      instance.Labels,
	}

	addInstanceNetworkInterfaces(extensions, instance.NetworkInterfaces)
	addInstanceDisks(extensions, instance.Disks)
	addInstanceSchedulingInfo(extensions, instance.Scheduling)

	extensions["gcp.cpuPlatform"] = ptrToString(instance.CpuPlatform)
	extensions["gcp.creationTimestamp"] = ptrToString(instance.CreationTimestamp)

	return extensions
}

// addInstanceNetworkInterfaces adds network interface information to extensions.
func addInstanceNetworkInterfaces(extensions map[string]interface{}, networkInterfaces []*computepb.NetworkInterface) {
	if len(networkInterfaces) == 0 {
		return
	}

	nics := make([]map[string]interface{}, 0, len(networkInterfaces))
	for _, nic := range networkInterfaces {
		nicInfo := map[string]interface{}{
			"name":       ptrToString(nic.Name),
			"network":    ptrToString(nic.Network),
			"subnetwork": ptrToString(nic.Subnetwork),
			"internalIP": ptrToString(nic.NetworkIP),
		}
		if len(nic.AccessConfigs) > 0 {
			nicInfo["externalIP"] = ptrToString(nic.AccessConfigs[0].NatIP)
		}
		nics = append(nics, nicInfo)
	}
	extensions["gcp.networkInterfaces"] = nics

	// Add primary IPs for quick access
	extensions["gcp.internalIP"] = ptrToString(networkInterfaces[0].NetworkIP)
	if len(networkInterfaces[0].AccessConfigs) > 0 {
		extensions["gcp.externalIP"] = ptrToString(networkInterfaces[0].AccessConfigs[0].NatIP)
	}
}

// addInstanceDisks adds disk information to extensions.
func addInstanceDisks(extensions map[string]interface{}, disks []*computepb.AttachedDisk) {
	if len(disks) == 0 {
		return
	}

	diskList := make([]map[string]interface{}, 0, len(disks))
	for _, disk := range disks {
		diskInfo := map[string]interface{}{
			"deviceName": ptrToString(disk.DeviceName),
			"source":     ptrToString(disk.Source),
			"boot":       ptrToBool(disk.Boot),
			"mode":       ptrToString(disk.Mode),
			"sizeGB":     ptrToInt64(disk.DiskSizeGb),
			"type":       ptrToString(disk.Type),
		}
		diskList = append(diskList, diskInfo)
	}
	extensions["gcp.disks"] = diskList
}

// addInstanceSchedulingInfo adds scheduling information to extensions.
func addInstanceSchedulingInfo(extensions map[string]interface{}, scheduling *computepb.Scheduling) {
	if scheduling != nil {
		extensions["gcp.preemptible"] = ptrToBool(scheduling.Preemptible)
		extensions["gcp.automaticRestart"] = ptrToBool(scheduling.AutomaticRestart)
	}
}

// extractMachineTypeName extracts the machine type name from a GCP machine type URL.
// e.g., "zones/us-central1-a/machineTypes/n1-standard-1" -> "n1-standard-1".
func extractMachineTypeName(machineTypeURL string) string {
	parts := strings.Split(machineTypeURL, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return machineTypeURL
}
