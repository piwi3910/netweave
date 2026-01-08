// Package openstack provides an OpenStack-native implementation of the O2-IMS adapter interface.
// It translates O2-IMS API operations to OpenStack API calls, mapping O2-IMS resources
// to OpenStack resources like Host Aggregates, Nova Instances, and Flavors.
package openstack

import (
	"context"
	"fmt"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/regions"
	"github.com/gophercloud/gophercloud/openstack/placement/v1/resourceproviders"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// OpenStackAdapter implements the adapter.Adapter interface for OpenStack NFVi backends.
// It provides O2-IMS functionality by mapping O2-IMS resources to OpenStack resources:
//   - Resource Pools → Host Aggregates (Nova placement)
//   - Resources → Nova Instances (compute VMs)
//   - Resource Types → Flavors
//   - Deployment Manager → OpenStack Region metadata
//   - Subscriptions → Polling-based (no native OpenStack subscriptions)
type OpenStackAdapter struct {
	// provider is the authenticated OpenStack provider client.
	provider *gophercloud.ProviderClient

	// compute is the Nova compute service client.
	compute *gophercloud.ServiceClient

	// placement is the Placement service client.
	placement *gophercloud.ServiceClient

	// identity is the Keystone identity service client.
	identity *gophercloud.ServiceClient

	// logger provides structured logging.
	logger *zap.Logger

	// oCloudID is the identifier of the parent O-Cloud.
	oCloudID string

	// deploymentManagerID is the identifier for this deployment manager.
	deploymentManagerID string

	// region is the OpenStack region this adapter manages.
	region string

	// projectName is the OpenStack project (tenant) name.
	projectName string

	// subscriptions holds active subscriptions (polling-based).
	subscriptions map[string]*adapter.Subscription

	// pollingStates tracks the polling state for each active subscription.
	pollingStates map[string]*subscriptionState
}

// Config holds configuration for creating an OpenStackAdapter.
type Config struct {
	// AuthURL is the Keystone authentication endpoint.
	// Example: "https://openstack.example.com:5000/v3"
	AuthURL string

	// Username is the OpenStack username for authentication.
	Username string

	// Password is the OpenStack password for authentication.
	Password string

	// ProjectName is the OpenStack project (tenant) name.
	ProjectName string

	// DomainName is the OpenStack domain name (default: "Default").
	DomainName string

	// Region is the OpenStack region to manage.
	// Example: "RegionOne"
	Region string

	// OCloudID is the identifier of the parent O-Cloud.
	OCloudID string

	// DeploymentManagerID is the identifier for this deployment manager.
	// If empty, defaults to "ocloud-openstack-{region}".
	DeploymentManagerID string

	// Logger is the logger to use. If nil, a default logger will be created.
	Logger *zap.Logger

	// Timeout is the timeout for OpenStack API calls.
	// Defaults to 30 seconds if not specified.
	Timeout time.Duration
}

// New creates a new OpenStackAdapter with the provided configuration.
// It authenticates with OpenStack and initializes service clients for Nova,
// Placement, and Keystone.
//
// Example:
//
//	adapter, err := openstack.New(&openstack.Config{
//	    AuthURL:     "https://openstack.example.com:5000/v3",
//	    Username:    "admin",
//	    Password:    os.Getenv("OPENSTACK_PASSWORD"),
//	    ProjectName: "o2ims",
//	    DomainName:  "Default",
//	    Region:      "RegionOne",
//	    OCloudID:    "ocloud-openstack-1",
//	})
func New(cfg *Config) (*OpenStackAdapter, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	// Apply defaults to configuration
	domainName, deploymentManagerID, timeout, logger := applyDefaults(cfg)

	// Authenticate with OpenStack
	provider, err := authenticateOpenStack(cfg, domainName, timeout, logger)
	if err != nil {
		return nil, err
	}

	// Initialize OpenStack service clients
	clients, err := initializeServiceClients(provider, cfg.Region, logger)
	if err != nil {
		return nil, err
	}

	adapter := &OpenStackAdapter{
		provider:            provider,
		compute:             clients.compute,
		placement:           clients.placement,
		identity:            clients.identity,
		logger:              logger,
		oCloudID:            cfg.OCloudID,
		deploymentManagerID: deploymentManagerID,
		region:              cfg.Region,
		projectName:         cfg.ProjectName,
		subscriptions:       make(map[string]*adapter.Subscription),
	}

	logger.Info("OpenStack adapter initialized successfully",
		zap.String("oCloudID", cfg.OCloudID),
		zap.String("deploymentManagerID", deploymentManagerID),
		zap.String("region", cfg.Region))

	return adapter, nil
}

// validateConfig validates required configuration fields.
func validateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	requiredFields := map[string]string{
		"authURL":     cfg.AuthURL,
		"username":    cfg.Username,
		"password":    cfg.Password,
		"projectName": cfg.ProjectName,
		"region":      cfg.Region,
		"oCloudID":    cfg.OCloudID,
	}

	for field, value := range requiredFields {
		if value == "" {
			return fmt.Errorf("%s is required", field)
		}
	}

	return nil
}

// applyDefaults applies default values to optional configuration fields.
func applyDefaults(cfg *Config) (string, string, time.Duration, *zap.Logger) {
	domainName := cfg.DomainName
	if domainName == "" {
		domainName = "Default"
	}

	deploymentManagerID := cfg.DeploymentManagerID
	if deploymentManagerID == "" {
		deploymentManagerID = fmt.Sprintf("ocloud-openstack-%s", cfg.Region)
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	logger := cfg.Logger
	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			// Return a no-op logger as fallback
			logger = zap.NewNop()
		}
	}

	return domainName, deploymentManagerID, timeout, logger
}

// authenticateOpenStack authenticates with OpenStack and returns a provider clien.
func authenticateOpenStack(
	cfg *Config,
	domainName string,
	timeout time.Duration,
	logger *zap.Logger,
) (*gophercloud.ProviderClient, error) {
	logger.Info("initializing OpenStack adapter",
		zap.String("authURL", cfg.AuthURL),
		zap.String("username", cfg.Username),
		zap.String("projectName", cfg.ProjectName),
		zap.String("domainName", domainName),
		zap.String("region", cfg.Region),
		zap.String("oCloudID", cfg.OCloudID))

	authOpts := gophercloud.AuthOptions{
		IdentityEndpoint: cfg.AuthURL,
		Username:         cfg.Username,
		Password:         cfg.Password,
		TenantName:       cfg.ProjectName,
		DomainName:       domainName,
		AllowReauth:      true,
	}

	provider, err := openstack.AuthenticatedClient(authOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with OpenStack: %w", err)
	}

	provider.HTTPClient.Timeout = timeout

	logger.Info("authenticated with OpenStack",
		zap.String("projectName", cfg.ProjectName))

	return provider, nil
}

// serviceClients holds OpenStack service clients.
type serviceClients struct {
	compute   *gophercloud.ServiceClient
	placement *gophercloud.ServiceClient
	identity  *gophercloud.ServiceClient
}

// initializeServiceClients initializes all required OpenStack service clients.
func initializeServiceClients(
	provider *gophercloud.ProviderClient,
	region string,
	logger *zap.Logger,
) (*serviceClients, error) {
	endpointOpts := gophercloud.EndpointOpts{Region: region}

	computeClient, err := openstack.NewComputeV2(provider, endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nova compute client: %w", err)
	}
	logger.Info("initialized Nova compute client", zap.String("region", region))

	placementClient, err := openstack.NewPlacementV1(provider, endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create Placement client: %w", err)
	}
	logger.Info("initialized Placement client", zap.String("region", region))

	identityClient, err := openstack.NewIdentityV3(provider, endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create Keystone identity client: %w", err)
	}
	logger.Info("initialized Keystone identity client", zap.String("region", region))

	return &serviceClients{
		compute:   computeClient,
		placement: placementClient,
		identity:  identityClient,
	}, nil
}

// Name returns the adapter name.
func (a *OpenStackAdapter) Name() string {
	return "openstack"
}

// Version returns the OpenStack API version this adapter supports.
func (a *OpenStackAdapter) Version() string {
	// OpenStack API versions: Nova v2.1, Placement v1.0, Keystone v3
	return "nova-v2.1"
}

// Capabilities returns the list of O2-IMS capabilities supported by this adapter.
// Note: OpenStack does not support native subscriptions, so we use polling-based subscriptions.
func (a *OpenStackAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityResourcePools,
		adapter.CapabilityResources,
		adapter.CapabilityResourceTypes,
		adapter.CapabilityDeploymentManagers,
		adapter.CapabilitySubscriptions, // Polling-based
		adapter.CapabilityHealthChecks,
	}
}

// GetDeploymentManager retrieves metadata about the OpenStack deployment manager.
// It queries the Keystone region information to construct the deployment manager metadata.
func (a *OpenStackAdapter) GetDeploymentManager(_ context.Context, id string) (*adapter.DeploymentManager, error) {
	a.logger.Debug("GetDeploymentManager called",
		zap.String("id", id))

	if id != a.deploymentManagerID {
		return nil, fmt.Errorf("deployment manager not found: %s", id)
	}

	// Query OpenStack region information
	allPages, err := regions.List(a.identity, nil).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list regions: %w", err)
	}

	regionList, err := regions.ExtractRegions(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract regions: %w", err)
	}

	// Find the current region
	var currentRegion *regions.Region
	for _, r := range regionList {
		if r.ID == a.region {
			currentRegion = &r
			break
		}
	}

	if currentRegion == nil {
		return nil, fmt.Errorf("region not found: %s", a.region)
	}

	// Construct deployment manager metadata
	dm := &adapter.DeploymentManager{
		DeploymentManagerID: a.deploymentManagerID,
		Name:                fmt.Sprintf("OpenStack %s", a.region),
		Description:         fmt.Sprintf("OpenStack NFVi deployment in region %s", a.region),
		OCloudID:            a.oCloudID,
		ServiceURI:          a.provider.IdentityEndpoint,
		SupportedLocations:  []string{a.region},
		Capabilities: []string{
			"resource-pools",
			"resources",
			"resource-types",
			"subscriptions",
		},
		Extensions: map[string]interface{}{
			"openstack.region":       currentRegion.ID,
			"openstack.description":  currentRegion.Description,
			"openstack.parentRegion": currentRegion.ParentRegionID,
			"openstack.projectName":  a.projectName,
			"openstack.authURL":      a.provider.IdentityEndpoint,
		},
	}

	a.logger.Info("retrieved deployment manager",
		zap.String("deploymentManagerID", dm.DeploymentManagerID),
		zap.String("region", a.region))

	return dm, nil
}

// Health performs a health check on the OpenStack backend.
// It verifies connectivity to Nova, Placement, and Keystone services.
func (a *OpenStackAdapter) Health(ctx context.Context) error {
	a.logger.Debug("health check called")

	// Check Nova compute service
	if err := a.checkNovaHealth(ctx); err != nil {
		a.logger.Error("Nova health check failed", zap.Error(err))
		return fmt.Errorf("nova API unreachable: %w", err)
	}

	// Check Placement service
	if err := a.checkPlacementHealth(ctx); err != nil {
		a.logger.Error("Placement health check failed", zap.Error(err))
		return fmt.Errorf("placement API unreachable: %w", err)
	}

	// Check Keystone identity service
	if err := a.checkKeystoneHealth(ctx); err != nil {
		a.logger.Error("Keystone health check failed", zap.Error(err))
		return fmt.Errorf("keystone API unreachable: %w", err)
	}

	a.logger.Debug("health check passed")
	return nil
}

// checkNovaHealth verifies Nova compute service connectivity.
func (a *OpenStackAdapter) checkNovaHealth(_ context.Context) error {
	// Query a small number of servers to verify connectivity
	listOpts := servers.ListOpts{
		Limit: 1,
	}

	_, err := servers.List(a.compute, listOpts).AllPages()
	if err != nil {
		return fmt.Errorf("failed to query Nova servers: %w", err)
	}

	return nil
}

// checkPlacementHealth verifies Placement service connectivity.
func (a *OpenStackAdapter) checkPlacementHealth(_ context.Context) error {
	// Query resource providers to verify connectivity
	_, err := resourceproviders.List(a.placement, resourceproviders.ListOpts{}).AllPages()
	if err != nil {
		return fmt.Errorf("failed to query Placement resource providers: %w", err)
	}

	return nil
}

// checkKeystoneHealth verifies Keystone identity service connectivity.
func (a *OpenStackAdapter) checkKeystoneHealth(_ context.Context) error {
	// Query regions to verify connectivity
	_, err := regions.List(a.identity, nil).AllPages()
	if err != nil {
		return fmt.Errorf("failed to query Keystone regions: %w", err)
	}

	return nil
}

// Close cleanly shuts down the adapter and releases resources.
func (a *OpenStackAdapter) Close() error {
	a.logger.Info("closing OpenStack adapter")

	// Stop all polling goroutines
	a.StopAllPolling()

	// Sync logger before shutdown (ignore sync errors on stderr/stdout)
	_ = a.logger.Sync()

	return nil
}

// NOTE: Filter matching and pagination use shared helpers from internal/adapter/helpers.go
// Use adapter.MatchesFilter() and adapter.ApplyPagination() instead of local implementations.

// generateFlavorID generates a consistent resource type ID for a flavor.
func generateFlavorID(flavor *flavors.Flavor) string {
	return fmt.Sprintf("openstack-flavor-%s", flavor.ID)
}
