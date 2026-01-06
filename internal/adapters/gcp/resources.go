package gcp

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// ListResources retrieves all resources (GCP instances) matching the provided filter.
func (a *GCPAdapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	a.logger.Debug("ListResources called",
		zap.Any("filter", filter))

	var resources []*adapter.Resource

	// List zones in the region first
	zoneIt := a.zonesClient.List(ctx, &computepb.ListZonesRequest{
		Project: a.projectID,
	})

	for {
		zone, err := zoneIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list zones: %w", err)
		}

		zoneName := ptrToString(zone.Name)
		if !strings.HasPrefix(zoneName, a.region) {
			continue
		}

		// List instances in this zone
		instanceIt := a.instancesClient.List(ctx, &computepb.ListInstancesRequest{
			Project: a.projectID,
			Zone:    zoneName,
		})

		for {
			instance, err := instanceIt.Next()
			if err == iterator.Done {
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
	}

	// Apply pagination
	if filter != nil {
		resources = adapter.ApplyPagination(resources, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resources",
		zap.Int("count", len(resources)))

	return resources, nil
}

// GetResource retrieves a specific resource (GCP instance) by ID.
func (a *GCPAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
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
func (a *GCPAdapter) CreateResource(ctx context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	a.logger.Debug("CreateResource called",
		zap.String("resourceTypeId", resource.ResourceTypeID))

	// Creating GCP instances requires extensive configuration
	return nil, fmt.Errorf("creating GCP instances requires additional configuration: use gcloud CLI or console")
}

// DeleteResource deletes a resource (GCP instance) by ID.
func (a *GCPAdapter) DeleteResource(ctx context.Context, id string) error {
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

	// Extract machine type name from URL
	machineTypeURL := ptrToString(instance.MachineType)
	machineType := extractMachineTypeName(machineTypeURL)
	resourceTypeID := generateMachineTypeID(machineType)

	// Determine resource pool ID based on pool mode
	var resourcePoolID string
	if a.poolMode == "zone" {
		resourcePoolID = generateZonePoolID(zone)
	} else {
		// In IG mode, we would need to look up which IG this instance belongs to
		// For now, use zone as fallback
		resourcePoolID = generateZonePoolID(zone)
	}

	// Build extensions with GCP instance details
	extensions := map[string]interface{}{
		"gcp.id":          ptrToInt64((*int64)(instance.Id)),
		"gcp.name":        instanceName,
		"gcp.zone":        zone,
		"gcp.machineType": machineType,
		"gcp.status":      ptrToString(instance.Status),
		"gcp.selfLink":    ptrToString(instance.SelfLink),
		"gcp.labels":      instance.Labels,
	}

	// Add network interfaces
	if len(instance.NetworkInterfaces) > 0 {
		nics := make([]map[string]interface{}, 0, len(instance.NetworkInterfaces))
		for _, nic := range instance.NetworkInterfaces {
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
		if len(instance.NetworkInterfaces) > 0 {
			extensions["gcp.internalIP"] = ptrToString(instance.NetworkInterfaces[0].NetworkIP)
			if len(instance.NetworkInterfaces[0].AccessConfigs) > 0 {
				extensions["gcp.externalIP"] = ptrToString(instance.NetworkInterfaces[0].AccessConfigs[0].NatIP)
			}
		}
	}

	// Add disks
	if len(instance.Disks) > 0 {
		disks := make([]map[string]interface{}, 0, len(instance.Disks))
		for _, disk := range instance.Disks {
			diskInfo := map[string]interface{}{
				"deviceName": ptrToString(disk.DeviceName),
				"source":     ptrToString(disk.Source),
				"boot":       ptrToBool(disk.Boot),
				"mode":       ptrToString(disk.Mode),
				"sizeGB":     ptrToInt64(disk.DiskSizeGb),
				"type":       ptrToString(disk.Type),
			}
			disks = append(disks, diskInfo)
		}
		extensions["gcp.disks"] = disks
	}

	// Add CPU platform
	extensions["gcp.cpuPlatform"] = ptrToString(instance.CpuPlatform)

	// Add creation timestamp
	extensions["gcp.creationTimestamp"] = ptrToString(instance.CreationTimestamp)

	// Add scheduling info
	if instance.Scheduling != nil {
		extensions["gcp.preemptible"] = ptrToBool(instance.Scheduling.Preemptible)
		extensions["gcp.automaticRestart"] = ptrToBool(instance.Scheduling.AutomaticRestart)
	}

	// Get description for the resource
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

// extractMachineTypeName extracts the machine type name from a GCP machine type URL.
// e.g., "zones/us-central1-a/machineTypes/n1-standard-1" -> "n1-standard-1"
func extractMachineTypeName(machineTypeURL string) string {
	parts := strings.Split(machineTypeURL, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return machineTypeURL
}
