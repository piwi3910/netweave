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
	"github.com/vmware/govmomi/session/cache"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"go.uber.org/zap"
)

// VMwareAdapter implements the adapter.Adapter interface for VMware vSphere backends.
// It provides O2-IMS functionality by mapping O2-IMS resources to vSphere resources:
//   - Resource Pools → vSphere Clusters or Resource Pools
//   - Resources → Virtual Machines
//   - Resource Types → VM hardware profiles
//   - Deployment Manager → vCenter/Datacenter metadata
//   - Subscriptions → Polling-based (vSphere events as fallback)
type VMwareAdapter struct {
	// client is the vSphere API client.
	client *govmomi.Client

	// finder is used to locate vSphere objects.
	finder *find.Finder

	// datacenter is the vSphere datacenter.
	datacenter *object.Datacenter

	// logger provides structured logging.
	logger *zap.Logger

	// oCloudID is the identifier of the parent O-Cloud.
	oCloudID string

	// deploymentManagerID is the identifier for this deployment manager.
	deploymentManagerID string

	// vcenterURL is the vCenter server URL.
	vcenterURL string

	// datacenterName is the vSphere datacenter name.
	datacenterName string

	// subscriptions holds active O2-IMS subscriptions (polling-based fallback).
	subscriptions map[string]*adapter.Subscription

	// subscriptionsMu protects the subscriptions map.
	subscriptionsMu sync.RWMutex

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
func New(cfg *Config) (*VMwareAdapter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Validate required configuration
	if cfg.VCenterURL == "" {
		return nil, fmt.Errorf("vCenterURL is required")
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("password is required")
	}
	if cfg.Datacenter == "" {
		return nil, fmt.Errorf("datacenter is required")
	}
	if cfg.OCloudID == "" {
		return nil, fmt.Errorf("oCloudID is required")
	}

	// Set defaults
	deploymentManagerID := cfg.DeploymentManagerID
	if deploymentManagerID == "" {
		deploymentManagerID = fmt.Sprintf("ocloud-vmware-%s", cfg.Datacenter)
	}

	poolMode := cfg.PoolMode
	if poolMode == "" {
		poolMode = "cluster"
	}
	if poolMode != "cluster" && poolMode != "pool" {
		return nil, fmt.Errorf("poolMode must be 'cluster' or 'pool', got %q", poolMode)
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Initialize logger
	logger := cfg.Logger
	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("failed to create logger: %w", err)
		}
	}

	logger.Info("initializing VMware adapter",
		zap.String("vCenterURL", cfg.VCenterURL),
		zap.String("datacenter", cfg.Datacenter),
		zap.String("oCloudID", cfg.OCloudID),
		zap.String("poolMode", poolMode))

	// Parse vCenter URL
	u, err := soap.ParseURL(cfg.VCenterURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vCenter URL: %w", err)
	}
	u.User = url.UserPassword(cfg.Username, cfg.Password)

	// Create session
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create client with session caching
	s := &cache.Session{
		URL:      u,
		Insecure: cfg.InsecureSkipVerify,
	}

	c := new(vim25.Client)
	err = s.Login(ctx, c, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to login to vCenter: %w", err)
	}

	client := &govmomi.Client{
		Client:         c,
		SessionManager: s.Manager(),
	}

	logger.Info("connected to vCenter",
		zap.String("vCenterURL", cfg.VCenterURL))

	// Create finder and set datacenter
	finder := find.NewFinder(client.Client, true)

	dc, err := finder.Datacenter(ctx, cfg.Datacenter)
	if err != nil {
		client.Logout(ctx)
		return nil, fmt.Errorf("failed to find datacenter %s: %w", cfg.Datacenter, err)
	}
	finder.SetDatacenter(dc)

	logger.Info("found datacenter",
		zap.String("datacenter", cfg.Datacenter))

	adp := &VMwareAdapter{
		client:              client,
		finder:              finder,
		datacenter:          dc,
		logger:              logger,
		oCloudID:            cfg.OCloudID,
		deploymentManagerID: deploymentManagerID,
		vcenterURL:          cfg.VCenterURL,
		datacenterName:      cfg.Datacenter,
		subscriptions:       make(map[string]*adapter.Subscription),
		poolMode:            poolMode,
	}

	logger.Info("VMware adapter initialized successfully",
		zap.String("oCloudID", cfg.OCloudID),
		zap.String("deploymentManagerID", deploymentManagerID),
		zap.String("datacenter", cfg.Datacenter),
		zap.String("poolMode", poolMode))

	return adp, nil
}

// Name returns the adapter name.
func (a *VMwareAdapter) Name() string {
	return "vmware"
}

// Version returns the vSphere API version this adapter supports.
func (a *VMwareAdapter) Version() string {
	if a.client != nil && a.client.Client != nil {
		return a.client.Client.ServiceContent.About.Version
	}
	return "vsphere-7.0"
}

// Capabilities returns the list of O2-IMS capabilities supported by this adapter.
func (a *VMwareAdapter) Capabilities() []adapter.Capability {
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
func (a *VMwareAdapter) Health(ctx context.Context) error {
	a.logger.Debug("health check called")

	// Check if we can retrieve the datacenter
	_, err := a.finder.Datacenter(ctx, a.datacenterName)
	if err != nil {
		a.logger.Error("vSphere health check failed", zap.Error(err))
		return fmt.Errorf("vCenter API unreachable: %w", err)
	}

	a.logger.Debug("health check passed")
	return nil
}

// Close cleanly shuts down the adapter and releases resources.
func (a *VMwareAdapter) Close() error {
	a.logger.Info("closing VMware adapter")

	// Clear subscriptions
	a.subscriptionsMu.Lock()
	a.subscriptions = make(map[string]*adapter.Subscription)
	a.subscriptionsMu.Unlock()

	// Logout from vCenter
	if a.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.client.Logout(ctx); err != nil {
			a.logger.Warn("failed to logout from vCenter", zap.Error(err))
		}
	}

	// Sync logger before shutdown
	if err := a.logger.Sync(); err != nil {
		// Ignore sync errors on stderr/stdout
		return nil
	}

	return nil
}

// matchesFilter checks if a resource matches the provided filter criteria.
func (a *VMwareAdapter) matchesFilter(filter *adapter.Filter, resourcePoolID, resourceTypeID, location string, labels map[string]string) bool {
	if filter == nil {
		return true
	}

	// Check ResourcePoolID filter
	if filter.ResourcePoolID != "" && filter.ResourcePoolID != resourcePoolID {
		return false
	}

	// Check ResourceTypeID filter
	if filter.ResourceTypeID != "" && filter.ResourceTypeID != resourceTypeID {
		return false
	}

	// Check Location filter
	if filter.Location != "" && filter.Location != location {
		return false
	}

	// Check Labels filter
	if len(filter.Labels) > 0 {
		for key, value := range filter.Labels {
			if labels[key] != value {
				return false
			}
		}
	}

	return true
}

// applyPagination applies limit and offset to a slice of results.
func applyPagination[T any](items []T, limit, offset int) []T {
	if offset >= len(items) {
		return []T{}
	}

	start := offset
	end := len(items)

	if limit > 0 && start+limit < end {
		end = start + limit
	}

	return items[start:end]
}

// generateVMProfileID generates a consistent resource type ID for a VM profile.
func generateVMProfileID(cpus int32, memoryMB int64) string {
	return fmt.Sprintf("vmware-profile-%dcpu-%dMB", cpus, memoryMB)
}

// generateVMID generates a consistent resource ID for a vSphere VM.
func generateVMID(vmName, clusterOrPool string) string {
	return fmt.Sprintf("vmware-vm-%s-%s", clusterOrPool, vmName)
}

// generateClusterPoolID generates a consistent resource pool ID for a Cluster.
func generateClusterPoolID(clusterName string) string {
	return fmt.Sprintf("vmware-cluster-%s", clusterName)
}

// generateResourcePoolID generates a consistent resource pool ID for a Resource Pool.
func generateResourcePoolID(poolName, clusterName string) string {
	return fmt.Sprintf("vmware-pool-%s-%s", clusterName, poolName)
}
