package vmware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"go.uber.org/zap"
)

// ListResources retrieves all resources (VMs) matching the provided filter.
func (a *Adapter) ListResources(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.Resource, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "ListResources", start, err) }()

	a.Logger.Debug("ListResources called",
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
			a.Logger.Warn("failed to get VM properties",
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

	a.Logger.Info("listed resources",
		zap.Int("count", len(resources)))

	return resources, nil
}

// GetResource retrieves a specific resource (VM) by ID.
func (a *Adapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "GetResource", start, err) }()

	a.Logger.Debug("GetResource called",
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

	for _, res := range resources {
		if res.ResourceID == id {
			return res, nil
		}
	}

	return nil, fmt.Errorf("resource not found: %s", id)
}

// CreateResource creates a new resource (VM).
func (a *Adapter) CreateResource(_ context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "CreateResource", start, err) }()

	a.Logger.Debug("CreateResource called",
		zap.String("resourceTypeId", resource.ResourceTypeID))

	// Creating VMs requires extensive configuration not available in the O2-IMS model
	return nil, fmt.Errorf("creating vSphere VMs requires additional configuration: use vCenter or vSphere CLI")
}

// UpdateResource updates an existing vSphere VM's annotations and custom attributes.
// Note: Core VM properties cannot be modified while VM is running.
// Only VM annotations (description) and custom attributes (via Extensions) can be updated.
func (a *Adapter) UpdateResource(
	ctx context.Context,
	id string,
	resource *adapter.Resource,
) (*adapter.Resource, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "UpdateResource", start, err) }()

	a.Logger.Debug("UpdateResource called",
		zap.String("resourceID", id))

	// Validate resource ID format
	if err = validateVMResourceID(id); err != nil {
		return nil, err
	}

	// Get VM name from current resource
	vmName, err := a.getVMNameFromResource(ctx, id)
	if err != nil {
		return nil, err
	}

	// Find the VM object
	vm, err := a.finder.VirtualMachine(ctx, vmName)
	if err != nil {
		err = fmt.Errorf("failed to find VM: %w", err)
		return nil, err
	}

	// Build and apply configuration updates
	if err = a.applyVMUpdates(ctx, vm, resource, vmName, id); err != nil {
		return nil, err
	}

	// Fetch and return updated resource
	return a.GetResource(ctx, id)
}

// validateVMResourceID validates the resource ID format.
func validateVMResourceID(id string) error {
	prefix := "vmware-vm-"
	if !strings.HasPrefix(id, prefix) {
		return fmt.Errorf("invalid resource ID format: %s", id)
	}
	return nil
}

// getVMNameFromResource extracts the VM name from a resource.
func (a *Adapter) getVMNameFromResource(ctx context.Context, id string) (string, error) {
	currentResource, err := a.GetResource(ctx, id)
	if err != nil {
		return "", err
	}

	vmName, ok := currentResource.Extensions["vmware.name"].(string)
	if !ok {
		return "", fmt.Errorf("cannot determine VM name for resource: %s", id)
	}

	return vmName, nil
}

// applyVMUpdates applies configuration updates to a VM.
func (a *Adapter) applyVMUpdates(
	ctx context.Context,
	vm *object.VirtualMachine,
	resource *adapter.Resource,
	vmName, resourceID string,
) error {
	annotation := buildVMAnnotation(resource)
	if annotation == "" {
		return nil // No updates to apply
	}

	spec := types.VirtualMachineConfigSpec{
		Annotation: annotation,
	}

	task, err := vm.Reconfigure(ctx, spec)
	if err != nil {
		return fmt.Errorf("failed to reconfigure VM: %w", err)
	}

	if err := task.Wait(ctx); err != nil {
		return fmt.Errorf("failed to wait for VM reconfiguration: %w", err)
	}

	a.Logger.Info("updated VM annotation",
		zap.String("vmName", vmName),
		zap.String("resourceID", resourceID))

	return nil
}

// buildVMAnnotation builds the VM annotation content from resource fields.
func buildVMAnnotation(resource *adapter.Resource) string {
	annotation := resource.Description

	if resource.Extensions != nil {
		customAttrs := extractCustomAttributes(resource.Extensions)
		if len(customAttrs) > 0 {
			annotation += "\n\nCustom Attributes:"
			for key, value := range customAttrs {
				annotation += fmt.Sprintf("\n%s=%s", key, value)
			}
		}
	}

	return annotation
}

// extractCustomAttributes extracts custom attributes from extensions.
func extractCustomAttributes(extensions map[string]interface{}) map[string]string {
	customAttrs := make(map[string]string)

	if attrs, ok := extensions["vmware.customAttributes"].(map[string]string); ok {
		for k, v := range attrs {
			customAttrs[k] = v
		}
	}

	return customAttrs
}

// DeleteResource deletes a resource (VM) by ID.
func (a *Adapter) DeleteResource(ctx context.Context, id string) error {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "DeleteResource", start, err) }()

	a.Logger.Debug("DeleteResource called",
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

	a.Logger.Info("deleted resource",
		zap.String("resourceId", id),
		zap.String("vmName", vmName))

	return nil
}

// vmToResource converts a vSphere VM to an O2-IMS Resource.
func (a *Adapter) vmToResource(vm *mo.VirtualMachine, vmName string) *adapter.Resource {
	config := vm.Summary.Config

	// Determine resource pool and type IDs
	resourcePoolID := a.determineVMResourcePoolID(vm)
	resourceTypeID := GenerateVMProfileID(config.NumCpu, int64(config.MemorySizeMB))
	resourceID := GenerateVMID(vmName, a.datacenterName)

	// Build extensions with VM details
	extensions := buildVMExtensions(vm, vmName, a.datacenterName)

	// Get description
	description := getVMDescription(vmName, config.Annotation)

	return &adapter.Resource{
		ResourceID:     resourceID,
		ResourceTypeID: resourceTypeID,
		ResourcePoolID: resourcePoolID,
		GlobalAssetID:  fmt.Sprintf("urn:vmware:vm:%s:%s", a.datacenterName, vmName),
		Description:    description,
		Extensions:     extensions,
	}
}

// determineVMResourcePoolID determines the resource pool ID based on pool mode.
func (a *Adapter) determineVMResourcePoolID(vm *mo.VirtualMachine) string {
	if a.poolMode == "cluster" {
		return GenerateClusterPoolID("default")
	}

	// In pool mode, use the resource pool reference
	if vm.ResourcePool != nil {
		return GenerateResourcePoolID(vm.ResourcePool.Value, "default")
	}
	return GenerateResourcePoolID("default", "default")
}

// buildVMExtensions builds the extensions map with VM details.
func buildVMExtensions(vm *mo.VirtualMachine, vmName, datacenterName string) map[string]interface{} {
	extensions := map[string]interface{}{
		"vmware.name":       vmName,
		"vmware.datacenter": datacenterName,
	}

	addVMConfigInfo(extensions, vm.Summary.Config)
	addVMRuntimeInfo(extensions, vm.Summary.Runtime)
	addVMQuickStats(extensions, vm.Summary.QuickStats)
	addVMGuestInfo(extensions, vm.Guest)
	addVMStorageInfo(extensions, vm.Summary.Storage)

	return extensions
}

// addVMConfigInfo adds VM configuration information to extensions.
func addVMConfigInfo(extensions map[string]interface{}, config types.VirtualMachineConfigSummary) {
	extensions["vmware.guestFullName"] = config.GuestFullName
	extensions["vmware.guestId"] = config.GuestId
	extensions["vmware.numCpu"] = config.NumCpu
	extensions["vmware.memorySizeMB"] = config.MemorySizeMB
	extensions["vmware.uuid"] = config.Uuid
	extensions["vmware.instanceUuid"] = config.InstanceUuid
	extensions["vmware.template"] = config.Template
}

// addVMRuntimeInfo adds VM runtime information to extensions.
func addVMRuntimeInfo(extensions map[string]interface{}, runtime types.VirtualMachineRuntimeInfo) {
	extensions["vmware.powerState"] = string(runtime.PowerState)
	extensions["vmware.connectionState"] = string(runtime.ConnectionState)
	if runtime.Host != nil {
		extensions["vmware.host"] = runtime.Host.Value
	}
	if runtime.BootTime != nil {
		extensions["vmware.bootTime"] = runtime.BootTime.String()
	}
}

// addVMQuickStats adds VM quick statistics to extensions.
func addVMQuickStats(extensions map[string]interface{}, stats types.VirtualMachineQuickStats) {
	extensions["vmware.overallCpuUsage"] = stats.OverallCpuUsage
	extensions["vmware.guestMemoryUsage"] = stats.GuestMemoryUsage
	extensions["vmware.hostMemoryUsage"] = stats.HostMemoryUsage
	extensions["vmware.uptimeSeconds"] = stats.UptimeSeconds
}

// addVMGuestInfo adds VM guest information to extensions.
func addVMGuestInfo(extensions map[string]interface{}, guest *types.GuestInfo) {
	if guest == nil {
		return
	}

	extensions["vmware.guestState"] = guest.GuestState
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

// addVMStorageInfo adds VM storage information to extensions.
func addVMStorageInfo(extensions map[string]interface{}, storage *types.VirtualMachineStorageSummary) {
	if storage != nil {
		extensions["vmware.committed"] = storage.Committed
		extensions["vmware.uncommitted"] = storage.Uncommitted
		extensions["vmware.unshared"] = storage.Unshared
	}
}

// getVMDescription returns a description for the VM.
func getVMDescription(vmName, annotation string) string {
	if annotation != "" {
		return annotation
	}
	return vmName
}
