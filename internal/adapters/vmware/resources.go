package vmware

import (
	"context"
	"fmt"
	"strings"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"go.uber.org/zap"
)

// ListResources retrieves all resources (VMs) matching the provided filter.
func (a *VMwareAdapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	a.logger.Debug("ListResources called",
		zap.Any("filter", filter))

	// Find all VMs in the datacenter
	vms, err := a.finder.VirtualMachineList(ctx, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	var resources []*adapter.Resource
	for _, vm := range vms {
		vmName := vm.Name()

		// Get VM properties
		var vmMo mo.VirtualMachine
		err := vm.Properties(ctx, vm.Reference(), []string{
			"summary",
			"config",
			"guest",
			"runtime",
			"resourcePool",
		}, &vmMo)
		if err != nil {
			a.logger.Warn("failed to get VM properties",
				zap.String("vm", vmName),
				zap.Error(err))
			continue
		}

		resource := a.vmToResource(&vmMo, vmName)

		// Apply filter
		if !adapter.MatchesFilter(filter, resource.ResourcePoolID, resource.ResourceTypeID, vmName, nil) {
			continue
		}

		resources = append(resources, resource)
	}

	// Apply pagination
	if filter != nil {
		resources = adapter.ApplyPagination(resources, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resources",
		zap.Int("count", len(resources)))

	return resources, nil
}

// GetResource retrieves a specific resource (VM) by ID.
func (a *VMwareAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	a.logger.Debug("GetResource called",
		zap.String("id", id))

	// Extract VM name from the ID
	prefix := "vmware-vm-"
	if !strings.HasPrefix(id, prefix) {
		return nil, fmt.Errorf("invalid resource ID format: %s", id)
	}

	// List all resources and find the matching one
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

// CreateResource creates a new resource (VM).
func (a *VMwareAdapter) CreateResource(ctx context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	a.logger.Debug("CreateResource called",
		zap.String("resourceTypeId", resource.ResourceTypeID))

	// Creating VMs requires extensive configuration not available in the O2-IMS model
	return nil, fmt.Errorf("creating vSphere VMs requires additional configuration: use vCenter or vSphere CLI")
}

// DeleteResource deletes a resource (VM) by ID.
func (a *VMwareAdapter) DeleteResource(ctx context.Context, id string) error {
	a.logger.Debug("DeleteResource called",
		zap.String("id", id))

	// Find the VM
	resource, err := a.GetResource(ctx, id)
	if err != nil {
		return err
	}

	// Get VM name from extensions
	vmName, ok := resource.Extensions["vmware.name"].(string)
	if !ok {
		return fmt.Errorf("cannot determine VM name for resource: %s", id)
	}

	// Find the VM object
	vm, err := a.finder.VirtualMachine(ctx, vmName)
	if err != nil {
		return fmt.Errorf("failed to find VM: %w", err)
	}

	// Power off the VM if it's running
	powerState, err := vm.PowerState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get VM power state: %w", err)
	}

	if powerState == types.VirtualMachinePowerStatePoweredOn {
		task, err := vm.PowerOff(ctx)
		if err != nil {
			return fmt.Errorf("failed to power off VM: %w", err)
		}
		if err := task.Wait(ctx); err != nil {
			return fmt.Errorf("failed to wait for VM power off: %w", err)
		}
	}

	// Delete the VM
	task, err := vm.Destroy(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete VM: %w", err)
	}

	if err := task.Wait(ctx); err != nil {
		return fmt.Errorf("failed to wait for VM deletion: %w", err)
	}

	a.logger.Info("deleted resource",
		zap.String("resourceId", id),
		zap.String("vmName", vmName))

	return nil
}

// vmToResource converts a vSphere VM to an O2-IMS Resource.
func (a *VMwareAdapter) vmToResource(vm *mo.VirtualMachine, vmName string) *adapter.Resource {
	// Determine resource pool ID
	var resourcePoolID string
	if a.poolMode == "cluster" {
		// In cluster mode, use the default cluster as the pool
		resourcePoolID = generateClusterPoolID("default")
	} else {
		// In pool mode, use the resource pool reference
		if vm.ResourcePool != nil {
			resourcePoolID = generateResourcePoolID(vm.ResourcePool.Value, "default")
		} else {
			resourcePoolID = generateResourcePoolID("default", "default")
		}
	}

	// Generate resource type ID based on CPU and memory
	var cpuCount int32
	var memoryMB int64
	if vm.Summary.Config != nil {
		cpuCount = vm.Summary.Config.NumCpu
		memoryMB = int64(vm.Summary.Config.MemorySizeMB)
	}
	resourceTypeID := generateVMProfileID(cpuCount, memoryMB)

	// Generate resource ID
	resourceID := generateVMID(vmName, a.datacenterName)

	// Build extensions with VM details
	extensions := map[string]interface{}{
		"vmware.name":       vmName,
		"vmware.datacenter": a.datacenterName,
	}

	// Add summary info
	if vm.Summary.Config != nil {
		config := vm.Summary.Config
		extensions["vmware.guestFullName"] = config.GuestFullName
		extensions["vmware.guestId"] = config.GuestId
		extensions["vmware.numCpu"] = config.NumCpu
		extensions["vmware.memorySizeMB"] = config.MemorySizeMB
		extensions["vmware.uuid"] = config.Uuid
		extensions["vmware.instanceUuid"] = config.InstanceUuid
		extensions["vmware.template"] = config.Template
	}

	// Add runtime info
	if vm.Summary.Runtime != nil {
		runtime := vm.Summary.Runtime
		extensions["vmware.powerState"] = string(runtime.PowerState)
		extensions["vmware.connectionState"] = string(runtime.ConnectionState)
		if runtime.Host != nil {
			extensions["vmware.host"] = runtime.Host.Value
		}
		if runtime.BootTime != nil {
			extensions["vmware.bootTime"] = runtime.BootTime.String()
		}
	}

	// Add quick stats
	if vm.Summary.QuickStats != nil {
		stats := vm.Summary.QuickStats
		extensions["vmware.overallCpuUsage"] = stats.OverallCpuUsage
		extensions["vmware.guestMemoryUsage"] = stats.GuestMemoryUsage
		extensions["vmware.hostMemoryUsage"] = stats.HostMemoryUsage
		extensions["vmware.uptimeSeconds"] = stats.UptimeSeconds
	}

	// Add guest info
	if vm.Guest != nil {
		guest := vm.Guest
		extensions["vmware.guestState"] = string(guest.GuestState)
		extensions["vmware.guestToolsStatus"] = string(guest.ToolsStatus)
		extensions["vmware.guestToolsVersion"] = guest.ToolsVersion
		extensions["vmware.hostName"] = guest.HostName
		extensions["vmware.ipAddress"] = guest.IpAddress

		// Add network info
		if len(guest.Net) > 0 {
			nics := make([]map[string]interface{}, 0, len(guest.Net))
			for _, nic := range guest.Net {
				nicInfo := map[string]interface{}{
					"network":    nic.Network,
					"connected":  nic.Connected,
					"macAddress": nic.MacAddress,
				}
				if len(nic.IpAddress) > 0 {
					nicInfo["ipAddresses"] = nic.IpAddress
				}
				nics = append(nics, nicInfo)
			}
			extensions["vmware.networkInterfaces"] = nics
		}
	}

	// Add storage info
	if vm.Summary.Storage != nil {
		storage := vm.Summary.Storage
		extensions["vmware.committed"] = storage.Committed
		extensions["vmware.uncommitted"] = storage.Uncommitted
		extensions["vmware.unshared"] = storage.Unshared
	}

	// Get description
	description := vmName
	if vm.Summary.Config != nil && vm.Summary.Config.Annotation != "" {
		description = vm.Summary.Config.Annotation
	}

	return &adapter.Resource{
		ResourceID:     resourceID,
		ResourceTypeID: resourceTypeID,
		ResourcePoolID: resourcePoolID,
		GlobalAssetID:  fmt.Sprintf("urn:vmware:vm:%s:%s", a.datacenterName, vmName),
		Description:    description,
		Extensions:     extensions,
	}
}
