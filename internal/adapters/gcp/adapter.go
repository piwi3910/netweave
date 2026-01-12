// Package gcp provides a Google Cloud Platform implementation of the O2-IMS adapter interface.
// It translates O2-IMS API operations to GCP API calls, mapping O2-IMS resources
// to GCP resources like Compute Engine instances, zones, and machine types.
//
// Resource Mapping:
//   - Resource Pools → Zones or Instance Groups
//   - Resources → Compute Engine Instances
//   - Resource Types → Machine Types
//   - Deployment Manager → GCP Project/Region metadata
package gcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

const (
	poolModeZone = "zone"
)

// GCPAdapter implements the adapter.Adapter interface for GCP backends.
// It provides O2-IMS functionality by mapping O2-IMS resources to GCP resources:
//   - Resource Pools → Zones or Managed Instance Groups
//   - Resources → Compute Engine Instances
//   - Resource Types → Machine Types
//   - Deployment Manager → GCP Project/Region metadata
//   - Subscriptions → Pub/Sub based (polling as fallback)
type Adapter struct {
	// instancesClient is the GCP Compute Engine instances client.
	instancesClient *compute.InstancesClient

	// machineTypesClient is the GCP Machine Types client.
	machineTypesClient *compute.MachineTypesClient

	// zonesClient is the GCP Zones client.
	zonesClient *compute.ZonesClient

	// regionsClient is the GCP Regions client.
	regionsClient *compute.RegionsClient

	// instanceGroupsClient is the GCP Instance Groups client.
	instanceGroupsClient *compute.InstanceGroupsClient

	// logger provides structured logging.
	logger *zap.Logger

	// oCloudID is the identifier of the parent O-Cloud.
	oCloudID string

	// deploymentManagerID is the identifier for this deployment manager.
	deploymentManagerID string

	// projectID is the GCP project ID.
	projectID string

	// region is the GCP region (e.g., "us-central1").
	region string

	// subscriptions holds active O2-IMS subscriptions (polling-based fallback).
	// Note: Subscriptions are stored in-memory and will be lost on adapter restart.
	// For production use, consider implementing persistent storage via Redis.
	subscriptions map[string]*adapter.Subscription

	// subscriptionsMu protects the subscriptions map.
	subscriptionsMu sync.RWMutex

	// poolMode determines how resource pools are mapped.
	// "zone" maps to Zones, "ig" maps to Instance Groups.
	poolMode string
}

// Config holds configuration for creating a GCPAdapter.
type Config struct {
	// ProjectID is the GCP project ID.
	ProjectID string

	// Region is the GCP region (e.g., "us-central1").
	Region string

	// CredentialsFile is the path to the GCP service account JSON credentials file.
	// If empty, the default credentials will be used (ADC).
	CredentialsFile string

	// CredentialsJSON is the raw JSON content of the service account credentials.
	// If provided, CredentialsFile is ignored.
	CredentialsJSON []byte

	// OCloudID is the identifier of the parent O-Cloud.
	OCloudID string

	// DeploymentManagerID is the identifier for this deployment manager.
	// If empty, defaults to "ocloud-gcp-{region}".
	DeploymentManagerID string

	// PoolMode determines how resource pools are mapped:
	// - "zone": Map to Zones (default)
	// - "ig": Map to Instance Groups
	PoolMode string

	// Timeout is the timeout for GCP API calls.
	// Defaults to 30 seconds if not specified.
	Timeout time.Duration

	// Logger is the logger to use. If nil, a default logger will be created.
	Logger *zap.Logger
}

// New creates a new GCPAdapter with the provided configuration.
// It authenticates with GCP and initializes service clients.
//
// Example:
//
//	adp, err := gcp.New(&gcp.Config{
//	    ProjectID:       "my-project",
//	    Region:          "us-central1",
//	    CredentialsFile: "/path/to/credentials.json",
//	    OCloudID:        "ocloud-gcp-1",
//	    PoolMode:        "zone",
//	})
func New(cfg *Config) (*Adapter, error) {
	if err := validateGCPConfig(cfg); err != nil {
		return nil, err
	}

	deploymentManagerID, poolMode := applyGCPDefaults(cfg)
	logger, err := initializeGCPLogger(cfg.Logger)
	if err != nil {
		return nil, err
	}

	logger.Info("initializing GCP adapter",
		zap.String("projectID", cfg.ProjectID),
		zap.String("region", cfg.Region),
		zap.String("oCloudID", cfg.OCloudID),
		zap.String("poolMode", poolMode))

	opts := buildGCPClientOptions(cfg, logger)
	clients, err := createGCPClients(context.Background(), opts, logger)
	if err != nil {
		return nil, err
	}

	return &Adapter{
		instancesClient:      clients.instancesClient,
		machineTypesClient:   clients.machineTypesClient,
		zonesClient:          clients.zonesClient,
		regionsClient:        clients.regionsClient,
		instanceGroupsClient: clients.instanceGroupsClient,
		logger:               logger,
		oCloudID:             cfg.OCloudID,
		deploymentManagerID:  deploymentManagerID,
		projectID:            cfg.ProjectID,
		region:               cfg.Region,
		subscriptions:        make(map[string]*adapter.Subscription),
		poolMode:             poolMode,
	}, nil
}

// validateGCPConfig validates required configuration fields.
func validateGCPConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}
	if cfg.ProjectID == "" {
		return fmt.Errorf("projectID is required")
	}
	if cfg.Region == "" {
		return fmt.Errorf("region is required")
	}
	if cfg.OCloudID == "" {
		return fmt.Errorf("oCloudID is required")
	}

	// Validate poolMode if provided
	if cfg.PoolMode != "" && cfg.PoolMode != poolModeZone && cfg.PoolMode != "ig" {
		return fmt.Errorf("poolMode must be 'zone' or 'ig', got %q", cfg.PoolMode)
	}

	return nil
}

// applyGCPDefaults applies default values to configuration.
func applyGCPDefaults(cfg *Config) (deploymentManagerID, poolMode string) {
	deploymentManagerID = cfg.DeploymentManagerID
	if deploymentManagerID == "" {
		deploymentManagerID = fmt.Sprintf("ocloud-gcp-%s", cfg.Region)
	}

	poolMode = cfg.PoolMode
	if poolMode == "" {
		poolMode = poolModeZone
	}

	return deploymentManagerID, poolMode
}

// initializeGCPLogger creates or returns the configured logger.
func initializeGCPLogger(logger *zap.Logger) (*zap.Logger, error) {
	if logger != nil {
		return logger, nil
	}
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	return logger, nil
}

// buildGCPClientOptions builds GCP client options.
func buildGCPClientOptions(cfg *Config, logger *zap.Logger) []option.ClientOption {
	var opts []option.ClientOption
	switch {
	case len(cfg.CredentialsJSON) > 0:
		opts = append(opts, option.WithCredentialsJSON(cfg.CredentialsJSON))
		logger.Info("using JSON credentials for authentication")
	case cfg.CredentialsFile != "":
		opts = append(opts, option.WithCredentialsFile(cfg.CredentialsFile))
		logger.Info("using credentials file for authentication",
			zap.String("credentialsFile", cfg.CredentialsFile))
	default:
		logger.Info("using default GCP credentials (ADC)")
	}
	return opts
}

// gcpClients holds GCP compute clients.
type gcpClients struct {
	instancesClient      *compute.InstancesClient
	machineTypesClient   *compute.MachineTypesClient
	zonesClient          *compute.ZonesClient
	regionsClient        *compute.RegionsClient
	instanceGroupsClient *compute.InstanceGroupsClient
}

// createGCPClients creates all required GCP compute clients.
func createGCPClients(ctx context.Context, opts []option.ClientOption, logger *zap.Logger) (*gcpClients, error) {
	instancesClient, err := compute.NewInstancesRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Instances client: %w", err)
	}

	machineTypesClient, err := compute.NewMachineTypesRESTClient(ctx, opts...)
	if err != nil {
		closeClients(logger, instancesClient)
		return nil, fmt.Errorf("failed to create Machine Types client: %w", err)
	}

	zonesClient, err := compute.NewZonesRESTClient(ctx, opts...)
	if err != nil {
		closeClients(logger, instancesClient, machineTypesClient)
		return nil, fmt.Errorf("failed to create Zones client: %w", err)
	}

	regionsClient, err := compute.NewRegionsRESTClient(ctx, opts...)
	if err != nil {
		closeClients(logger, instancesClient, machineTypesClient, zonesClient)
		return nil, fmt.Errorf("failed to create Regions client: %w", err)
	}

	instanceGroupsClient, err := compute.NewInstanceGroupsRESTClient(ctx, opts...)
	if err != nil {
		closeClients(logger, instancesClient, machineTypesClient, zonesClient, regionsClient)
		return nil, fmt.Errorf("failed to create Instance Groups client: %w", err)
	}

	return &gcpClients{
		instancesClient:      instancesClient,
		machineTypesClient:   machineTypesClient,
		zonesClient:          zonesClient,
		regionsClient:        regionsClient,
		instanceGroupsClient: instanceGroupsClient,
	}, nil
}

// closeClients closes multiple GCP clients and logs any errors.
// This is a cleanup helper for error paths during client initialization.
type closer interface {
	Close() error
}

func closeClients(logger *zap.Logger, clients ...closer) {
	for _, client := range clients {
		if client != nil {
			// G104: Handle Close() error in cleanup path
			if err := client.Close(); err != nil {
				logger.Warn("failed to close client during cleanup", zap.Error(err))
			}
		}
	}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "gcp"
}

// Version returns the GCP API version this adapter supports.
func (a *Adapter) Version() string {
	return "compute-v1"
}

// Capabilities returns the list of O2-IMS capabilities supported by this adapter.
func (a *Adapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityResourcePools,
		adapter.CapabilityResources,
		adapter.CapabilityResourceTypes,
		adapter.CapabilityDeploymentManagers,
		adapter.CapabilitySubscriptions, // Polling-based
		adapter.CapabilityHealthChecks,
	}
}

// Health performs a health check on the GCP backend.
// It verifies connectivity to GCP services.
func (a *Adapter) Health(ctx context.Context) (err error) {
	start := time.Now()
	defer func() { adapter.ObserveHealthCheck("gcp", start, err) }()

	a.logger.Debug("health check called")

	// Use a timeout to prevent indefinite blocking
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check by getting the region
	_, err = a.regionsClient.Get(healthCtx, &computepb.GetRegionRequest{
		Project: a.projectID,
		Region:  a.region,
	})
	if err != nil {
		a.logger.Error("GCP health check failed", zap.Error(err))
		return fmt.Errorf("gcp API unreachable: %w", err)
	}

	a.logger.Debug("health check passed")
	return nil
}

// Close cleanly shuts down the adapter and releases resources.
func (a *Adapter) Close() error {
	a.logger.Info("closing GCP adapter")

	// Clear subscriptions
	a.subscriptionsMu.Lock()
	a.subscriptions = make(map[string]*adapter.Subscription)
	a.subscriptionsMu.Unlock()

	// Close all clients
	var errs []error
	if err := a.instancesClient.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := a.machineTypesClient.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := a.zonesClient.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := a.regionsClient.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := a.instanceGroupsClient.Close(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing GCP clients: %v", errs)
	}

	// Sync logger before shutdown
	// Ignore sync errors on stderr/stdout
	_ = a.logger.Sync()

	return nil
}

// NOTE: Filter matching and pagination use shared helpers from internal/adapter/helpers.go
// Use adapter.MatchesFilter() and adapter.ApplyPagination() instead of local implementations.

// generateMachineTypeID generates a consistent resource type ID for a machine type.
func generateMachineTypeID(machineType string) string {
	return fmt.Sprintf("gcp-machine-type-%s", machineType)
}

// generateInstanceID generates a consistent resource ID for a GCP instance.
func generateInstanceID(instanceName, zone string) string {
	return fmt.Sprintf("gcp-instance-%s-%s", zone, instanceName)
}

// generateZonePoolID generates a consistent resource pool ID for a Zone.
func generateZonePoolID(zone string) string {
	return fmt.Sprintf("gcp-zone-%s", zone)
}

// generateIGPoolID generates a consistent resource pool ID for an Instance Group.
func generateIGPoolID(igName, zone string) string {
	return fmt.Sprintf("gcp-ig-%s-%s", zone, igName)
}

// ptrToString safely converts a *string to string.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ptrToInt64 safely converts a *int64 to int64.
func ptrToInt64(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

// ptrToInt32 safely converts a *int32 to int32.
func ptrToInt32(i *int32) int32 {
	if i == nil {
		return 0
	}
	return *i
}

// ptrToBool safely converts a *bool to bool.
func ptrToBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
