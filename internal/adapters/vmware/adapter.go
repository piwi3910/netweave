// Package vmware provides a VMware vSphere implementation of the O2-IMS adapter interface.
// It translates O2-IMS API operations to vSphere API calls, mapping O2-IMS resources
// to vSphere resources like VMs, resource pools, and clusters.
//
// Resource Mapping:
//   - Resource Pools → vSphere Clusters or Resource Pools
//   - Resources → Virtual Machines
//   - Resource Types → VM hardware profiles (CPU, memory, storage)
//   - Deployment Manager → vCenter Server/Datacenter metadata
package vmware

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/session/cache"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"go.uber.org/zap"
)

const (
	poolModeCluster = "cluster"
)

// Adapter implements the adapter.Adapter interface for VMware vSphere backends.
// It provides O2-IMS functionality by mapping O2-IMS resources to vSphere resources:
//   - Resource Pools → vSphere Clusters or Resource Pools
//   - Resources → Virtual Machines
//   - Resource Types → VM hardware profiles
//   - Deployment Manager → vCenter/Datacenter metadata
//   - Subscriptions → Polling-based (vSphere events as fallback)
type Adapter struct {
	// client is the vSphere API client.
	client *govmomi.Client

	// finder is used to locate vSphere objects.
	finder *find.Finder

	// datacenter is the vSphere datacenter.
	datacenter *object.Datacenter

	// logger provides structured logging.
	Logger *zap.Logger

	// oCloudID is the identifier of the parent O-Cloud.
	oCloudID string

	// deploymentManagerID is the identifier for this deployment manager.
	deploymentManagerID string

	// vcenterURL is the vCenter server URL.
	vcenterURL string

	// datacenterName is the vSphere datacenter name.
	datacenterName string

	// subscriptions holds active O2-IMS subscriptions (polling-based fallback).
	// Note: Subscriptions are stored in-memory and will be lost on adapter restart.
	// For production use, consider implementing persistent storage via Redis.
	Subscriptions map[string]*adapter.Subscription

	// subscriptionsMu protects the subscriptions map.
	SubscriptionsMu sync.RWMutex

	// poolMode determines how resource pools are mapped.
	// "cluster" maps to Clusters, "pool" maps to Resource Pools.
	poolMode string
}

// Config holds configuration for creating a VMwareAdapter.
type Config struct {
	// VCenterURL is the vCenter server URL (e.g., "https://vcenter.example.com/sdk").
	VCenterURL string

	// Username is the vCenter username.
	Username string

	// Password is the vCenter password.
	Password string

	// Datacenter is the vSphere datacenter name.
	Datacenter string

	// InsecureSkipVerify skips TLS certificate verification (NOT for production).
	InsecureSkipVerify bool

	// OCloudID is the identifier of the parent O-Cloud.
	OCloudID string

	// DeploymentManagerID is the identifier for this deployment manager.
	// If empty, defaults to "ocloud-vmware-{datacenter}".
	DeploymentManagerID string

	// PoolMode determines how resource pools are mapped:
	// - "cluster": Map to Clusters (default)
	// - "pool": Map to Resource Pools
	PoolMode string

	// Timeout is the timeout for vSphere API calls.
	// Defaults to 30 seconds if not specified.
	Timeout time.Duration

	// Logger is the logger to use. If nil, a default logger will be created.
	Logger *zap.Logger
}

// New creates a new VMwareAdapter with the provided configuration.
// It connects to vCenter and initializes the vSphere client.
//
// Example:
//
//	adp, err := vmware.New(&vmware.Config{
//	    VCenterURL:         "https://vcenter.example.com/sdk",
//	    Username:           os.Getenv("VSPHERE_USER"),
//	    Password:           os.Getenv("VSPHERE_PASSWORD"),
//	    Datacenter:         "DC1",
//	    InsecureSkipVerify: true,
//	    OCloudID:           "ocloud-vmware-1",
//	    PoolMode:           "cluster",
//	})
func New(cfg *Config) (*Adapter, error) {
	if err := validateVMwareConfig(cfg); err != nil {
		return nil, err
	}

	deploymentManagerID, poolMode, timeout := applyVMwareDefaults(cfg)
	logger, err := initializeVMwareLogger(cfg.Logger)
	if err != nil {
		return nil, err
	}

	logger.Info("initializing VMware adapter",
		zap.String("vCenterURL", cfg.VCenterURL),
		zap.String("datacenter", cfg.Datacenter),
		zap.String("oCloudID", cfg.OCloudID),
		zap.String("poolMode", poolMode))

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := createVMwareClient(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	finder, dc, err := setupVMwareDatacenter(ctx, client, cfg.Datacenter, logger)
	if err != nil {
		// G104: Handle logout error - log if it fails but don't override the original error
		if logoutErr := client.Logout(ctx); logoutErr != nil {
			logger.Warn("failed to logout after datacenter setup error", zap.Error(logoutErr))
		}
		return nil, err
	}

	return &Adapter{
		client:              client,
		finder:              finder,
		datacenter:          dc,
		Logger:              logger,
		oCloudID:            cfg.OCloudID,
		deploymentManagerID: deploymentManagerID,
		vcenterURL:          cfg.VCenterURL,
		datacenterName:      cfg.Datacenter,
		Subscriptions:       make(map[string]*adapter.Subscription),
		poolMode:            poolMode,
	}, nil
}

// validateVMwareConfig validates required configuration fields.
func validateVMwareConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}
	if cfg.VCenterURL == "" {
		return fmt.Errorf("vCenterURL is required")
	}
	if cfg.Username == "" {
		return fmt.Errorf("username is required")
	}
	if cfg.Password == "" {
		return fmt.Errorf("password is required")
	}
	if cfg.Datacenter == "" {
		return fmt.Errorf("datacenter is required")
	}
	if cfg.OCloudID == "" {
		return fmt.Errorf("oCloudID is required")
	}

	// Validate poolMode if provided
	if cfg.PoolMode != "" && cfg.PoolMode != "cluster" && cfg.PoolMode != "pool" {
		return fmt.Errorf("poolMode must be 'cluster' or 'pool', got %q", cfg.PoolMode)
	}

	return nil
}

// applyVMwareDefaults applies default values to configuration.
func applyVMwareDefaults(cfg *Config) (string, string, time.Duration) {
	deploymentManagerID := cfg.DeploymentManagerID
	if deploymentManagerID == "" {
		deploymentManagerID = fmt.Sprintf("ocloud-vmware-%s", cfg.Datacenter)
	}

	poolMode := cfg.PoolMode
	if poolMode == "" {
		poolMode = poolModeCluster
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return deploymentManagerID, poolMode, timeout
}

// initializeVMwareLogger creates or returns the configured logger.
func initializeVMwareLogger(logger *zap.Logger) (*zap.Logger, error) {
	if logger != nil {
		return logger, nil
	}
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to create Logger: %w", err)
	}
	return logger, nil
}

// createVMwareClient creates and authenticates a vCenter client.
func createVMwareClient(ctx context.Context, cfg *Config, logger *zap.Logger) (*govmomi.Client, error) {
	u, err := soap.ParseURL(cfg.VCenterURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vCenter URL: %w", err)
	}
	u.User = url.UserPassword(cfg.Username, cfg.Password)

	s := &cache.Session{
		URL:      u,
		Insecure: cfg.InsecureSkipVerify,
	}

	c := new(vim25.Client)
	if err := s.Login(ctx, c, nil); err != nil {
		return nil, fmt.Errorf("failed to login to vCenter: %w", err)
	}

	client := &govmomi.Client{
		Client:         c,
		SessionManager: session.NewManager(c),
	}

	logger.Info("connected to vCenter", zap.String("vCenterURL", cfg.VCenterURL))
	return client, nil
}

// setupVMwareDatacenter creates a finder and locates the datacenter.
func setupVMwareDatacenter(
	ctx context.Context,
	client *govmomi.Client,
	datacenterName string,
	logger *zap.Logger,
) (*find.Finder, *object.Datacenter, error) {
	finder := find.NewFinder(client.Client, true)

	dc, err := finder.Datacenter(ctx, datacenterName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find datacenter %s: %w", datacenterName, err)
	}
	finder.SetDatacenter(dc)

	logger.Info("found datacenter", zap.String("datacenter", datacenterName))
	return finder, dc, nil
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "vmware"
}

// Version returns the vSphere API version this adapter supports.
func (a *Adapter) Version() string {
	if a.client != nil && a.client.Client != nil {
		return a.client.ServiceContent.About.Version
	}
	return "vsphere-7.0"
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

// Health performs a health check on the vSphere backend.
// It verifies connectivity to vCenter.
func (a *Adapter) Health(ctx context.Context) error {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveHealthCheck("vmware", start, err) }()

	a.Logger.Debug("health check called")

	// Use a timeout to prevent indefinite blocking
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check if we can retrieve the datacenter
	_, err = a.finder.Datacenter(healthCtx, a.datacenterName)
	if err != nil {
		a.Logger.Error("vSphere health check failed", zap.Error(err))
		return fmt.Errorf("vcenter API unreachable: %w", err)
	}

	a.Logger.Debug("health check passed")
	return nil
}

// Close cleanly shuts down the adapter and releases resources.
func (a *Adapter) Close() error {
	a.Logger.Info("closing VMware adapter")

	// Clear subscriptions
	a.SubscriptionsMu.Lock()
	a.Subscriptions = make(map[string]*adapter.Subscription)
	a.SubscriptionsMu.Unlock()

	// Logout from vCenter
	if a.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.client.Logout(ctx); err != nil {
			a.Logger.Warn("failed to logout from vCenter", zap.Error(err))
		}
	}

	// Sync logger before shutdown
	if err := a.Logger.Sync(); err != nil {
		// Ignore sync errors on stderr/stdout
		return nil
	}

	return nil
}

// NOTE: Filter matching and pagination use shared helpers from internal/adapter/helpers.go
// Use adapter.MatchesFilter() and adapter.ApplyPagination() instead of local implementations.

// generateVMProfileID generates a consistent resource type ID for a VM profile.
func GenerateVMProfileID(cpus int32, memoryMB int64) string {
	return fmt.Sprintf("vmware-profile-%dcpu-%dMB", cpus, memoryMB)
}

// generateVMID generates a consistent resource ID for a vSphere VM.
func GenerateVMID(vmName, clusterOrPool string) string {
	return fmt.Sprintf("vmware-vm-%s-%s", clusterOrPool, vmName)
}

// generateClusterPoolID generates a consistent resource pool ID for a Cluster.
func GenerateClusterPoolID(clusterName string) string {
	return fmt.Sprintf("vmware-cluster-%s", clusterName)
}

// generateResourcePoolID generates a consistent resource pool ID for a Resource Pool.
func GenerateResourcePoolID(poolName, clusterName string) string {
	return fmt.Sprintf("vmware-pool-%s-%s", clusterName, poolName)
}
