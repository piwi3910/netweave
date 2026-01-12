package openstack

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/observability"
)

const (
	// defaultPollingInterval is the default polling interval for resource changes.
	defaultPollingInterval = 30 * time.Second

	// defaultWebhookTimeout is the timeout for webhook HTTP requests.
	defaultWebhookTimeout = 10 * time.Second

	// defaultMaxRetries is the maximum number of webhook delivery retries.
	defaultMaxRetries = 3

	// defaultRetryDelay is the base delay between webhook retries.
	defaultRetryDelay = 2 * time.Second
)

// subscriptionState tracks polling state for each subscription.
type subscriptionState struct {
	subscription     *adapter.Subscription
	lastPollTime     time.Time
	resourceSnapshot map[string]string // resourceID -> hash of resource state
	ticker           *time.Ticker
	stopCh           chan struct{}
	wg               sync.WaitGroup
}

// subscriptionStore is a thread-safe in-memory store for subscriptions.
var (
	subscriptionMu      sync.RWMutex
	pollingStateMu      sync.RWMutex
	webhookClientMu     sync.Mutex
	sharedWebhookClient *http.Client
)

// initWebhookClient initializes the shared HTTP client for webhook delivery.
func initWebhookClient() *http.Client {
	webhookClientMu.Lock()
	defer webhookClientMu.Unlock()

	if sharedWebhookClient == nil {
		sharedWebhookClient = &http.Client{
			Timeout: defaultWebhookTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	}
	return sharedWebhookClient
}

// CreateSubscription creates a new event subscription for OpenStack resources.
// It starts a polling goroutine to detect resource changes and send notifications.
func (a *Adapter) CreateSubscription(
	ctx context.Context,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	a.logger.Debug("CreateSubscription called",
		zap.String("callback", sub.Callback))

	if sub.Callback == "" {
		return nil, fmt.Errorf("callback URL is required")
	}

	// Generate subscription ID if not provided
	subscriptionID := sub.SubscriptionID
	if subscriptionID == "" {
		subscriptionID = fmt.Sprintf("openstack-sub-%s", uuid.New().String())
	}

	// Create subscription object
	subscription := &adapter.Subscription{
		SubscriptionID:         subscriptionID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	// Store subscription in memory
	subscriptionMu.Lock()
	a.subscriptions[subscriptionID] = subscription
	subscriptionMu.Unlock()

	// Start polling for this subscription
	if err := a.startPolling(ctx, subscription); err != nil {
		a.logger.Error("failed to start polling",
			zap.String("subscriptionID", subscriptionID),
			zap.Error(err))
		// Clean up subscription on failure
		subscriptionMu.Lock()
		delete(a.subscriptions, subscriptionID)
		subscriptionMu.Unlock()
		return nil, fmt.Errorf("failed to start polling: %w", err)
	}

	a.logger.Info("created subscription with polling",
		zap.String("subscriptionID", subscriptionID),
		zap.String("callback", sub.Callback))

	return subscription, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *Adapter) GetSubscription(_ context.Context, id string) (*adapter.Subscription, error) {
	a.logger.Debug("GetSubscription called",
		zap.String("id", id))

	// Retrieve subscription from memory
	subscriptionMu.RLock()
	subscription, exists := a.subscriptions[id]
	subscriptionMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}

	a.logger.Debug("retrieved subscription",
		zap.String("subscriptionID", subscription.SubscriptionID))

	return subscription, nil
}

// UpdateSubscription updates an existing subscription.
// It stops the old polling goroutine and starts a new one with updated configuration.
func (a *Adapter) UpdateSubscription(
	ctx context.Context,
	id string,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	start := time.Now()
	var err error
	defer func() { adapter.ObserveOperation("openstack", "UpdateSubscription", start, err) }()

	a.logger.Debug("UpdateSubscription called",
		zap.String("id", id),
		zap.String("callback", sub.Callback))

	// Validate callback URL (defense-in-depth: server validates HTTP input, adapter validates programmatic calls)
	if sub.Callback == "" {
		err = fmt.Errorf("callback URL is required")
		return nil, err
	}

	// Check if subscription exists and get existing config
	subscriptionMu.Lock()
	existing, exists := a.subscriptions[id]
	if !exists {
		subscriptionMu.Unlock()
		err = fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
		return nil, err
	}

	// Create updated subscription preserving the ID
	updated := &adapter.Subscription{
		SubscriptionID:         id,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	// Stop old polling goroutine before updating (prevents race with polling reads)
	subscriptionMu.Unlock()
	if stopErr := a.stopPolling(id); stopErr != nil {
		a.logger.Warn("failed to stop old polling",
			zap.String("subscriptionID", id),
			zap.Error(stopErr))
	}

	// Hold lock from here until polling successfully starts to prevent races
	subscriptionMu.Lock()
	defer subscriptionMu.Unlock()

	// Update in memory
	a.subscriptions[id] = updated

	// Start new polling with updated configuration
	if err = a.startPolling(ctx, updated); err != nil {
		a.logger.Error("failed to restart polling",
			zap.String("subscriptionID", id),
			zap.Error(err))

		// Rollback to existing subscription on failure
		a.subscriptions[id] = existing

		// Best-effort attempt to restart old polling
		if restartErr := a.startPolling(ctx, existing); restartErr != nil {
			a.logger.Error("failed to rollback to old subscription",
				zap.String("subscriptionID", id),
				zap.Error(restartErr))
		}

		return nil, fmt.Errorf("failed to restart polling: %w", err)
	}

	a.logger.Info("updated subscription",
		zap.String("subscriptionID", id),
		zap.String("oldCallback", existing.Callback),
		zap.String("newCallback", sub.Callback))

	return updated, nil
}

// DeleteSubscription deletes a subscription by ID and stops its polling goroutine.
func (a *Adapter) DeleteSubscription(_ context.Context, id string) error {
	a.logger.Debug("DeleteSubscription called",
		zap.String("id", id))

	// Remove subscription from memory
	subscriptionMu.Lock()
	_, exists := a.subscriptions[id]
	if !exists {
		subscriptionMu.Unlock()
		return fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}
	delete(a.subscriptions, id)
	subscriptionMu.Unlock()

	// Stop polling for this subscription
	if err := a.stopPolling(id); err != nil {
		a.logger.Warn("failed to stop polling",
			zap.String("subscriptionID", id),
			zap.Error(err))
	}

	a.logger.Info("deleted subscription",
		zap.String("subscriptionID", id))

	return nil
}

// ListSubscriptions retrieves all active subscriptions.
func (a *Adapter) ListSubscriptions(_ context.Context) ([]*adapter.Subscription, error) {
	a.logger.Debug("ListSubscriptions called")

	subscriptionMu.RLock()
	subscriptions := make([]*adapter.Subscription, 0, len(a.subscriptions))
	for _, sub := range a.subscriptions {
		subscriptions = append(subscriptions, sub)
	}
	subscriptionMu.RUnlock()

	a.logger.Debug("listed subscriptions",
		zap.Int("count", len(subscriptions)))

	return subscriptions, nil
}

// startPolling starts the polling goroutine for a subscription.
func (a *Adapter) startPolling(ctx context.Context, sub *adapter.Subscription) error {
	pollingStateMu.Lock()
	defer pollingStateMu.Unlock()

	// Initialize polling states map if needed
	if a.pollingStates == nil {
		a.pollingStates = make(map[string]*subscriptionState)
	}

	// Check if already polling
	if _, exists := a.pollingStates[sub.SubscriptionID]; exists {
		return fmt.Errorf("subscription already polling: %s", sub.SubscriptionID)
	}

	// Create initial resource snapshot
	snapshot, err := a.createResourceSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to create initial snapshot: %w", err)
	}

	// Create polling state
	state := &subscriptionState{
		subscription:     sub,
		lastPollTime:     time.Now(),
		resourceSnapshot: snapshot,
		ticker:           time.NewTicker(defaultPollingInterval),
		stopCh:           make(chan struct{}),
	}

	a.pollingStates[sub.SubscriptionID] = state

	// Start polling goroutine
	state.wg.Add(1)
	go a.pollResourceChanges(ctx, state)

	return nil
}

// stopPolling stops the polling goroutine for a subscription.
func (a *Adapter) stopPolling(subscriptionID string) error {
	pollingStateMu.Lock()
	state, exists := a.pollingStates[subscriptionID]
	if !exists {
		pollingStateMu.Unlock()
		return fmt.Errorf("no polling state found for subscription: %s", subscriptionID)
	}
	delete(a.pollingStates, subscriptionID)
	pollingStateMu.Unlock()

	// Signal stop and wait for goroutine
	close(state.stopCh)
	state.ticker.Stop()
	state.wg.Wait()

	return nil
}

// pollResourceChanges runs the polling loop for a subscription.
func (a *Adapter) pollResourceChanges(ctx context.Context, state *subscriptionState) {
	defer state.wg.Done()

	a.logger.Info("started polling for subscription",
		zap.String("subscriptionID", state.subscription.SubscriptionID),
		zap.Duration("interval", defaultPollingInterval))

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("context canceled, stopping polling",
				zap.String("subscriptionID", state.subscription.SubscriptionID))
			return

		case <-state.stopCh:
			a.logger.Info("stopped polling for subscription",
				zap.String("subscriptionID", state.subscription.SubscriptionID))
			return

		case <-state.ticker.C:
			if err := a.detectAndNotifyChanges(ctx, state); err != nil {
				a.logger.Error("error detecting changes",
					zap.String("subscriptionID", state.subscription.SubscriptionID),
					zap.Error(err))
			}
			state.lastPollTime = time.Now()
		}
	}
}

// detectAndNotifyChanges detects resource changes and sends notifications.
func (a *Adapter) detectAndNotifyChanges(ctx context.Context, state *subscriptionState) error {
	// Create new snapshot
	newSnapshot, err := a.createResourceSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Detect changes
	changes := a.detectChanges(state.resourceSnapshot, newSnapshot)

	// Process each change
	for _, change := range changes {
		if a.matchesFilter(state.subscription, change) {
			if err := a.sendWebhookNotification(ctx, state.subscription, change); err != nil {
				a.logger.Error("failed to send webhook notification",
					zap.String("subscriptionID", state.subscription.SubscriptionID),
					zap.String("resourceID", change.ResourceID),
					zap.String("eventType", change.EventType),
					zap.Error(err))
			}
		}
	}

	// Update snapshot
	state.resourceSnapshot = newSnapshot

	return nil
}

// resourceChange represents a detected change in resource state.
type resourceChange struct {
	EventType  string
	ResourceID string
	Resource   *adapter.Resource
}

// createResourceSnapshot creates a snapshot of current OpenStack resources.
func (a *Adapter) createResourceSnapshot(_ context.Context) (map[string]string, error) {
	snapshot := make(map[string]string)

	// Skip if compute client is not initialized (e.g., in tests)
	if a.compute == nil {
		return snapshot, nil
	}

	// Query servers (resources)
	allPages, err := servers.List(a.compute, servers.ListOpts{}).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	serverList, err := servers.ExtractServers(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract servers: %w", err)
	}

	// Create hash for each server
	for _, server := range serverList {
		resourceID := generateServerResourceID(&server)
		hash := computeResourceHash(&server)
		snapshot[resourceID] = hash
	}

	return snapshot, nil
}

// detectChanges compares snapshots and returns detected changes.
func (a *Adapter) detectChanges(oldSnapshot, newSnapshot map[string]string) []resourceChange {
	var changes []resourceChange

	// Detect created and updated resources
	for resourceID, newHash := range newSnapshot {
		oldHash, existed := oldSnapshot[resourceID]
		if !existed {
			// Resource created
			changes = append(changes, resourceChange{
				EventType:  string(models.EventTypeResourceCreated),
				ResourceID: resourceID,
			})
		} else if oldHash != newHash {
			// Resource updated
			changes = append(changes, resourceChange{
				EventType:  string(models.EventTypeResourceUpdated),
				ResourceID: resourceID,
			})
		}
	}

	// Detect deleted resources
	for resourceID := range oldSnapshot {
		if _, exists := newSnapshot[resourceID]; !exists {
			// Resource deleted
			changes = append(changes, resourceChange{
				EventType:  string(models.EventTypeResourceDeleted),
				ResourceID: resourceID,
			})
		}
	}

	return changes
}

// matchesFilter checks if a resource change matches the subscription filter.
func (a *Adapter) matchesFilter(sub *adapter.Subscription, change resourceChange) bool {
	// If no filter, match all changes
	if sub.Filter == nil {
		return true
	}

	// Check resource ID filter
	if sub.Filter.ResourceID != "" && sub.Filter.ResourceID != change.ResourceID {
		return false
	}

	// Check resource type filter
	if sub.Filter.ResourceTypeID != "" {
		// For OpenStack, we'd need to fetch the resource and check its type
		// For now, we'll match all if ResourceTypeID is specified
		return true
	}

	return true
}

// sendWebhookNotification sends a webhook notification for a resource change.
func (a *Adapter) sendWebhookNotification(
	ctx context.Context,
	sub *adapter.Subscription,
	change resourceChange,
) error {
	// Fetch current resource details if it still exists
	var resourceData any
	if change.EventType != string(models.EventTypeResourceDeleted) {
		resource, err := a.getResourceDetails(ctx, change.ResourceID)
		if err != nil {
			a.logger.Warn("failed to fetch resource details",
				zap.String("resourceID", change.ResourceID),
				zap.Error(err))
			resourceData = map[string]string{"resourceId": change.ResourceID}
		} else {
			resourceData = resource
		}
	} else {
		resourceData = map[string]string{"resourceId": change.ResourceID}
	}

	// Create notification payload
	notification := &models.Notification{
		SubscriptionID:         sub.SubscriptionID,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		EventType:              change.EventType,
		Resource:               resourceData,
		Timestamp:              time.Now(),
	}

	// Serialize to JSON
	payload, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Send with retries
	return a.deliverWebhookWithRetries(ctx, sub.Callback, payload)
}

// deliverWebhookWithRetries delivers a webhook with exponential backoff retries.
func (a *Adapter) deliverWebhookWithRetries(
	ctx context.Context,
	callbackURL string,
	payload []byte,
) error {
	client := initWebhookClient()

	var lastErr error
	for attempt := 0; attempt <= defaultMaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2s, 4s, 8s
			backoffMultiplier := 1 << (attempt - 1)
			delay := defaultRetryDelay * time.Duration(backoffMultiplier)
			select {
			case <-ctx.Done():
				return fmt.Errorf("context canceled during retry: %w", ctx.Err())
			case <-time.After(delay):
			}

			a.logger.Debug("retrying webhook delivery",
				zap.String("callback", callbackURL),
				zap.Int("attempt", attempt))
		}

		startTime := time.Now()
		statusCode, err := a.deliverWebhook(ctx, client, callbackURL, payload)
		duration := time.Since(startTime)

		// Record metrics
		metrics := observability.GetMetrics()
		metrics.RecordWebhookDelivery(duration, statusCode, err)

		if err == nil && statusCode >= 200 && statusCode < 300 {
			a.logger.Debug("webhook delivered successfully",
				zap.String("callback", callbackURL),
				zap.Int("statusCode", statusCode),
				zap.Duration("duration", duration))
			return nil
		}

		lastErr = err
		if err != nil {
			a.logger.Warn("webhook delivery failed",
				zap.String("callback", callbackURL),
				zap.Int("attempt", attempt),
				zap.Error(err))
		} else {
			a.logger.Warn("webhook returned non-2xx status",
				zap.String("callback", callbackURL),
				zap.Int("statusCode", statusCode),
				zap.Int("attempt", attempt))
			lastErr = fmt.Errorf("HTTP %d", statusCode)
		}
	}

	return fmt.Errorf("webhook delivery failed after %d attempts: %w", defaultMaxRetries+1, lastErr)
}

// deliverWebhook performs a single webhook delivery attempt.
func (a *Adapter) deliverWebhook(
	ctx context.Context,
	client *http.Client,
	callbackURL string,
	payload []byte,
) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "o2ims-gateway/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			a.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	return resp.StatusCode, nil
}

// getResourceDetails fetches detailed information about a resource.
func (a *Adapter) getResourceDetails(ctx context.Context, resourceID string) (*adapter.Resource, error) {
	// Extract server UUID from resourceID (format: openstack-server-{uuid})
	// For now, use GetResource if available
	return a.GetResource(ctx, resourceID)
}

// computeResourceHash computes a hash of a resource's state.
func computeResourceHash(resource any) string {
	// Serialize resource to JSON
	data, err := json.Marshal(resource)
	if err != nil {
		return ""
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// generateServerResourceID generates a consistent resource ID for a server.
func generateServerResourceID(server *servers.Server) string {
	return fmt.Sprintf("openstack-server-%s", server.ID)
}

// StopAllPolling stops all active polling goroutines (called during shutdown).
func (a *Adapter) StopAllPolling() {
	pollingStateMu.Lock()
	states := make([]*subscriptionState, 0, len(a.pollingStates))
	for _, state := range a.pollingStates {
		states = append(states, state)
	}
	pollingStateMu.Unlock()

	a.logger.Info("stopping all polling goroutines",
		zap.Int("count", len(states)))

	// Stop all polling goroutines
	for _, state := range states {
		close(state.stopCh)
		state.ticker.Stop()
	}

	// Wait for all to finish
	for _, state := range states {
		state.wg.Wait()
	}

	a.logger.Info("all polling goroutines stopped")
}
