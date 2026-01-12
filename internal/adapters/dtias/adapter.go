// Package dtias provides a Dell DTIAS bare-metal implementation of the O2-IMS adapter interface.
// It translates O2-IMS API operations to Dell DTIAS REST API calls, mapping O2-IMS resources
// to DTIAS bare-metal infrastructure components.
//
// Resource Mapping:
//   - Resource Pools → DTIAS Server Pools / Resource Groups
//   - Resources → Physical Servers
//   - Resource Types → Server Types (CPU, RAM, storage configurations)
//   - Deployment Manager → DTIAS Datacenter metadata
package dtias

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// Adapter implements the adapter.Adapter interface for Dell DTIAS bare-metal backends.
// It provides O2-IMS functionality by mapping O2-IMS resources to DTIAS API resources:
//   - Resource Pools → DTIAS Server Pools
//   - Resources → Physical Servers with full hardware inventory
//   - Resource Types → Server Profiles (compute/storage/network configurations)
//   - Subscriptions → Polling-based change detection (DTIAS has no native events)
type Adapter struct {
	// client is the DTIAS REST API client
	client *Client

	// logger provides structured logging
	logger *zap.Logger

	// config holds the adapter configuration
	config *Config

	// oCloudID is the identifier of the parent O-Cloud
	oCloudID string

	// deploymentManagerID is the identifier for this deployment manager
	deploymentManagerID string

	// subscriptions holds active subscriptions for polling-based change detection.
	// Since DTIAS has no native event system, subscriptions are stored locally
	// and the gateway layer implements polling to detect changes.
	subscriptions map[string]*adapter.Subscription

	// subscriptionsMu protects the subscriptions map.
	subscriptionsMu sync.RWMutex
}

// Config holds configuration for creating a DTIASAdapter.
type Config struct {
	// Endpoint is the DTIAS API endpoint URL (e.g., "https://dtias.dell.com/api/v1")
	Endpoint string `yaml:"endpoint"`

	// APIKey is the authentication API key for DTIAS
	APIKey string `yaml:"apiKey"`

	// ClientCert is the path to the client certificate for mTLS (optional)
	ClientCert string `yaml:"clientCert"`

	// ClientKey is the path to the client key for mTLS (optional)
	ClientKey string `yaml:"clientKey"`

	// CACert is the path to the CA certificate for server verification (optional)
	// If not provided, system root CAs are used for certificate validation
	CACert string `yaml:"caCert"`

	// Timeout is the HTTP client timeout
	Timeout time.Duration `yaml:"timeout"`

	// OCloudID is the identifier of the parent O-Cloud
	OCloudID string `yaml:"ocloudId"`

	// DeploymentManagerID is the identifier for this deployment manager
	DeploymentManagerID string `yaml:"deploymentManagerId"`

	// Datacenter is the DTIAS datacenter identifier
	Datacenter string `yaml:"datacenter"`

	// RetryAttempts is the number of retry attempts for failed API calls
	RetryAttempts int `yaml:"retryAttempts"`

	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration `yaml:"retryDelay"`

	// Logger is the logger to use. If nil, a default logger will be created.
	Logger *zap.Logger `yaml:"-"`
}

// New creates a new DTIASAdapter with the provided configuration.
// It initializes the DTIAS REST API client with authentication and TLS settings.
//
// Example:
//
//	adapter, err := dtias.New(&dtias.Config{
//	    Endpoint:            "https://dtias.example.com/api/v1",
//	    APIKey:              "your-api-key",
//	    ClientCert:          "/path/to/cert.pem",
//	    ClientKey:           "/path/to/key.pem",
//	    CACert:              "/path/to/ca.pem",
//	    Timeout:             30 * time.Second,
//	    OCloudID:            "ocloud-dtias-1",
//	    DeploymentManagerID: "ocloud-dtias-edge-1",
//	    Datacenter:          "dc-dallas-1",
//	    RetryAttempts:       3,
//	    RetryDelay:          2 * time.Second,
//	})
func New(cfg *Config) (*Adapter, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	setConfigDefaults(cfg)

	logger, err := getLogger(cfg)
	if err != nil {
		return nil, err
	}

	client, err := createDTIASClient(cfg, logger)
	if err != nil {
		return nil, err
	}

	adp := &Adapter{
		client:              client,
		logger:              logger,
		config:              cfg,
		oCloudID:            cfg.OCloudID,
		deploymentManagerID: cfg.DeploymentManagerID,
		subscriptions:       make(map[string]*adapter.Subscription),
	}

	logger.Info("DTIAS adapter initialized",
		zap.String("endpoint", cfg.Endpoint),
		zap.String("oCloudId", cfg.OCloudID),
		zap.String("deploymentManagerId", cfg.DeploymentManagerID),
		zap.String("datacenter", cfg.Datacenter))

	return adp, nil
}

// validateConfig validates the required configuration fields.
func validateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}
	if cfg.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("apiKey is required")
	}
	if cfg.OCloudID == "" {
		return fmt.Errorf("ocloudId is required")
	}
	return nil
}

// setConfigDefaults sets default values for optional configuration fields.
func setConfigDefaults(cfg *Config) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.RetryAttempts == 0 {
		cfg.RetryAttempts = 3
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = 2 * time.Second
	}
	if cfg.DeploymentManagerID == "" {
		cfg.DeploymentManagerID = fmt.Sprintf("%s-dtias-dm", cfg.OCloudID)
	}
}

// getLogger returns the logger from config or creates a new production logger.
func getLogger(cfg *Config) (*zap.Logger, error) {
	if cfg.Logger != nil {
		return cfg.Logger, nil
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	return logger, nil
}

// createDTIASClient creates a new DTIAS client with the provided configuration.
func createDTIASClient(cfg *Config, logger *zap.Logger) (*Client, error) {
	client, err := NewClient(&ClientConfig{
		Endpoint:      cfg.Endpoint,
		APIKey:        cfg.APIKey,
		ClientCert:    cfg.ClientCert,
		ClientKey:     cfg.ClientKey,
		CACert:        cfg.CACert,
		Timeout:       cfg.Timeout,
		RetryAttempts: cfg.RetryAttempts,
		RetryDelay:    cfg.RetryDelay,
		Logger:        logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create DTIAS client: %w", err)
	}
	return client, nil
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "dtias"
}

// Version returns the DTIAS API version this adapter supports.
func (a *Adapter) Version() string {
	return "1.0.0"
}

// Capabilities returns the list of O2-IMS capabilities supported by this adapter.
// DTIAS supports resource management with polling-based subscriptions.
// Note: DTIAS has no native event system, so subscriptions are stored locally
// and the gateway layer implements polling to detect changes and send notifications.
func (a *Adapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityResourcePools,
		adapter.CapabilityResources,
		adapter.CapabilityResourceTypes,
		adapter.CapabilityDeploymentManagers,
		adapter.CapabilitySubscriptions, // Polling-based implementation
		adapter.CapabilityHealthChecks,
	}
}

// Health performs a health check on the DTIAS backend.
// It verifies connectivity and authentication to the DTIAS API.
func (a *Adapter) Health(ctx context.Context) error {
	a.logger.Debug("health check called")

	// Perform health check by querying DTIAS API status endpoint
	if err := a.client.HealthCheck(ctx); err != nil {
		a.logger.Error("health check failed",
			zap.Error(err))
		return fmt.Errorf("DTIAS API unreachable: %w", err)
	}

	a.logger.Debug("health check passed")
	return nil
}

// Close cleanly shuts down the adapter and releases resources.
func (a *Adapter) Close() error {
	a.logger.Info("closing DTIAS adapter")

	// Clear subscriptions
	a.subscriptionsMu.Lock()
	a.subscriptions = make(map[string]*adapter.Subscription)
	a.subscriptionsMu.Unlock()

	// Close client connections
	if err := a.client.Close(); err != nil {
		return fmt.Errorf("failed to close DTIAS client: %w", err)
	}

	// Sync logger before shutdown
	// Ignore sync errors on stderr/stdout which are common
	_ = a.logger.Sync()

	return nil
}
