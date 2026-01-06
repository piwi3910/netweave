// Package azure provides an Azure implementation of the O2-IMS adapter interface.
// It translates O2-IMS API operations to Azure API calls, mapping O2-IMS resources
// to Azure resources like Virtual Machines, Resource Groups, and VM sizes.
//
// Resource Mapping:
//   - Resource Pools → Resource Groups or Availability Zones
//   - Resources → Azure Virtual Machines
//   - Resource Types → Azure VM Sizes
//   - Deployment Manager → Azure Subscription/Region metadata
package azure

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// AzureAdapter implements the adapter.Adapter interface for Azure backends.
// It provides O2-IMS functionality by mapping O2-IMS resources to Azure resources:
//   - Resource Pools → Resource Groups or Availability Zones
//   - Resources → Azure Virtual Machines
//   - Resource Types → Azure VM Sizes (Standard_D2s_v3, etc.)
//   - Deployment Manager → Azure Subscription/Region metadata
//   - Subscriptions → Event Grid based (polling as fallback)
type AzureAdapter struct {
	// vmClient is the Azure VM client.
	vmClient *armcompute.VirtualMachinesClient

	// vmSizeClient is the Azure VM Sizes client.
	vmSizeClient *armcompute.VirtualMachineSizesClient

	// resourceGroupClient is the Azure Resource Group client.
	resourceGroupClient *armresources.ResourceGroupsClient

	// credential is the Azure credential.
	credential azcore.TokenCredential

	// logger provides structured logging.
	logger *zap.Logger

	// oCloudID is the identifier of the parent O-Cloud.
	oCloudID string

	// deploymentManagerID is the identifier for this deployment manager.
	deploymentManagerID string

	// subscriptionID is the Azure subscription ID.
	subscriptionID string

	// location is the Azure region/location.
	location string

	// subscriptions holds active O2-IMS subscriptions (polling-based fallback).
	// Note: Subscriptions are stored in-memory and will be lost on adapter restart.
	// For production use, consider implementing persistent storage via Redis.
	subscriptions map[string]*adapter.Subscription

	// subscriptionsMu protects the subscriptions map.
	subscriptionsMu sync.RWMutex

	// poolMode determines how resource pools are mapped.
	// "rg" maps to Resource Groups, "az" maps to Availability Zones.
	poolMode string
}

// Config holds configuration for creating an AzureAdapter.
type Config struct {
	// SubscriptionID is the Azure subscription ID.
	SubscriptionID string

	// Location is the Azure region/location (e.g., "eastus", "westeurope").
	Location string

	// TenantID is the Azure AD tenant ID (required for service principal auth).
	TenantID string

	// ClientID is the Azure AD application/client ID (required for service principal auth).
	ClientID string

	// ClientSecret is the Azure AD application secret (required for service principal auth).
	ClientSecret string

	// UseManagedIdentity enables Azure Managed Identity authentication.
	// If true, TenantID, ClientID, and ClientSecret are ignored.
	UseManagedIdentity bool

	// OCloudID is the identifier of the parent O-Cloud.
	OCloudID string

	// DeploymentManagerID is the identifier for this deployment manager.
	// If empty, defaults to "ocloud-azure-{location}".
	DeploymentManagerID string

	// PoolMode determines how resource pools are mapped:
	// - "rg": Map to Resource Groups (default)
	// - "az": Map to Availability Zones
	PoolMode string

	// Timeout is the timeout for Azure API calls.
	// Defaults to 30 seconds if not specified.
	Timeout time.Duration

	// Logger is the logger to use. If nil, a default logger will be created.
	Logger *zap.Logger
}

// New creates a new AzureAdapter with the provided configuration.
// It authenticates with Azure and initializes service clients.
//
// Example:
//
//	adp, err := azure.New(&azure.Config{
//	    SubscriptionID: os.Getenv("AZURE_SUBSCRIPTION_ID"),
//	    Location:       "eastus",
//	    TenantID:       os.Getenv("AZURE_TENANT_ID"),
//	    ClientID:       os.Getenv("AZURE_CLIENT_ID"),
//	    ClientSecret:   os.Getenv("AZURE_CLIENT_SECRET"),
//	    OCloudID:       "ocloud-azure-1",
//	    PoolMode:       "rg",
//	})
func New(cfg *Config) (*AzureAdapter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Validate required configuration
	if cfg.SubscriptionID == "" {
		return nil, fmt.Errorf("subscriptionID is required")
	}
	if cfg.Location == "" {
		return nil, fmt.Errorf("location is required")
	}
	if cfg.OCloudID == "" {
		return nil, fmt.Errorf("oCloudID is required")
	}

	// Validate authentication configuration
	if !cfg.UseManagedIdentity {
		if cfg.TenantID == "" {
			return nil, fmt.Errorf("tenantID is required when not using managed identity")
		}
		if cfg.ClientID == "" {
			return nil, fmt.Errorf("clientID is required when not using managed identity")
		}
		if cfg.ClientSecret == "" {
			return nil, fmt.Errorf("clientSecret is required when not using managed identity")
		}
	}

	// Set defaults
	deploymentManagerID := cfg.DeploymentManagerID
	if deploymentManagerID == "" {
		deploymentManagerID = fmt.Sprintf("ocloud-azure-%s", cfg.Location)
	}

	poolMode := cfg.PoolMode
	if poolMode == "" {
		poolMode = "rg"
	}
	if poolMode != "rg" && poolMode != "az" {
		return nil, fmt.Errorf("poolMode must be 'rg' or 'az', got %q", poolMode)
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

	logger.Info("initializing Azure adapter",
		zap.String("subscriptionID", cfg.SubscriptionID),
		zap.String("location", cfg.Location),
		zap.String("oCloudID", cfg.OCloudID),
		zap.String("poolMode", poolMode),
		zap.Bool("useManagedIdentity", cfg.UseManagedIdentity))

	// Create Azure credential
	var cred azcore.TokenCredential
	var err error

	if cfg.UseManagedIdentity {
		cred, err = azidentity.NewManagedIdentityCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create managed identity credential: %w", err)
		}
		logger.Info("using Azure Managed Identity for authentication")
	} else {
		cred, err = azidentity.NewClientSecretCredential(
			cfg.TenantID,
			cfg.ClientID,
			cfg.ClientSecret,
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create service principal credential: %w", err)
		}
		logger.Info("using Azure Service Principal for authentication",
			zap.String("tenantID", cfg.TenantID),
			zap.String("clientID", cfg.ClientID))
	}

	// Create ARM client options
	clientOpts := &arm.ClientOptions{}

	// Create VM client
	vmClient, err := armcompute.NewVirtualMachinesClient(cfg.SubscriptionID, cred, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM client: %w", err)
	}

	// Create VM Sizes client
	vmSizeClient, err := armcompute.NewVirtualMachineSizesClient(cfg.SubscriptionID, cred, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM Sizes client: %w", err)
	}

	// Create Resource Group client
	resourceGroupClient, err := armresources.NewResourceGroupsClient(cfg.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Resource Group client: %w", err)
	}

	adp := &AzureAdapter{
		vmClient:            vmClient,
		vmSizeClient:        vmSizeClient,
		resourceGroupClient: resourceGroupClient,
		credential:          cred,
		logger:              logger,
		oCloudID:            cfg.OCloudID,
		deploymentManagerID: deploymentManagerID,
		subscriptionID:      cfg.SubscriptionID,
		location:            cfg.Location,
		subscriptions:       make(map[string]*adapter.Subscription),
		poolMode:            poolMode,
	}

	logger.Info("Azure adapter initialized successfully",
		zap.String("oCloudID", cfg.OCloudID),
		zap.String("deploymentManagerID", deploymentManagerID),
		zap.String("location", cfg.Location),
		zap.String("poolMode", poolMode))

	return adp, nil
}

// Name returns the adapter name.
func (a *AzureAdapter) Name() string {
	return "azure"
}

// Version returns the Azure API version this adapter supports.
func (a *AzureAdapter) Version() string {
	return "compute-2023-09-01"
}

// Capabilities returns the list of O2-IMS capabilities supported by this adapter.
func (a *AzureAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityResourcePools,
		adapter.CapabilityResources,
		adapter.CapabilityResourceTypes,
		adapter.CapabilityDeploymentManagers,
		adapter.CapabilitySubscriptions, // Polling-based
		adapter.CapabilityHealthChecks,
	}
}

// Health performs a health check on the Azure backend.
// It verifies connectivity to Azure services.
func (a *AzureAdapter) Health(ctx context.Context) error {
	a.logger.Debug("health check called")

	// Use a timeout to prevent indefinite blocking
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check VM Sizes service (lightweight API call)
	pager := a.vmSizeClient.NewListPager(a.location, nil)
	_, err := pager.NextPage(healthCtx)
	if err != nil {
		a.logger.Error("Azure health check failed", zap.Error(err))
		return fmt.Errorf("azure API unreachable: %w", err)
	}

	a.logger.Debug("health check passed")
	return nil
}

// Close cleanly shuts down the adapter and releases resources.
func (a *AzureAdapter) Close() error {
	a.logger.Info("closing Azure adapter")

	// Clear subscriptions
	a.subscriptionsMu.Lock()
	a.subscriptions = make(map[string]*adapter.Subscription)
	a.subscriptionsMu.Unlock()

	// Sync logger before shutdown
	if err := a.logger.Sync(); err != nil {
		// Ignore sync errors on stderr/stdout
		return nil
	}

	return nil
}

// NOTE: Filter matching and pagination use shared helpers from internal/adapter/helpers.go
// Use adapter.MatchesFilter() and adapter.ApplyPagination() instead of local implementations.

// generateVMSizeID generates a consistent resource type ID for a VM size.
func generateVMSizeID(vmSize string) string {
	return fmt.Sprintf("azure-vm-size-%s", vmSize)
}

// generateVMID generates a consistent resource ID for an Azure VM.
func generateVMID(vmName, resourceGroup string) string {
	return fmt.Sprintf("azure-vm-%s-%s", resourceGroup, vmName)
}

// generateRGPoolID generates a consistent resource pool ID for a Resource Group.
func generateRGPoolID(resourceGroup string) string {
	return fmt.Sprintf("azure-rg-%s", resourceGroup)
}

// generateAZPoolID generates a consistent resource pool ID for an Availability Zone.
func generateAZPoolID(location, zone string) string {
	return fmt.Sprintf("azure-az-%s-%s", location, zone)
}

// tagsToMap converts Azure tags (map[string]*string) to a map[string]string.
func tagsToMap(tags map[string]*string) map[string]string {
	result := make(map[string]string)
	for k, v := range tags {
		if v != nil {
			result[k] = *v
		}
	}
	return result
}

// ptrToString safely converts a *string to string.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ptrToInt32 safely converts a *int32 to int32.
func ptrToInt32(i *int32) int32 {
	if i == nil {
		return 0
	}
	return *i
}
