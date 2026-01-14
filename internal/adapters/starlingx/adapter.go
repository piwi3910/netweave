// Package starlingx provides a StarlingX/Wind River implementation of the O2-IMS adapter interface.
// It translates O2-IMS API operations to StarlingX System Inventory (sysinv) API calls,
// mapping StarlingX hosts to O2-IMS Resources, labels to Resource Pools, and system information
// to Deployment Managers.
package starlingx

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/storage"
)

// Adapter implements the adapter.Adapter interface for StarlingX backends.
// It provides O2-IMS functionality by mapping O2-IMS resources to StarlingX resources:
//   - Resource Pools → StarlingX Labels (hosts grouped by label)
//   - Resources → StarlingX Compute Hosts (ihosts with personality=compute)
//   - Resource Types → StarlingX Host Personalities
//   - Deployment Manager → StarlingX System Information
type Adapter struct {
	client              *Client
	store               storage.Store
	logger              *zap.Logger
	oCloudID            string
	deploymentManagerID string
	region              string
}

// Config holds configuration for creating a StarlingX Adapter.
type Config struct {
	// Endpoint is the StarlingX System Inventory API endpoint.
	// Example: "http://controller:6385"
	Endpoint string

	// KeystoneEndpoint is the Keystone authentication endpoint.
	// Example: "http://controller:5000"
	KeystoneEndpoint string

	// Username for Keystone authentication.
	Username string

	// Password for Keystone authentication.
	Password string

	// ProjectName is the Keystone project/tenant name.
	// Example: "admin"
	ProjectName string

	// DomainName is the Keystone domain name.
	// Example: "Default"
	DomainName string

	// Region is the StarlingX region identifier (optional).
	Region string

	// OCloudID is the identifier of the parent O-Cloud.
	OCloudID string

	// DeploymentManagerID is the identifier for this deployment manager.
	DeploymentManagerID string

	// Store is the subscription storage backend (Redis).
	// Optional: If nil, subscription operations will return not implemented errors.
	Store storage.Store

	// Logger is the logger to use. If nil, a default logger will be created.
	Logger *zap.Logger
}

// New creates a new StarlingX Adapter with the provided configuration.
//
// Example:
//
//	adapter, err := starlingx.New(&starlingx.Config{
//	    Endpoint:            "http://controller:6385",
//	    KeystoneEndpoint:    "http://controller:5000",
//	    Username:            "admin",
//	    Password:            "secret",
//	    ProjectName:         "admin",
//	    DomainName:          "Default",
//	    OCloudID:            "starlingx-ocloud-1",
//	    DeploymentManagerID: "starlingx-dm-1",
//	    Region:              "RegionOne",
//	})
func New(cfg *Config) (*Adapter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}

	if cfg.KeystoneEndpoint == "" {
		return nil, fmt.Errorf("keystone endpoint is required")
	}

	if cfg.Username == "" || cfg.Password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	if cfg.ProjectName == "" {
		cfg.ProjectName = "admin"
	}

	if cfg.DomainName == "" {
		cfg.DomainName = "Default"
	}

	if cfg.OCloudID == "" {
		return nil, fmt.Errorf("oCloudID is required")
	}

	if cfg.DeploymentManagerID == "" {
		return nil, fmt.Errorf("deploymentManagerID is required")
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

	// Create authentication client
	authClient := NewAuthClient(
		cfg.KeystoneEndpoint,
		cfg.Username,
		cfg.Password,
		cfg.ProjectName,
		cfg.DomainName,
		logger,
	)

	// Create API client
	client := NewClient(cfg.Endpoint, authClient, logger)

	adapter := &Adapter{
		client:              client,
		store:               cfg.Store,
		logger:              logger,
		oCloudID:            cfg.OCloudID,
		deploymentManagerID: cfg.DeploymentManagerID,
		region:              cfg.Region,
	}

	logger.Info("starlingx adapter initialized",
		zap.String("endpoint", cfg.Endpoint),
		zap.String("username", cfg.Username),
		zap.String("oCloudID", cfg.OCloudID),
		zap.String("deploymentManagerID", cfg.DeploymentManagerID),
	)

	return adapter, nil
}

// Name returns the unique name of this adapter.
func (a *Adapter) Name() string {
	return "starlingx"
}

// Version returns the version of the backend system this adapter supports.
func (a *Adapter) Version() string {
	return "8.0" // StarlingX 8.0 is the current release
}

// Capabilities returns the list of O2-IMS capabilities this adapter supports.
func (a *Adapter) Capabilities() []adapter.Capability {
	capabilities := []adapter.Capability{
		adapter.CapabilityResourcePools,
		adapter.CapabilityResources,
		adapter.CapabilityResourceTypes,
		adapter.CapabilityDeploymentManagers,
		adapter.CapabilityHealthChecks,
	}

	// Add subscriptions capability if store is available
	if a.store != nil {
		capabilities = append(capabilities, adapter.CapabilitySubscriptions)
	}

	return capabilities
}

// Health performs a health check on the StarlingX backend.
func (a *Adapter) Health(ctx context.Context) error {
	// Try to list systems as a health check
	systems, err := a.client.ListSystems(ctx)
	if err != nil {
		return fmt.Errorf("starlingx health check failed: %w", err)
	}

	if len(systems) == 0 {
		return fmt.Errorf("no starlingx systems found")
	}

	a.logger.Debug("starlingx health check passed",
		zap.Int("system_count", len(systems)),
	)

	return nil
}

// Close cleanly shuts down the adapter and releases resources.
func (a *Adapter) Close() error {
	a.logger.Info("closing starlingx adapter")
	// No persistent connections to close
	return nil
}
