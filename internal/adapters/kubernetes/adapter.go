// Package kubernetes provides a Kubernetes-native implementation of the O2-IMS adapter interface.
// It translates O2-IMS API operations to native Kubernetes API calls, mapping O2-IMS resources
// to Kubernetes resources like Nodes, MachineSets, and StorageClasses.
package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/storage"
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

	// store is the subscription storage backend (Redis).
	store storage.Store

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

	// Store is the subscription storage backend (Redis).
	// Optional: If nil, subscription operations will return not implemented errors.
	Store storage.Store

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
		store:               cfg.Store,
		logger:              logger,
		oCloudID:            cfg.OCloudID,
		deploymentManagerID: cfg.DeploymentManagerID,
		namespace:           namespace,
	}

	logger.Info("Kubernetes adapter initialized",
		zap.String("oCloudId", cfg.OCloudID),
		zap.String("deploymentManagerId", cfg.DeploymentManagerID),
		zap.String("namespace", namespace),
		zap.Bool("subscriptionsEnabled", cfg.Store != nil))

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
// Stores subscription in Redis. Kubernetes watch/informer integration is handled
// by the controller package which monitors Redis subscriptions.
func (a *KubernetesAdapter) CreateSubscription(
	ctx context.Context,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	a.logger.Debug("CreateSubscription called",
		zap.String("callback", sub.Callback),
		zap.String("subscriptionId", sub.SubscriptionID))

	if a.store == nil {
		a.logger.Warn("subscription storage not configured")
		return nil, fmt.Errorf("subscription storage not configured")
	}

	// Convert adapter.Subscription to storage.Subscription
	var filter storage.SubscriptionFilter
	if sub.Filter != nil {
		filter = storage.SubscriptionFilter{
			ResourcePoolID: sub.Filter.ResourcePoolID,
			ResourceTypeID: sub.Filter.ResourceTypeID,
			ResourceID:     sub.Filter.ResourceID,
		}
	}

	storageSub := &storage.Subscription{
		ID:                     sub.SubscriptionID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 filter,
	}

	// Store subscription in Redis
	if err := a.store.Create(ctx, storageSub); err != nil {
		a.logger.Error("failed to store subscription",
			zap.String("subscriptionId", sub.SubscriptionID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to store subscription: %w", err)
	}

	a.logger.Info("subscription created",
		zap.String("subscriptionId", sub.SubscriptionID),
		zap.String("callback", sub.Callback))

	return sub, nil
}

// GetSubscription retrieves a specific subscription by ID from Redis.
func (a *KubernetesAdapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	a.logger.Debug("GetSubscription called",
		zap.String("id", id))

	if a.store == nil {
		a.logger.Warn("subscription storage not configured")
		return nil, fmt.Errorf("subscription storage not configured")
	}

	// Retrieve subscription from Redis
	storageSub, err := a.store.Get(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			a.logger.Debug("subscription not found",
				zap.String("subscriptionId", id))
			return nil, fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
		}
		a.logger.Error("failed to retrieve subscription",
			zap.String("subscriptionId", id),
			zap.Error(err))
		return nil, fmt.Errorf("failed to retrieve subscription: %w", err)
	}

	// Convert storage.Subscription to adapter.Subscription
	var filter *adapter.SubscriptionFilter
	hasFilter := storageSub.Filter.ResourcePoolID != "" ||
		storageSub.Filter.ResourceTypeID != "" ||
		storageSub.Filter.ResourceID != ""
	if hasFilter {
		filter = &adapter.SubscriptionFilter{
			ResourcePoolID: storageSub.Filter.ResourcePoolID,
			ResourceTypeID: storageSub.Filter.ResourceTypeID,
			ResourceID:     storageSub.Filter.ResourceID,
		}
	}

	adapterSub := &adapter.Subscription{
		SubscriptionID:         storageSub.ID,
		Callback:               storageSub.Callback,
		ConsumerSubscriptionID: storageSub.ConsumerSubscriptionID,
		Filter:                 filter,
	}

	return adapterSub, nil
}

// UpdateSubscription updates an existing subscription.
// Updates both the subscription in Redis and notifies the controller to restart watchers.
func (a *KubernetesAdapter) UpdateSubscription(ctx context.Context, id string, sub *adapter.Subscription) (result *adapter.Subscription, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("kubernetes", "UpdateSubscription", start, err) }()

	a.logger.Debug("UpdateSubscription called",
		zap.String("id", id),
		zap.String("callback", sub.Callback))

	if a.store == nil {
		a.logger.Warn("subscription storage not configured")
		return nil, fmt.Errorf("subscription storage not configured")
	}

	// Validate callback URL
	if sub.Callback == "" {
		return nil, fmt.Errorf("callback URL is required")
	}

	// Get existing subscription for logging
	existingSub, err := a.getExistingSubscription(ctx, id)
	if err != nil {
		return nil, err
	}

	// Prepare and update storage subscription
	storageSub := a.convertToStorageSubscription(id, sub)
	if err := a.updateSubscriptionInStore(ctx, id, storageSub); err != nil {
		return nil, err
	}

	a.logger.Info("subscription updated",
		zap.String("subscriptionId", id),
		zap.String("oldCallback", existingSub.Callback),
		zap.String("newCallback", sub.Callback))

	// Return updated subscription
	return a.convertToAdapterSubscription(id, storageSub), nil
}

// getExistingSubscription retrieves an existing subscription with proper error handling.
func (a *KubernetesAdapter) getExistingSubscription(ctx context.Context, id string) (*storage.Subscription, error) {
	existingSub, err := a.store.Get(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			a.logger.Debug("subscription not found",
				zap.String("subscriptionId", id))
			return nil, fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
		}
		a.logger.Error("failed to retrieve existing subscription",
			zap.String("subscriptionId", id),
			zap.Error(err))
		return nil, fmt.Errorf("failed to retrieve subscription: %w", err)
	}
	return existingSub, nil
}

// convertToStorageSubscription converts adapter subscription to storage format.
// Filter handling: A nil Filter means no filtering (subscribe to all resources).
// An empty Filter struct {} also means no filtering since all fields will be empty strings.
// Partial filters are supported (e.g., only ResourcePoolID set filters by pool only).
func (a *KubernetesAdapter) convertToStorageSubscription(id string, sub *adapter.Subscription) *storage.Subscription {
	storageSub := &storage.Subscription{
		ID:                     id,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
	}

	if sub.Filter != nil {
		storageSub.Filter = storage.SubscriptionFilter{
			ResourcePoolID: sub.Filter.ResourcePoolID,
			ResourceTypeID: sub.Filter.ResourceTypeID,
			ResourceID:     sub.Filter.ResourceID,
		}
	}

	return storageSub
}

// updateSubscriptionInStore updates subscription in Redis with proper error handling.
func (a *KubernetesAdapter) updateSubscriptionInStore(ctx context.Context, id string, storageSub *storage.Subscription) error {
	if err := a.store.Update(ctx, storageSub); err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			a.logger.Debug("subscription not found",
				zap.String("subscriptionId", id))
			return fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
		}
		a.logger.Error("failed to update subscription",
			zap.String("subscriptionId", id),
			zap.Error(err))
		return fmt.Errorf("failed to update subscription: %w", err)
	}
	return nil
}

// convertToAdapterSubscription converts storage subscription to adapter format.
func (a *KubernetesAdapter) convertToAdapterSubscription(id string, storageSub *storage.Subscription) *adapter.Subscription {
	var filter *adapter.SubscriptionFilter
	hasFilter := storageSub.Filter.ResourcePoolID != "" ||
		storageSub.Filter.ResourceTypeID != "" ||
		storageSub.Filter.ResourceID != ""
	if hasFilter {
		filter = &adapter.SubscriptionFilter{
			ResourcePoolID: storageSub.Filter.ResourcePoolID,
			ResourceTypeID: storageSub.Filter.ResourceTypeID,
			ResourceID:     storageSub.Filter.ResourceID,
		}
	}

	return &adapter.Subscription{
		SubscriptionID:         id,
		Callback:               storageSub.Callback,
		ConsumerSubscriptionID: storageSub.ConsumerSubscriptionID,
		Filter:                 filter,
	}
}

// DeleteSubscription deletes a subscription by ID from Redis.
// The controller package monitors Redis and stops the corresponding Kubernetes watchers.
func (a *KubernetesAdapter) DeleteSubscription(ctx context.Context, id string) error {
	a.logger.Debug("DeleteSubscription called",
		zap.String("id", id))

	if a.store == nil {
		a.logger.Warn("subscription storage not configured")
		return fmt.Errorf("subscription storage not configured")
	}

	// Delete subscription from Redis
	if err := a.store.Delete(ctx, id); err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			a.logger.Debug("subscription not found for deletion",
				zap.String("subscriptionId", id))
			return fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
		}
		a.logger.Error("failed to delete subscription",
			zap.String("subscriptionId", id),
			zap.Error(err))
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	a.logger.Info("subscription deleted",
		zap.String("subscriptionId", id))

	return nil
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
