// Package onap implements the SMO plugin for ONAP (Open Network Automation Platform) integration.
// This plugin provides dual-mode operation:
//   - Northbound Mode (O2-IMS → ONAP): Sync inventory to A&AI, publish events to DMaaP
//   - DMS Backend Mode (ONAP SO → O2-DMS): Execute deployments via ONAP Service Orchestrator
package onap

import (
	"context"
	"fmt"
	"reflect"
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

	// Parse and validate configuration
	cfg, err := p.parseAndValidateConfig(config)
	if err != nil {
		return err
	}

	p.config = cfg
	p.logInitialization(cfg)

	// Initialize all ONAP clients
	if err := p.initializeClients(cfg); err != nil {
		return err
	}

	// Perform initial health check and log status
	p.performHealthCheck(ctx)

	return nil
}

// parseAndValidateConfig parses and validates the plugin configuration.
func (p *Plugin) parseAndValidateConfig(config map[string]interface{}) (*Config, error) {
	cfg := DefaultConfig()
	parseConfig(config, cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// logInitialization logs the plugin initialization details.
func (p *Plugin) logInitialization(cfg *Config) {
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
}

// initializeClients initializes all enabled ONAP clients.
func (p *Plugin) initializeClients(cfg *Config) error {
	// Initialize northbound clients
	if err := p.initializeNorthboundClients(cfg); err != nil {
		return err
	}

	// Initialize DMS backend clients
	if err := p.initializeDMSClients(cfg); err != nil {
		return err
	}

	return nil
}

// initializeNorthboundClients initializes A&AI and DMaaP clients.
func (p *Plugin) initializeNorthboundClients(cfg *Config) error {
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

	return nil
}

// initializeDMSClients initializes SO and SDNC clients for DMS backend.
func (p *Plugin) initializeDMSClients(cfg *Config) error {
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

	return nil
}

// performHealthCheck performs initial health check and logs the resul.
func (p *Plugin) performHealthCheck(ctx context.Context) {
	health := p.Health(ctx)
	if !health.Healthy {
		p.logger.Warn("ONAP plugin initialized but some components are unhealthy",
			zap.String("message", health.Message),
		)
	} else {
		p.logger.Info("ONAP plugin initialized successfully")
	}
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
func (p *Plugin) checkComponentHealth(
	ctx context.Context,
	name string,
	healthFn func(context.Context) error,
) smo.ComponentHealth {
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

	errs := p.closeAllClients()

	p.closed = true

	if len(errs) > 0 {
		return fmt.Errorf("errors closing ONAP plugin: %v", errs)
	}

	p.logger.Info("ONAP plugin closed successfully")
	return nil
}

// closeAllClients closes all ONAP clients and collects any errors.
func (p *Plugin) closeAllClients() []error {
	var errs []error

	errs = p.closeClient(p.aaiClient, "A&AI", errs)
	errs = p.closeClient(p.dmaapClient, "DMaaP", errs)
	errs = p.closeClient(p.soClient, "SO", errs)
	errs = p.closeClient(p.sdncClient, "SDNC", errs)

	return errs
}

// isNilInterface checks if an interface contains a nil value.
// This is necessary because in Go, an interface containing a nil pointer is not nil itself.
func isNilInterface(i interface{}) bool {
	if i == nil {
		return true
	}
	// Use reflection to check if the underlying value is nil
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return false
	}
}

// closeClient closes a single client and appends any error to the error slice.
func (p *Plugin) closeClient(client interface{ Close() error }, name string, errs []error) []error {
	// Check if the interface contains a nil value using reflection
	// This is necessary because a nil pointer wrapped in an interface is not nil
	if client == nil || isNilInterface(client) {
		return errs
	}

	if err := client.Close(); err != nil {
		return append(errs, fmt.Errorf("failed to close %s client: %w", name, err))
	}
	return errs
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if err := c.validateURLs(); err != nil {
		return err
	}
	if err := c.validateAuth(); err != nil {
		return err
	}
	return c.validateTuning()
}

// validateURLs validates URL configuration fields.
func (c *Config) validateURLs() error {
	if c.EnableInventorySync && c.AAIURL == "" {
		return fmt.Errorf("aaiUrl is required when inventory sync is enabled")
	}
	if c.EnableEventPublishing && c.DMaaPURL == "" {
		return fmt.Errorf("dmaapUrl is required when event publishing is enabled")
	}
	if c.EnableDMSBackend && c.SOURL == "" {
		return fmt.Errorf("soUrl is required when DMS backend is enabled")
	}
	return nil
}

// validateAuth validates authentication configuration.
func (c *Config) validateAuth() error {
	if c.Username == "" || c.Password == "" {
		return fmt.Errorf("username and password are required")
	}
	return nil
}

// validateTuning validates tuning parameters.
func (c *Config) validateTuning() error {
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
func parseConfig(input map[string]interface{}, output *Config) {
	parseStringFields(input, output)
	parseTLSFields(input, output)
	parseTimingFields(input, output)
	parseFeatureFlags(input, output)
}

// parseStringFields parses string configuration fields.
func parseStringFields(input map[string]interface{}, output *Config) {
	output.AAIURL = getStringValue(input, "aaiUrl", output.AAIURL)
	output.DMaaPURL = getStringValue(input, "dmaapUrl", output.DMaaPURL)
	output.SOURL = getStringValue(input, "soUrl", output.SOURL)
	output.SDNCURL = getStringValue(input, "sdncUrl", output.SDNCURL)
	output.Username = getStringValue(input, "username", output.Username)
	output.Password = getStringValue(input, "password", output.Password)
}

// parseTLSFields parses TLS configuration fields.
func parseTLSFields(input map[string]interface{}, output *Config) {
	output.TLSEnabled = getBoolValue(input, "tlsEnabled", output.TLSEnabled)
	output.TLSCertFile = getStringValue(input, "tlsCertFile", output.TLSCertFile)
	output.TLSKeyFile = getStringValue(input, "tlsKeyFile", output.TLSKeyFile)
	output.TLSCAFile = getStringValue(input, "tlsCAFile", output.TLSCAFile)
	output.TLSInsecureSkipVerify = getBoolValue(input, "tlsInsecureSkipVerify", output.TLSInsecureSkipVerify)
}

// parseTimingFields parses timing and retry configuration fields.
func parseTimingFields(input map[string]interface{}, output *Config) {
	output.InventorySyncInterval = getDurationValue(input, "inventorySyncInterval", output.InventorySyncInterval)
	output.EventPublishBatchSize = getIntValue(input, "eventPublishBatchSize", output.EventPublishBatchSize)
	output.RequestTimeout = getDurationValue(input, "requestTimeout", output.RequestTimeout)
	output.MaxRetries = getIntValue(input, "maxRetries", output.MaxRetries)
}

// parseFeatureFlags parses feature flag configuration fields.
func parseFeatureFlags(input map[string]interface{}, output *Config) {
	output.EnableInventorySync = getBoolValue(input, "enableInventorySync", output.EnableInventorySync)
	output.EnableEventPublishing = getBoolValue(input, "enableEventPublishing", output.EnableEventPublishing)
	output.EnableDMSBackend = getBoolValue(input, "enableDmsBackend", output.EnableDMSBackend)
	output.EnableSDNC = getBoolValue(input, "enableSdnc", output.EnableSDNC)
}

// getStringValue retrieves a string value from config map with default fallback.
func getStringValue(input map[string]interface{}, key, defaultVal string) string {
	if val, ok := input[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultVal
}

// getBoolValue retrieves a bool value from config map with default fallback.
func getBoolValue(input map[string]interface{}, key string, defaultVal bool) bool {
	if val, ok := input[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultVal
}

// getIntValue retrieves an int value from config map with default fallback.
func getIntValue(input map[string]interface{}, key string, defaultVal int) int {
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

// getDurationValue retrieves a duration value from config map with default fallback.
func getDurationValue(input map[string]interface{}, key string, defaultVal time.Duration) time.Duration {
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
