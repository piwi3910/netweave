// Package kubernetes provides a Kubernetes-native implementation of the O2-IMS adapter interface.
// It translates O2-IMS API operations to native Kubernetes API calls, mapping O2-IMS resources
// to Kubernetes resources like Nodes, MachineSets, and StorageClasses.
package kubernetes

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/piwi3910/netweave/internal/adapter"
)

// KubernetesAdapter implements the adapter.Adapter interface for Kubernetes backends.
// It provides O2-IMS functionality by mapping O2-IMS resources to native Kubernetes resources:
//   - Resource Pools → MachineSets / NodePools
//   - Resources → Nodes / Machines
//   - Resource Types → Aggregated from Nodes and StorageClasses
//   - Deployment Manager → Custom Resource or ConfigMap
//   - Subscriptions → Kubernetes Informers + Redis storage
type KubernetesAdapter struct {
	// client is the Kubernetes client for API operations.
	client kubernetes.Interface

	// logger provides structured logging.
	logger *zap.Logger

	// oCloudID is the identifier of the parent O-Cloud.
	oCloudID string

	// deploymentManagerID is the identifier for this deployment manager.
	deploymentManagerID string

	// namespace is the default namespace for O2-IMS resources.
	namespace string
}

// Config holds configuration for creating a KubernetesAdapter.
type Config struct {
	// Kubeconfig is the path to the kubeconfig file.
	// If empty, in-cluster configuration will be used.
	Kubeconfig string

	// OCloudID is the identifier of the parent O-Cloud.
	OCloudID string

	// DeploymentManagerID is the identifier for this deployment manager.
	DeploymentManagerID string

	// Namespace is the default namespace for O2-IMS resources.
	// Defaults to "o2ims-system" if not specified.
	Namespace string

	// Logger is the logger to use. If nil, a default logger will be created.
	Logger *zap.Logger
}

// New creates a new KubernetesAdapter with the provided configuration.
// It initializes the Kubernetes client using either kubeconfig or in-cluster config.
//
// Example:
//
//	adapter, err := kubernetes.New(&kubernetes.Config{
//	    Kubeconfig:          "/path/to/kubeconfig",
//	    OCloudID:            "ocloud-1",
//	    DeploymentManagerID: "ocloud-k8s-1",
//	    Namespace:           "o2ims-system",
//	})
func New(cfg *Config) (*KubernetesAdapter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Set default namespace if not specified
	namespace := cfg.Namespace
	if namespace == "" {
		namespace = "o2ims-system"
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

	// Build Kubernetes client configuration
	var restConfig *rest.Config
	var err error

	if cfg.Kubeconfig != "" {
		// Use kubeconfig file
		restConfig, err = clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
		}
		logger.Info("initialized Kubernetes client from kubeconfig",
			zap.String("kubeconfig", cfg.Kubeconfig))
	} else {
		// Use in-cluster configuration
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to build in-cluster config: %w", err)
		}
		logger.Info("initialized Kubernetes client from in-cluster config")
	}

	// Create Kubernetes client
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	adapter := &KubernetesAdapter{
		client:              client,
		logger:              logger,
		oCloudID:            cfg.OCloudID,
		deploymentManagerID: cfg.DeploymentManagerID,
		namespace:           namespace,
	}

	logger.Info("Kubernetes adapter initialized",
		zap.String("oCloudId", cfg.OCloudID),
		zap.String("deploymentManagerId", cfg.DeploymentManagerID),
		zap.String("namespace", namespace))

	return adapter, nil
}

// Name returns the adapter name.
func (a *KubernetesAdapter) Name() string {
	return "kubernetes"
}

// Version returns the Kubernetes API version this adapter supports.
func (a *KubernetesAdapter) Version() string {
	// This will be populated from server version in future implementation
	return "1.30.0"
}

// Capabilities returns the list of O2-IMS capabilities supported by this adapter.
func (a *KubernetesAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityResourcePools,
		adapter.CapabilityResources,
		adapter.CapabilityResourceTypes,
		adapter.CapabilityDeploymentManagers,
		adapter.CapabilitySubscriptions,
		adapter.CapabilityHealthChecks,
	}
}

// GetDeploymentManager, ListResourcePools, GetResourcePool, CreateResourcePool,
// UpdateResourcePool, DeleteResourcePool, ListResources, GetResource, CreateResource,
// DeleteResource, ListResourceTypes, and GetResourceType are implemented in separate files:
// - deploymentmanagers.go: Deployment manager operations
// - resourcepools.go: Resource pool operations
// - resources.go: Resource operations
// - resourcetypes.go: Resource type operations

// CreateSubscription creates a new event subscription.
// Implementation will store subscription in Redis and start watching K8s resources.
func (a *KubernetesAdapter) CreateSubscription(
	_ context.Context,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	a.logger.Debug("CreateSubscription called",
		zap.String("callback", sub.Callback))

	// TODO: Implement subscription creation
	// This will store in Redis and configure informers
	return nil, fmt.Errorf("not implemented")
}

// GetSubscription retrieves a specific subscription by ID.
// Implementation will retrieve subscription from Redis.
func (a *KubernetesAdapter) GetSubscription(_ context.Context, id string) (*adapter.Subscription, error) {
	a.logger.Debug("GetSubscription called",
		zap.String("id", id))

	// TODO: Implement subscription retrieval
	// This will retrieve from Redis
	return nil, fmt.Errorf("not implemented")
}

// DeleteSubscription deletes a subscription by ID.
// Implementation will remove subscription from Redis and stop watching.
func (a *KubernetesAdapter) DeleteSubscription(_ context.Context, id string) error {
	a.logger.Debug("DeleteSubscription called",
		zap.String("id", id))

	// TODO: Implement subscription deletion
	// This will remove from Redis and stop informers
	return fmt.Errorf("not implemented")
}

// Health performs a health check on the Kubernetes backend.
// It verifies connectivity to the Kubernetes API server.
func (a *KubernetesAdapter) Health(_ context.Context) error {
	a.logger.Debug("Health check called")

	// Perform basic health check by querying server version
	_, err := a.client.Discovery().ServerVersion()
	if err != nil {
		a.logger.Error("health check failed",
			zap.Error(err))
		return fmt.Errorf("kubernetes API unreachable: %w", err)
	}

	a.logger.Debug("health check passed")
	return nil
}

// Close cleanly shuts down the adapter and releases resources.
func (a *KubernetesAdapter) Close() error {
	a.logger.Info("closing Kubernetes adapter")

	// Sync logger before shutdown
	// Ignore sync errors on stderr/stdout which are common
	_ = a.logger.Sync()

	return nil
}
