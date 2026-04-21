package gcp

import (
	"context"
	"fmt"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

// ExportValidateGCPConfig exposes validateGCPConfig for testing.
func ExportValidateGCPConfig(cfg *Config) error {
	return validateGCPConfig(cfg)
}

// ExportApplyGCPDefaults exposes applyGCPDefaults for testing.
func ExportApplyGCPDefaults(cfg *Config) (string, string) {
	return applyGCPDefaults(cfg)
}

// ExportInitializeGCPLogger exposes initializeGCPLogger for testing.
func ExportInitializeGCPLogger(logger *zap.Logger) (*zap.Logger, error) {
	return initializeGCPLogger(logger)
}

// ExportBuildGCPClientOptions exposes buildGCPClientOptions for testing.
func ExportBuildGCPClientOptions(cfg *Config, logger *zap.Logger) int {
	opts := buildGCPClientOptions(cfg, logger)
	return len(opts)
}

// ExportCloseClients exposes closeClients for testing.
func ExportCloseClients(logger *zap.Logger, clients ...closer) {
	closeClients(logger, clients...)
}

// ExportMachineTypeToResourceType exposes machineTypeToResourceType for testing.
func (a *Adapter) ExportMachineTypeToResourceType(mt *computepb.MachineType) *adapter.ResourceType {
	return a.machineTypeToResourceType(mt)
}

// ExportAddInstanceNetworkInterfaces exposes addInstanceNetworkInterfaces for testing.
func ExportAddInstanceNetworkInterfaces(extensions map[string]interface{}, nics []*computepb.NetworkInterface) {
	addInstanceNetworkInterfaces(extensions, nics)
}

// ExportAddInstanceDisks exposes addInstanceDisks for testing.
func ExportAddInstanceDisks(extensions map[string]interface{}, disks []*computepb.AttachedDisk) {
	addInstanceDisks(extensions, disks)
}

// ExportAddInstanceSchedulingInfo exposes addInstanceSchedulingInfo for testing.
func ExportAddInstanceSchedulingInfo(extensions map[string]interface{}, scheduling *computepb.Scheduling) {
	addInstanceSchedulingInfo(extensions, scheduling)
}

// ExportInstanceToResource exposes instanceToResource for testing.
func (a *Adapter) ExportInstanceToResource(instance *computepb.Instance, zone string) *adapter.Resource {
	return a.instanceToResource(instance, zone)
}

// ExportBuildInstanceExtensions exposes buildInstanceExtensions for testing.
func ExportBuildInstanceExtensions(instance *computepb.Instance, instanceName, zone, machineType string) map[string]interface{} {
	return buildInstanceExtensions(instance, instanceName, zone, machineType)
}

// MockCloser is a test helper implementing the closer interface.
type MockCloser struct {
	Err error
}

// Close implements the closer interface.
func (m *MockCloser) Close() error {
	return m.Err
}

// ExportNewFakeGCPAdapter creates a GCP Adapter backed by a fake HTTP server endpoint.
// The endpoint should be the URL of a local httptest server.
func ExportNewFakeGCPAdapter(ctx context.Context, endpoint string) (*Adapter, error) {
	opts := []option.ClientOption{
		option.WithEndpoint(endpoint),
		option.WithoutAuthentication(),
	}

	instancesClient, err := compute.NewInstancesRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Instances client: %w", err)
	}

	machineTypesClient, err := compute.NewMachineTypesRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Machine Types client: %w", err)
	}

	zonesClient, err := compute.NewZonesRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Zones client: %w", err)
	}

	regionsClient, err := compute.NewRegionsRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Regions client: %w", err)
	}

	instanceGroupsClient, err := compute.NewInstanceGroupsRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Instance Groups client: %w", err)
	}

	return &Adapter{
		instancesClient:      instancesClient,
		machineTypesClient:   machineTypesClient,
		zonesClient:          zonesClient,
		regionsClient:        regionsClient,
		instanceGroupsClient: instanceGroupsClient,
		logger:               zap.NewNop(),
		oCloudID:             "test-ocloud",
		deploymentManagerID:  "test-dm-gcp",
		projectID:            "test-project",
		region:               "us-central1",
		subs:                 adapter.NewInMemorySubscriptionStore(),
		poolMode:             poolModeZone,
	}, nil
}

// NewTestAdapter creates a GCP Adapter for testing with a no-op logger.
func NewTestAdapter() *Adapter {
	return &Adapter{
		logger: zap.NewNop(),
		subs:   adapter.NewInMemorySubscriptionStore(),
	}
}

// ExportSubscriptions exposes the underlying subscriptions map for tests.
// Callers must not access it concurrently with adapter operations.
func (a *Adapter) ExportSubscriptions() map[string]*adapter.Subscription {
	return a.subs.RawMap()
}

// ExportCloseAdapter closes all clients on the adapter.
func (a *Adapter) ExportCloseAdapter() {
	if a.instancesClient != nil {
		a.instancesClient.Close()
	}
	if a.machineTypesClient != nil {
		a.machineTypesClient.Close()
	}
	if a.zonesClient != nil {
		a.zonesClient.Close()
	}
	if a.regionsClient != nil {
		a.regionsClient.Close()
	}
	if a.instanceGroupsClient != nil {
		a.instanceGroupsClient.Close()
	}
}
