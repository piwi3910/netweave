// Package controllers provides control loops for managing O2-IMS subscriptions.
// It implements Kubernetes watch/informer mechanisms for event notifications.
package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/piwi3910/netweave/internal/storage"
)

const (
	// EventStreamKey is the Redis Stream key for webhook events.
	EventStreamKey = "o2ims:events"

	// MaxStreamLength limits the event stream size.
	MaxStreamLength = 10000

	// InformerResyncPeriod is the resync interval for Kubernetes informers.
	InformerResyncPeriod = 30 * time.Second
)

// EventType represents the type of resource event.
type EventType string

const (
	// EventTypeCreated indicates a resource was created.
	EventTypeCreated EventType = "Created"

	// EventTypeUpdated indicates a resource was updated.
	EventTypeUpdated EventType = "Updated"

	// EventTypeDeleted indicates a resource was deleted.
	EventTypeDeleted EventType = "Deleted"
)

// ResourceEvent represents a resource change event.
type ResourceEvent struct {
	// SubscriptionID is the ID of the subscription receiving this event.
	SubscriptionID string `json:"subscriptionId"`

	// EventType is the type of event (Created, Updated, Deleted).
	EventType string `json:"notificationEventType"`

	// ObjectRef is the O2-IMS API path to the resource.
	ObjectRef string `json:"objectRef"`

	// ResourceTypeID identifies the resource type.
	ResourceTypeID string `json:"resourceTypeId"`

	// ResourcePoolID identifies the resource pool (if applicable).
	ResourcePoolID string `json:"resourcePoolId,omitempty"`

	// GlobalResourceID is the global identifier for the resource.
	GlobalResourceID string `json:"globalResourceId"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// NotificationID is a unique identifier for this notification.
	NotificationID string `json:"notificationId"`

	// CallbackURL is the webhook endpoint to deliver to.
	CallbackURL string `json:"callbackUrl"`
}

// SubscriptionController watches Kubernetes resources and delivers webhook notifications.
type SubscriptionController struct {
	// k8sClient is the Kubernetes client for API operations.
	k8sClient kubernetes.Interface

	// store is the subscription storage backend.
	store storage.Store

	// redisClient is used for event queue operations.
	redisClient *redis.Client

	// logger provides structured logging.
	logger *zap.Logger

	// oCloudID is the identifier of the parent O-Cloud.
	oCloudID string

	// informerFactory creates Kubernetes informers.
	informerFactory informers.SharedInformerFactory

	// stopCh is used to signal controller shutdown.
	stopCh chan struct{}

	// wg tracks running goroutines.
	wg sync.WaitGroup
}

// Config holds configuration for creating a SubscriptionController.
type Config struct {
	// K8sClient is the Kubernetes client for API operations.
	K8sClient kubernetes.Interface

	// Store is the subscription storage backend.
	Store storage.Store

	// RedisClient is used for event queue operations.
	RedisClient *redis.Client

	// Logger is the logger to use.
	Logger *zap.Logger

	// OCloudID is the identifier of the parent O-Cloud.
	OCloudID string
}

// NewSubscriptionController creates a new SubscriptionController.
func NewSubscriptionController(cfg *Config) (*SubscriptionController, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if cfg.K8sClient == nil {
		return nil, fmt.Errorf("k8s client cannot be nil")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("store cannot be nil")
	}
	if cfg.RedisClient == nil {
		return nil, fmt.Errorf("redis client cannot be nil")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if cfg.OCloudID == "" {
		return nil, fmt.Errorf("oCloudID cannot be empty")
	}

	factory := informers.NewSharedInformerFactory(cfg.K8sClient, InformerResyncPeriod)

	return &SubscriptionController{
		k8sClient:       cfg.K8sClient,
		store:           cfg.Store,
		redisClient:     cfg.RedisClient,
		logger:          cfg.Logger,
		oCloudID:        cfg.OCloudID,
		informerFactory: factory,
		stopCh:          make(chan struct{}),
	}, nil
}

// Start starts the subscription controller and begins watching resources.
func (c *SubscriptionController) Start(ctx context.Context) error {
	c.logger.Info("starting subscription controller")

	// Set up Kubernetes informers for watched resources
	if err := c.setupInformers(); err != nil {
		return fmt.Errorf("failed to setup informers: %w", err)
	}

	// Start informers
	c.informerFactory.Start(c.stopCh)

	// Wait for informer caches to sync
	c.logger.Info("waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(c.stopCh,
		c.informerFactory.Core().V1().Nodes().Informer().HasSynced,
		c.informerFactory.Core().V1().Namespaces().Informer().HasSynced,
	); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	c.logger.Info("subscription controller started successfully")

	// Wait for context cancellation
	<-ctx.Done()

	// Stop controller
	return c.Stop()
}

// Stop stops the subscription controller and waits for all goroutines to finish.
func (c *SubscriptionController) Stop() error {
	c.logger.Info("stopping subscription controller")

	// Signal shutdown
	close(c.stopCh)

	// Wait for all goroutines to finish
	c.wg.Wait()

	c.logger.Info("subscription controller stopped")
	return nil
}

// setupInformers configures event handlers for Kubernetes resources.
func (c *SubscriptionController) setupInformers() error {
	// Watch Nodes (Resources)
	nodeInformer := c.informerFactory.Core().V1().Nodes().Informer()
	if _, err := nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleNodeAdd,
		UpdateFunc: c.handleNodeUpdate,
		DeleteFunc: c.handleNodeDelete,
	}); err != nil {
		return fmt.Errorf("failed to add node event handler: %w", err)
	}

	// Watch Namespaces (Resource Pools)
	namespaceInformer := c.informerFactory.Core().V1().Namespaces().Informer()
	if _, err := namespaceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleNamespaceAdd,
		UpdateFunc: c.handleNamespaceUpdate,
		DeleteFunc: c.handleNamespaceDelete,
	}); err != nil {
		return fmt.Errorf("failed to add namespace event handler: %w", err)
	}

	return nil
}

// handleNodeAdd handles node creation events.
func (c *SubscriptionController) handleNodeAdd(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		c.logger.Error("invalid object type in handleNodeAdd")
		return
	}

	c.logger.Debug("node created",
		zap.String("node", node.Name))

	ctx := context.Background()
	c.processNodeEvent(ctx, node, EventTypeCreated)
}

// handleNodeUpdate handles node update events.
func (c *SubscriptionController) handleNodeUpdate(oldObj, newObj interface{}) {
	oldNode, ok := oldObj.(*corev1.Node)
	if !ok {
		c.logger.Error("invalid old object type in handleNodeUpdate")
		return
	}

	newNode, ok := newObj.(*corev1.Node)
	if !ok {
		c.logger.Error("invalid new object type in handleNodeUpdate")
		return
	}

	// Only process if resource version changed
	if oldNode.ResourceVersion == newNode.ResourceVersion {
		return
	}

	c.logger.Debug("node updated",
		zap.String("node", newNode.Name))

	ctx := context.Background()
	c.processNodeEvent(ctx, newNode, EventTypeUpdated)
}

// handleNodeDelete handles node deletion events.
func (c *SubscriptionController) handleNodeDelete(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		// Handle DeletedFinalStateUnknown
		tombstone, tombstoneOk := obj.(cache.DeletedFinalStateUnknown)
		if !tombstoneOk {
			c.logger.Error("invalid object type in handleNodeDelete")
			return
		}
		node, ok = tombstone.Obj.(*corev1.Node)
		if !ok {
			c.logger.Error("invalid tombstone object type")
			return
		}
	}

	c.logger.Debug("node deleted",
		zap.String("node", node.Name))

	ctx := context.Background()
	c.processNodeEvent(ctx, node, EventTypeDeleted)
}

// handleNamespaceAdd handles namespace creation events.
func (c *SubscriptionController) handleNamespaceAdd(obj interface{}) {
	ns, ok := obj.(*corev1.Namespace)
	if !ok {
		c.logger.Error("invalid object type in handleNamespaceAdd")
		return
	}

	c.logger.Debug("namespace created",
		zap.String("namespace", ns.Name))

	ctx := context.Background()
	c.processNamespaceEvent(ctx, ns, EventTypeCreated)
}

// handleNamespaceUpdate handles namespace update events.
func (c *SubscriptionController) handleNamespaceUpdate(oldObj, newObj interface{}) {
	oldNs, ok := oldObj.(*corev1.Namespace)
	if !ok {
		c.logger.Error("invalid old object type in handleNamespaceUpdate")
		return
	}

	newNs, ok := newObj.(*corev1.Namespace)
	if !ok {
		c.logger.Error("invalid new object type in handleNamespaceUpdate")
		return
	}

	// Only process if resource version changed
	if oldNs.ResourceVersion == newNs.ResourceVersion {
		return
	}

	c.logger.Debug("namespace updated",
		zap.String("namespace", newNs.Name))

	ctx := context.Background()
	c.processNamespaceEvent(ctx, newNs, EventTypeUpdated)
}

// handleNamespaceDelete handles namespace deletion events.
func (c *SubscriptionController) handleNamespaceDelete(obj interface{}) {
	ns, ok := obj.(*corev1.Namespace)
	if !ok {
		// Handle DeletedFinalStateUnknown
		tombstone, tombstoneOk := obj.(cache.DeletedFinalStateUnknown)
		if !tombstoneOk {
			c.logger.Error("invalid object type in handleNamespaceDelete")
			return
		}
		ns, ok = tombstone.Obj.(*corev1.Namespace)
		if !ok {
			c.logger.Error("invalid tombstone object type")
			return
		}
	}

	c.logger.Debug("namespace deleted",
		zap.String("namespace", ns.Name))

	ctx := context.Background()
	c.processNamespaceEvent(ctx, ns, EventTypeDeleted)
}

// processNodeEvent finds matching subscriptions and queues webhook notifications.
func (c *SubscriptionController) processNodeEvent(ctx context.Context, node *corev1.Node, eventType EventType) {
	// Track event processing
	EventsProcessedTotal.WithLabelValues("k8s-node", string(eventType)).Inc()

	// Get all subscriptions
	subs, err := c.store.List(ctx)
	if err != nil {
		c.logger.Error("failed to list subscriptions",
			zap.Error(err))
		return
	}

	// Update active subscriptions gauge
	ActiveSubscriptionsGauge.Set(float64(len(subs)))

	// Extract resource pool from node labels
	resourcePoolID := ""
	if poolLabel, ok := node.Labels["resource-pool"]; ok {
		resourcePoolID = poolLabel
	}

	// Find matching subscriptions and queue events
	for _, sub := range subs {
		if c.matchesFilter(sub, "k8s-node", resourcePoolID, node.Name) {
			event := &ResourceEvent{
				SubscriptionID:   sub.ID,
				EventType:        fmt.Sprintf("o2ims.Resource.%s", eventType),
				ObjectRef:        fmt.Sprintf("/o2ims/v1/resources/%s", node.Name),
				ResourceTypeID:   "k8s-node",
				ResourcePoolID:   resourcePoolID,
				GlobalResourceID: node.Name,
				Timestamp:        time.Now(),
				NotificationID:   fmt.Sprintf("notif-%s-%d", node.Name, time.Now().UnixNano()),
				CallbackURL:      sub.Callback,
			}

			if err := c.queueEvent(ctx, event); err != nil {
				c.logger.Error("failed to queue event",
					zap.Error(err),
					zap.String("subscription", sub.ID))
			} else {
				// Track queued event
				EventsQueuedTotal.WithLabelValues(sub.ID, "k8s-node").Inc()
			}
		}
	}
}

// processNamespaceEvent finds matching subscriptions and queues webhook notifications.
func (c *SubscriptionController) processNamespaceEvent(ctx context.Context, ns *corev1.Namespace, eventType EventType) {
	// Track event processing
	EventsProcessedTotal.WithLabelValues("k8s-namespace", string(eventType)).Inc()

	// Get all subscriptions
	subs, err := c.store.List(ctx)
	if err != nil {
		c.logger.Error("failed to list subscriptions",
			zap.Error(err))
		return
	}

	// Update active subscriptions gauge
	ActiveSubscriptionsGauge.Set(float64(len(subs)))

	// Find matching subscriptions and queue events
	for _, sub := range subs {
		if c.matchesFilter(sub, "k8s-namespace", "", ns.Name) {
			event := &ResourceEvent{
				SubscriptionID:   sub.ID,
				EventType:        fmt.Sprintf("o2ims.ResourcePool.%s", eventType),
				ObjectRef:        fmt.Sprintf("/o2ims/v1/resourcePools/%s", ns.Name),
				ResourceTypeID:   "k8s-namespace",
				GlobalResourceID: ns.Name,
				Timestamp:        time.Now(),
				NotificationID:   fmt.Sprintf("notif-%s-%d", ns.Name, time.Now().UnixNano()),
				CallbackURL:      sub.Callback,
			}

			if err := c.queueEvent(ctx, event); err != nil {
				c.logger.Error("failed to queue event",
					zap.Error(err),
					zap.String("subscription", sub.ID))
			} else {
				// Track queued event
				EventsQueuedTotal.WithLabelValues(sub.ID, "k8s-namespace").Inc()
			}
		}
	}
}

// matchesFilter checks if a resource matches the subscription filter.
func (c *SubscriptionController) matchesFilter(
	sub *storage.Subscription,
	resourceTypeID, resourcePoolID, resourceID string,
) bool {
	return sub.Filter.MatchesFilter(resourcePoolID, resourceTypeID, resourceID)
}

// queueEvent adds an event to the Redis Stream for webhook delivery.
func (c *SubscriptionController) queueEvent(ctx context.Context, event *ResourceEvent) error {
	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Add to Redis Stream
	args := &redis.XAddArgs{
		Stream: EventStreamKey,
		MaxLen: MaxStreamLength,
		Approx: true,
		Values: map[string]interface{}{
			"event": string(data),
		},
	}

	if _, err := c.redisClient.XAdd(ctx, args).Result(); err != nil {
		return fmt.Errorf("failed to add event to stream: %w", err)
	}

	c.logger.Debug("event queued",
		zap.String("subscription", event.SubscriptionID),
		zap.String("event_type", event.EventType))

	return nil
}

// GetNodeByName retrieves a Kubernetes node by name (helper for testing).
func (c *SubscriptionController) GetNodeByName(ctx context.Context, name string) (*corev1.Node, error) {
	node, err := c.k8sClient.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("node not found: %s", name)
		}
		return nil, fmt.Errorf("failed to get node: %w", err)
	}
	return node, nil
}
