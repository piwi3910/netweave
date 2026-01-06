// Package onap implements the SMO plugin for ONAP (Open Network Automation Platform) integration.
// This plugin provides dual-mode operation:
//   - Northbound Mode (O2-IMS → ONAP): Sync inventory to A&AI, publish events to DMaaP
//   - DMS Backend Mode (ONAP SO → O2-DMS): Execute deployments via ONAP Service Orchestrator
package onap

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/smo"
)

// Plugin implements the SMO Plugin interface for ONAP integration.
// It provides bidirectional integration with ONAP components:
//   - A&AI (Active & Available Inventory) for inventory management
//   - DMaaP (Data Movement as a Platform) for event publishing
//   - SO (Service Orchestrator) for deployment orchestration
//   - SDNC (Software Defined Network Controller) for SDN configuration
type Plugin struct {
	name    string
	version string
	logger  *zap.Logger
	config  *Config
	mu      sync.RWMutex
	closed  bool

	// Northbound clients (netweave → ONAP)
	aaiClient   *AAIClient
	dmaapClient *DMaaPClient

	// DMS backend clients (ONAP → netweave)
	soClient   *SOClient
	sdncClient *SDNCClient
}

// Config defines the configuration for the ONAP plugin.
type Config struct {
	// Northbound Configuration (netweave → ONAP)
	AAIURL   string `mapstructure:"aaiUrl"`
	DMaaPURL string `mapstructure:"dmaapUrl"`

	// DMS Backend Configuration (ONAP → netweave)
	SOURL   string `mapstructure:"soUrl"`
	SDNCURL string `mapstructure:"sdncUrl"`

	// Authentication
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`

	// TLS Configuration
	TLSEnabled            bool   `mapstructure:"tlsEnabled"`
	TLSCertFile           string `mapstructure:"tlsCertFile"`
	TLSKeyFile            string `mapstructure:"tlsKeyFile"`
	TLSCAFile             string `mapstructure:"tlsCAFile"`
	TLSInsecureSkipVerify bool   `mapstructure:"tlsInsecureSkipVerify"`

	// Settings
	InventorySyncInterval time.Duration `mapstructure:"inventorySyncInterval"`
	EventPublishBatchSize int           `mapstructure:"eventPublishBatchSize"`
	RequestTimeout        time.Duration `mapstructure:"requestTimeout"`
	MaxRetries            int           `mapstructure:"maxRetries"`

	// Feature Toggles
	EnableInventorySync   bool `mapstructure:"enableInventorySync"`
	EnableEventPublishing bool `mapstructure:"enableEventPublishing"`
	EnableDMSBackend      bool `mapstructure:"enableDmsBackend"`
	EnableSDNC            bool `mapstructure:"enableSdnc"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		TLSEnabled:            true,
		TLSInsecureSkipVerify: false,
		InventorySyncInterval: 5 * time.Minute,
		EventPublishBatchSize: 100,
		RequestTimeout:        30 * time.Second,
		MaxRetries:            3,
		EnableInventorySync:   true,
		EnableEventPublishing: true,
		EnableDMSBackend:      true,
		EnableSDNC:            false, // Optional component
	}
}

// NewPlugin creates a new ONAP plugin instance with the provided logger.
// The plugin is not initialized until Initialize() is called.
func NewPlugin(logger *zap.Logger) *Plugin {
	return &Plugin{
		name:    "onap",
		version: "1.0.0",
		logger:  logger,
	}
}

// Metadata returns the plugin's identifying information.
func (p *Plugin) Metadata() smo.PluginMetadata {
	return smo.PluginMetadata{
		Name:        p.name,
		Version:     p.version,
		Description: "ONAP (Open Network Automation Platform) integration plugin with dual-mode operation",
		Vendor:      "Linux Foundation / ONAP Community",
	}
}

// Capabilities returns the list of features this plugin supports.
func (p *Plugin) Capabilities() []smo.Capability {
	caps := []smo.Capability{}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.config == nil {
		return caps
	}

	if p.config.EnableInventorySync {
		caps = append(caps, smo.CapInventorySync)
	}

	if p.config.EnableEventPublishing {
		caps = append(caps, smo.CapEventPublishing)
	}

	if p.config.EnableDMSBackend {
		caps = append(caps,
			smo.CapWorkflowOrchestration,
			smo.CapServiceModeling,
		)
	}

	// Policy management is always supported through ONAP Policy Framework
	caps = append(caps, smo.CapPolicyManagement)

	return caps
}

// Initialize initializes the plugin with the provided configuration.
// It validates the configuration and establishes connections to ONAP components.
func (p *Plugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("plugin is closed")
	}

	// Parse configuration
	cfg := DefaultConfig()
	if err := parseConfig(config, cfg); err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	p.config = cfg
	p.logger.Info("Initializing ONAP plugin",
		zap.String("aaiUrl", cfg.AAIURL),
		zap.String("dmaapUrl", cfg.DMaaPURL),
		zap.String("soUrl", cfg.SOURL),
		zap.String("sdncUrl", cfg.SDNCURL),
		zap.Bool("enableInventorySync", cfg.EnableInventorySync),
		zap.Bool("enableEventPublishing", cfg.EnableEventPublishing),
		zap.Bool("enableDmsBackend", cfg.EnableDMSBackend),
		zap.Bool("enableSdnc", cfg.EnableSDNC),
	)

	// Initialize northbound clients
	if cfg.EnableInventorySync && cfg.AAIURL != "" {
		p.logger.Info("Initializing A&AI client", zap.String("url", cfg.AAIURL))
		aaiClient, err := NewAAIClient(cfg, p.logger)
		if err != nil {
			return fmt.Errorf("failed to create A&AI client: %w", err)
		}
		p.aaiClient = aaiClient
	}

	if cfg.EnableEventPublishing && cfg.DMaaPURL != "" {
		p.logger.Info("Initializing DMaaP client", zap.String("url", cfg.DMaaPURL))
		dmaapClient, err := NewDMaaPClient(cfg, p.logger)
		if err != nil {
			return fmt.Errorf("failed to create DMaaP client: %w", err)
		}
		p.dmaapClient = dmaapClient
	}

	// Initialize DMS backend clients
	if cfg.EnableDMSBackend && cfg.SOURL != "" {
		p.logger.Info("Initializing SO client", zap.String("url", cfg.SOURL))
		soClient, err := NewSOClient(cfg, p.logger)
		if err != nil {
			return fmt.Errorf("failed to create SO client: %w", err)
		}
		p.soClient = soClient
	}

	if cfg.EnableSDNC && cfg.SDNCURL != "" {
		p.logger.Info("Initializing SDNC client", zap.String("url", cfg.SDNCURL))
		sdncClient, err := NewSDNCClient(cfg, p.logger)
		if err != nil {
			return fmt.Errorf("failed to create SDNC client: %w", err)
		}
		p.sdncClient = sdncClient
	}

	// Perform initial health check
	health := p.Health(ctx)
	if !health.Healthy {
		p.logger.Warn("ONAP plugin initialized but some components are unhealthy",
			zap.String("message", health.Message),
		)
	} else {
		p.logger.Info("ONAP plugin initialized successfully")
	}

	return nil
}

// Health checks the health status of all ONAP component connections.
func (p *Plugin) Health(ctx context.Context) smo.HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return smo.HealthStatus{
			Healthy:   false,
			Message:   "plugin is closed",
			Timestamp: time.Now(),
		}
	}

	status := smo.HealthStatus{
		Healthy:   true,
		Details:   make(map[string]smo.ComponentHealth),
		Timestamp: time.Now(),
	}

	// Check A&AI health
	if p.aaiClient != nil {
		aaiHealth := p.checkComponentHealth(ctx, "aai", p.aaiClient.Health)
		status.Details["aai"] = aaiHealth
		if !aaiHealth.Healthy {
			status.Healthy = false
		}
	}

	// Check DMaaP health
	if p.dmaapClient != nil {
		dmaapHealth := p.checkComponentHealth(ctx, "dmaap", p.dmaapClient.Health)
		status.Details["dmaap"] = dmaapHealth
		if !dmaapHealth.Healthy {
			status.Healthy = false
		}
	}

	// Check SO health
	if p.soClient != nil {
		soHealth := p.checkComponentHealth(ctx, "so", p.soClient.Health)
		status.Details["so"] = soHealth
		if !soHealth.Healthy {
			status.Healthy = false
		}
	}

	// Check SDNC health (optional)
	if p.sdncClient != nil {
		sdncHealth := p.checkComponentHealth(ctx, "sdnc", p.sdncClient.Health)
		status.Details["sdnc"] = sdncHealth
		// SDNC is optional, don't mark overall as unhealthy if it's down
	}

	if status.Healthy {
		status.Message = "all ONAP components are healthy"
	} else {
		status.Message = "one or more ONAP components are unhealthy"
	}

	return status
}

// checkComponentHealth is a helper to check individual component health with timeout.
func (p *Plugin) checkComponentHealth(ctx context.Context, name string, healthFn func(context.Context) error) smo.ComponentHealth {
	start := time.Now()

	// Create context with timeout for health check
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := healthFn(checkCtx)
	responseTime := time.Since(start)

	health := smo.ComponentHealth{
		Name:         name,
		ResponseTime: responseTime,
	}

	if err != nil {
		health.Healthy = false
		health.Message = fmt.Sprintf("health check failed: %v", err)
		p.logger.Warn("ONAP component unhealthy",
			zap.String("component", name),
			zap.Error(err),
			zap.Duration("responseTime", responseTime),
		)
	} else {
		health.Healthy = true
		health.Message = "healthy"
		p.logger.Debug("ONAP component healthy",
			zap.String("component", name),
			zap.Duration("responseTime", responseTime),
		)
	}

	return health
}

// Close cleanly shuts down the plugin and releases all resources.
func (p *Plugin) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.logger.Info("Closing ONAP plugin")

	var errs []error

	// Close all clients
	if p.aaiClient != nil {
		if err := p.aaiClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close A&AI client: %w", err))
		}
	}

	if p.dmaapClient != nil {
		if err := p.dmaapClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close DMaaP client: %w", err))
		}
	}

	if p.soClient != nil {
		if err := p.soClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close SO client: %w", err))
		}
	}

	if p.sdncClient != nil {
		if err := p.sdncClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close SDNC client: %w", err))
		}
	}

	p.closed = true

	if len(errs) > 0 {
		return fmt.Errorf("errors closing ONAP plugin: %v", errs)
	}

	p.logger.Info("ONAP plugin closed successfully")
	return nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.EnableInventorySync && c.AAIURL == "" {
		return fmt.Errorf("aaiUrl is required when inventory sync is enabled")
	}

	if c.EnableEventPublishing && c.DMaaPURL == "" {
		return fmt.Errorf("dmaapUrl is required when event publishing is enabled")
	}

	if c.EnableDMSBackend && c.SOURL == "" {
		return fmt.Errorf("soUrl is required when DMS backend is enabled")
	}

	if c.Username == "" || c.Password == "" {
		return fmt.Errorf("username and password are required")
	}

	if c.RequestTimeout <= 0 {
		return fmt.Errorf("requestTimeout must be positive")
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("maxRetries cannot be negative")
	}

	if c.EventPublishBatchSize <= 0 {
		return fmt.Errorf("eventPublishBatchSize must be positive")
	}

	return nil
}

// parseConfig parses a map[string]interface{} into a Config struct.
func parseConfig(input map[string]interface{}, output *Config) error {
	// Helper function to get string values
	getString := func(key string, defaultVal string) string {
		if val, ok := input[key]; ok {
			if str, ok := val.(string); ok {
				return str
			}
		}
		return defaultVal
	}

	// Helper function to get bool values
	getBool := func(key string, defaultVal bool) bool {
		if val, ok := input[key]; ok {
			if b, ok := val.(bool); ok {
				return b
			}
		}
		return defaultVal
	}

	// Helper function to get int values
	getInt := func(key string, defaultVal int) int {
		if val, ok := input[key]; ok {
			switch v := val.(type) {
			case int:
				return v
			case int64:
				return int(v)
			case float64:
				return int(v)
			}
		}
		return defaultVal
	}

	// Helper function to get duration values
	getDuration := func(key string, defaultVal time.Duration) time.Duration {
		if val, ok := input[key]; ok {
			switch v := val.(type) {
			case string:
				if d, err := time.ParseDuration(v); err == nil {
					return d
				}
			case time.Duration:
				return v
			}
		}
		return defaultVal
	}

	// Parse all configuration fields
	output.AAIURL = getString("aaiUrl", output.AAIURL)
	output.DMaaPURL = getString("dmaapUrl", output.DMaaPURL)
	output.SOURL = getString("soUrl", output.SOURL)
	output.SDNCURL = getString("sdncUrl", output.SDNCURL)
	output.Username = getString("username", output.Username)
	output.Password = getString("password", output.Password)

	output.TLSEnabled = getBool("tlsEnabled", output.TLSEnabled)
	output.TLSCertFile = getString("tlsCertFile", output.TLSCertFile)
	output.TLSKeyFile = getString("tlsKeyFile", output.TLSKeyFile)
	output.TLSCAFile = getString("tlsCAFile", output.TLSCAFile)
	output.TLSInsecureSkipVerify = getBool("tlsInsecureSkipVerify", output.TLSInsecureSkipVerify)

	output.InventorySyncInterval = getDuration("inventorySyncInterval", output.InventorySyncInterval)
	output.EventPublishBatchSize = getInt("eventPublishBatchSize", output.EventPublishBatchSize)
	output.RequestTimeout = getDuration("requestTimeout", output.RequestTimeout)
	output.MaxRetries = getInt("maxRetries", output.MaxRetries)

	output.EnableInventorySync = getBool("enableInventorySync", output.EnableInventorySync)
	output.EnableEventPublishing = getBool("enableEventPublishing", output.EnableEventPublishing)
	output.EnableDMSBackend = getBool("enableDmsBackend", output.EnableDMSBackend)
	output.EnableSDNC = getBool("enableSdnc", output.EnableSDNC)

	return nil
}
