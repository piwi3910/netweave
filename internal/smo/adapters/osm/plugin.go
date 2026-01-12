// Package osm implements the OSM (Open Source MANO) integration plugin for dual-mode operation.
// It provides both northbound integration (netweave → OSM) for VIM synchronization and event
// publishing, and DMS backend mode (OSM → netweave O2-DMS) for NS/VNF lifecycle management.
//
// Architecture:
//   - Northbound Mode: Sync infrastructure inventory to OSM, publish resource events
//   - DMS Backend Mode: Execute NS/VNF deployments via OSM NBI API
//
// OSM NBI (Northbound Interface) API integration:
//   - VIM account management
//   - NS descriptor onboarding (NSD)
//   - VNF descriptor onboarding (VNFD)
//   - NS lifecycle operations (instantiate, terminate, scale, heal)
//   - Status monitoring and queries
package osm

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Plugin implements the OSM integration plugin for dual-mode operation.
// It supports both northbound integration (inventory sync) and DMS backend
// mode (deployment lifecycle management).
type Plugin struct {
	name    string
	version string
	config  *Config
	client  *Client

	// State management
	mu       sync.RWMutex
	running  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	lastSync time.Time

	// Capabilities
	capabilities []string
}

// Config holds the configuration for the OSM plugin.
type Config struct {
	// OSM NBI (Northbound Interface) configuration
	NBIURL   string `yaml:"nbiUrl"`   // OSM NBI API endpoint (e.g., https://osm.example.com:9999)
	Username string `yaml:"username"` // OSM username
	Password string `yaml:"password"` // OSM password
	Project  string `yaml:"project"`  // OSM project/tenant (default: "admin")

	// Timeouts and intervals
	RequestTimeout        time.Duration `yaml:"requestTimeout"`        // HTTP request timeout (default: 30s)
	InventorySyncInterval time.Duration `yaml:"inventorySyncInterval"` // Inventory sync interval (default: 5m)
	LCMPollingInterval    time.Duration `yaml:"lcmPollingInterval"`    // LCM operation polling interval (default: 10s)

	// Retry configuration
	MaxRetries      int           `yaml:"maxRetries"`      // Maximum number of retries (default: 3)
	RetryDelay      time.Duration `yaml:"retryDelay"`      // Initial retry delay (default: 1s)
	RetryMaxDelay   time.Duration `yaml:"retryMaxDelay"`   // Maximum retry delay (default: 30s)
	RetryMultiplier float64       `yaml:"retryMultiplier"` // Retry delay multiplier (default: 2.0)

	// Feature flags
	EnableInventorySync bool `yaml:"enableInventorySync"` // Enable automatic inventory sync (default: true)
	EnableEventPublish  bool `yaml:"enableEventPublish"`  // Enable event publishing to OSM (default: true)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Project:               "admin",
		RequestTimeout:        30 * time.Second,
		InventorySyncInterval: 5 * time.Minute,
		LCMPollingInterval:    10 * time.Second,
		MaxRetries:            3,
		RetryDelay:            1 * time.Second,
		RetryMaxDelay:         30 * time.Second,
		RetryMultiplier:       2.0,
		EnableInventorySync:   true,
		EnableEventPublish:    true,
	}
}

// NewPlugin creates a new OSM plugin instance with the provided configuration.
// It initializes the OSM NBI client and validates the configuration.
func NewPlugin(config *Config) (*Plugin, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Validate required configuration
	if config.NBIURL == "" {
		return nil, fmt.Errorf("nbiUrl is required")
	}
	if config.Username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if config.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	// Set defaults for optional fields
	if config.Project == "" {
		config.Project = "admin"
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.InventorySyncInterval == 0 {
		config.InventorySyncInterval = 5 * time.Minute
	}
	if config.LCMPollingInterval == 0 {
		config.LCMPollingInterval = 10 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second
	}
	if config.RetryMaxDelay == 0 {
		config.RetryMaxDelay = 30 * time.Second
	}
	if config.RetryMultiplier == 0 {
		config.RetryMultiplier = 2.0
	}

	// Create OSM NBI client
	client, err := NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create OSM client: %w", err)
	}

	return &Plugin{
		name:    "osm",
		version: "1.0.0",
		config:  config,
		client:  client,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
		capabilities: []string{
			"inventory-sync",
			"workflow-orchestration",
			"service-modeling", // NS/VNF descriptors
			"package-management",
			"deployment-lifecycle",
			"scaling",
		},
	}, nil
}

// Name returns the unique name of this plugin.
func (p *Plugin) Name() string {
	return p.name
}

// Version returns the version of this plugin.
func (p *Plugin) Version() string {
	return p.version
}

// Capabilities returns the list of capabilities this plugin supports.
func (p *Plugin) Capabilities() []string {
	return p.capabilities
}

// Initialize initializes the plugin and starts background tasks if configured.
// It authenticates with the OSM NBI and optionally starts the inventory sync loop.
func (p *Plugin) Initialize(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("plugin already initialized")
	}

	// Authenticate with OSM NBI
	if err := p.client.Authenticate(ctx); err != nil {
		return fmt.Errorf("failed to authenticate with OSM: %w", err)
	}

	// Start inventory sync loop if enabled
	if p.config.EnableInventorySync {
		// Create a detached context for the long-running background sync loop
		// The loop will create its own child contexts with timeouts for each sync operation
		syncCtx := context.WithoutCancel(ctx)
		go p.inventorySyncLoop(syncCtx)
	}

	p.running = true
	return nil
}

// Health performs a health check on the OSM backend.
// It verifies connectivity to the OSM NBI and authentication status.
func (p *Plugin) Health(ctx context.Context) error {
	// Check if plugin is initialized
	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return fmt.Errorf("plugin not initialized")
	}
	p.mu.RUnlock()

	// Verify OSM NBI connectivity and authentication
	return p.client.Health(ctx)
}

// Close cleanly shuts down the plugin and releases resources.
// It stops background tasks and closes the OSM client connection.
func (p *Plugin) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	// Signal shutdown
	close(p.stopCh)

	// Wait for background tasks to complete (with timeout)
	select {
	case <-p.doneCh:
		// Background tasks completed
	case <-time.After(30 * time.Second):
		// Timeout waiting for shutdown
		return fmt.Errorf("timeout waiting for plugin shutdown")
	}

	// Close OSM client
	if err := p.client.Close(); err != nil {
		return fmt.Errorf("failed to close OSM client: %w", err)
	}

	p.running = false
	return nil
}

// inventorySyncLoop runs periodic inventory synchronization with OSM.
// It syncs VIM accounts and infrastructure resources at the configured interval.
func (p *Plugin) inventorySyncLoop(ctx context.Context) {
	defer close(p.doneCh)

	ticker := time.NewTicker(p.config.InventorySyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			if err := p.syncInventory(syncCtx); err != nil {
				// Log error but continue syncing
				// TODO: Add structured logging when logger is available
				_ = err
			}
			cancel()

			p.mu.Lock()
			p.lastSync = time.Now()
			p.mu.Unlock()
		}
	}
}

// syncInventory performs a full inventory synchronization with OSM.
// This is an internal method called by the sync loop.
// NOTE: Full inventory sync is planned for future release - see GitHub issue #33.
func (p *Plugin) syncInventory(_ context.Context) error {
	// Inventory synchronization steps (future implementation):
	// 1. Fetch current VIM accounts from OSM
	// 2. Compare with local inventory
	// 3. Update/create VIM accounts as needed
	// Tracked in: https://github.com/piwi3910/netweave/issues/33
	return nil
}

// SupportsWorkflows returns whether this plugin supports workflow orchestration.
func (p *Plugin) SupportsWorkflows() bool {
	return true
}

// SupportsServiceModeling returns whether this plugin supports service modeling.
func (p *Plugin) SupportsServiceModeling() bool {
	return true
}

// SupportsPolicyManagement returns whether this plugin supports policy management.
func (p *Plugin) SupportsPolicyManagement() bool {
	return false // OSM doesn't have native policy management
}

// SupportsRollback returns whether this plugin supports deployment rollback.
func (p *Plugin) SupportsRollback() bool {
	return false // OSM doesn't support NS rollback
}

// SupportsScaling returns whether this plugin supports deployment scaling.
func (p *Plugin) SupportsScaling() bool {
	return true
}

// SupportsGitOps returns whether this plugin supports GitOps workflows.
func (p *Plugin) SupportsGitOps() bool {
	return false // OSM doesn't support GitOps
}

// LastSyncTime returns the timestamp of the last successful inventory sync.
func (p *Plugin) LastSyncTime() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastSync
}
