package vmware

import (
	"context"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"go.uber.org/zap"
)

// ExportNewSimulatorAdapter creates an Adapter using a simulator vim25.Client and datacenter.
// This allows testing functions that need a real finder/client.
func ExportNewSimulatorAdapter(ctx context.Context, c *vim25.Client, datacenterName, poolMode string) (*Adapter, error) {
	gc := &govmomi.Client{
		Client:         c,
		SessionManager: session.NewManager(c),
	}

	f := find.NewFinder(c, true)
	dc, err := f.Datacenter(ctx, datacenterName)
	if err != nil {
		return nil, err
	}
	f.SetDatacenter(dc)

	if poolMode == "" {
		poolMode = poolModeCluster
	}

	return &Adapter{
		client:              gc,
		finder:              f,
		datacenter:          dc,
		Logger:              zap.NewNop(),
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm-vmware",
		vcenterURL:          "https://vcenter.test/sdk",
		datacenterName:      datacenterName,
		Subscriptions:       make(map[string]*adapter.Subscription),
		poolMode:            poolMode,
	}, nil
}

// ExportFinder returns the adapter's finder for testing.
func (a *Adapter) ExportFinder() *find.Finder {
	return a.finder
}

// ExportDatacenter returns the adapter's datacenter for testing.
func (a *Adapter) ExportDatacenter() *object.Datacenter {
	return a.datacenter
}

// ExportBuildVMExtensions exposes buildVMExtensions for testing.
func ExportBuildVMExtensions(vm *mo.VirtualMachine, vmName, datacenterName string) map[string]interface{} {
	return buildVMExtensions(vm, vmName, datacenterName)
}

// ExportAddVMConfigInfo exposes addVMConfigInfo for testing.
func ExportAddVMConfigInfo(extensions map[string]interface{}, config types.VirtualMachineConfigSummary) {
	addVMConfigInfo(extensions, config)
}

// ExportAddVMRuntimeInfo exposes addVMRuntimeInfo for testing.
func ExportAddVMRuntimeInfo(extensions map[string]interface{}, runtime types.VirtualMachineRuntimeInfo) {
	addVMRuntimeInfo(extensions, runtime)
}

// ExportAddVMQuickStats exposes addVMQuickStats for testing.
func ExportAddVMQuickStats(extensions map[string]interface{}, stats types.VirtualMachineQuickStats) {
	addVMQuickStats(extensions, stats)
}

// ExportAddVMGuestInfo exposes addVMGuestInfo for testing.
func ExportAddVMGuestInfo(extensions map[string]interface{}, guest *types.GuestInfo) {
	addVMGuestInfo(extensions, guest)
}

// ExportAddVMStorageInfo exposes addVMStorageInfo for testing.
func ExportAddVMStorageInfo(extensions map[string]interface{}, storage *types.VirtualMachineStorageSummary) {
	addVMStorageInfo(extensions, storage)
}

// ExportDetermineVMResourcePoolID exposes determineVMResourcePoolID for testing.
func (a *Adapter) ExportDetermineVMResourcePoolID(vm *mo.VirtualMachine) string {
	return a.determineVMResourcePoolID(vm)
}

// ExportExtractTenantIDFromVM exposes extractTenantIDFromVM for testing.
func (a *Adapter) ExportExtractTenantIDFromVM(vm *mo.VirtualMachine) string {
	return a.extractTenantIDFromVM(vm)
}

// ExportValidateVMwareConfig exposes validateVMwareConfig for testing.
func ExportValidateVMwareConfig(cfg *Config) error {
	return validateVMwareConfig(cfg)
}

// ExportApplyVMwareDefaults exposes applyVMwareDefaults for testing.
func ExportApplyVMwareDefaults(cfg *Config) (string, string, time.Duration) {
	return applyVMwareDefaults(cfg)
}

// ExportVMToResource exposes vmToResource for testing.
func (a *Adapter) ExportVMToResource(vm *mo.VirtualMachine, vmName string) *adapter.Resource {
	return a.vmToResource(vm, vmName)
}

// ExportInitializeVMwareLogger exposes initializeVMwareLogger for testing.
func ExportInitializeVMwareLogger(logger *zap.Logger) (*zap.Logger, error) {
	return initializeVMwareLogger(logger)
}

// ExportGetVMDescription exposes getVMDescription for testing.
func ExportGetVMDescription(vmName, annotation string) string {
	return getVMDescription(vmName, annotation)
}
