package openstack

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	"github.com/piwi3910/netweave/internal/security/callbackurl"
	"github.com/piwi3910/netweave/internal/security/urlredact"
	"github.com/piwi3910/netweave/internal/webhook"
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

// SubscriptionState tracks polling state for each subscription.
type SubscriptionState struct {
	subscription     *adapter.Subscription
	lastPollTime     time.Time
	resourceSnapshot map[string]string // resourceID -> hash of resource state
	ticker           *time.Ticker
	stopCh           chan struct{}
	wg               sync.WaitGroup
}

// Global state for polling machinery. Subscription CRUD is handled by the
// shared adapter.InMemorySubscriptionStore on a.subs.
var (
	pollingStateMu sync.RWMutex
)

// validateCallback runs the shared callback-URL SSRF validator, honoring the
// adapter's test-only allow-private-networks escape hatch.
func (a *Adapter) validateCallback(ctx context.Context, rawURL string) error {
	return callbackurl.Validate(ctx, rawURL, callbackurl.Options{
		AllowPrivateNetworks: a.allowPrivateWebhookTargets,
	})
}

// initWebhookClient returns (and lazily constructs) the per-adapter HTTP
// client for webhook delivery. The client uses the SSRF-safe transport from
// internal/webhook: every connect re-resolves the hostname and refuses to
// dial any IP in the banned set (loopback, RFC1918 private space, link-local
// including cloud metadata 169.254.169.254, CGNAT, multicast, IPv6
// equivalents). A construction failure is logged and a best-effort minimal
// client is returned so polling does not panic; the delivery-time
// callbackurl.Validate call still rejects SSRF targets before the request is
// dispatched.
func (a *Adapter) initWebhookClient() *http.Client {
	a.webhookClientMu.Lock()
	defer a.webhookClientMu.Unlock()

	if a.webhookClient != nil {
		return a.webhookClient
	}

	client, err := webhook.NewHTTPClient(&webhook.ClientConfig{
		Timeout:              defaultWebhookTimeout,
		AllowPrivateNetworks: a.allowPrivateWebhookTargets,
	})
	if err != nil {
		a.logger.Error("failed to construct SSRF-safe webhook client, falling back to minimal client",
			zap.Error(err))
		client = &http.Client{Timeout: defaultWebhookTimeout}
	}
	a.webhookClient = client
	return a.webhookClient
}

// CreateSubscription creates a new event subscription for OpenStack resources.
// It starts a polling goroutine to detect resource changes and send notifications.
func (a *Adapter) CreateSubscription(
	ctx context.Context,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	a.logger.Debug("CreateSubscription called",
		zap.String("callback", urlredact.Redact(sub.Callback)))

	if err := a.validateCallback(ctx, sub.Callback); err != nil {
		return nil, fmt.Errorf("invalid callback URL: %w", err)
	}

	subscriptionID := sub.SubscriptionID
	if subscriptionID == "" {
		subscriptionID = fmt.Sprintf("openstack-sub-%s", uuid.New().String())
	}

	subscription := &adapter.Subscription{
		SubscriptionID:         subscriptionID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	if err := a.subs.Create(ctx, subscription); err != nil {
		return nil, err
	}

	if err := a.startPolling(ctx, subscription); err != nil {
		a.logger.Error("failed to start polling",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err))
		// Roll back the subscription on polling-start failure.
		if deleteErr := a.subs.Delete(context.WithoutCancel(ctx), subscriptionID); deleteErr != nil {
			a.logger.Warn("failed to roll back subscription after polling start failure",
				zap.String("subscription_id", subscriptionID),
				zap.Error(deleteErr))
		}
		return nil, fmt.Errorf("failed to start polling: %w", err)
	}

	a.logger.Info("created subscription with polling",
		zap.String("subscriptionID", subscriptionID),
		zap.String("callback", urlredact.Redact(sub.Callback)))

	return subscription, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *Adapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	a.logger.Debug("GetSubscription called", zap.String("id", id))

	subscription, err := a.subs.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	a.logger.Debug("retrieved subscription",
		zap.String("subscription_id", subscription.SubscriptionID))
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
		zap.String("callback", urlredact.Redact(sub.Callback)))

	if validateErr := a.validateCallback(ctx, sub.Callback); validateErr != nil {
		err = fmt.Errorf("invalid callback URL: %w", validateErr)
		return nil, err
	}

	existing, err := a.subs.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	updated := &adapter.Subscription{
		SubscriptionID:         id,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	// Stop old polling goroutine before updating (prevents races with polling reads).
	if stopErr := a.stopPolling(id); stopErr != nil {
		a.logger.Warn("failed to stop old polling",
			zap.String("subscription_id", id),
			zap.Error(stopErr))
	}

	if err = a.subs.Update(ctx, id, updated); err != nil {
		return nil, err
	}

	// Start new polling with updated configuration.
	if err = a.startPolling(ctx, updated); err != nil {
		a.logger.Error("failed to restart polling",
			zap.String("subscription_id", id),
			zap.Error(err))

		// Rollback to existing subscription on failure.
		if rollbackErr := a.subs.Update(context.WithoutCancel(ctx), id, existing); rollbackErr != nil {
			a.logger.Error("failed to rollback subscription record",
				zap.String("subscription_id", id),
				zap.Error(rollbackErr))
		}
		// Best-effort attempt to restart old polling.
		if restartErr := a.startPolling(ctx, existing); restartErr != nil {
			a.logger.Error("failed to rollback to old subscription",
				zap.String("subscription_id", id),
				zap.Error(restartErr))
		}

		return nil, fmt.Errorf("failed to restart polling: %w", err)
	}

	a.logger.Info("updated subscription",
		zap.String("subscription_id", id),
		zap.String("old_callback", existing.Callback),
		zap.String("new_callback", sub.Callback))

	return updated, nil
}

// DeleteSubscription deletes a subscription by ID and stops its polling goroutine.
func (a *Adapter) DeleteSubscription(ctx context.Context, id string) error {
	a.logger.Debug("DeleteSubscription called", zap.String("id", id))

	if err := a.subs.Delete(ctx, id); err != nil {
		return err
	}

	if err := a.stopPolling(id); err != nil {
		a.logger.Warn("failed to stop polling",
			zap.String("subscription_id", id),
			zap.Error(err))
	}

	a.logger.Info("deleted subscription", zap.String("subscription_id", id))
	return nil
}

// ListSubscriptions retrieves all active subscriptions.
func (a *Adapter) ListSubscriptions(ctx context.Context) ([]*adapter.Subscription, error) {
	a.logger.Debug("ListSubscriptions called")

	subscriptions, err := a.subs.List(ctx, nil)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		a.logger.Warn("listing subscriptions returned unexpected error", zap.Error(err))
		return nil, err
	}

	a.logger.Debug("listed subscriptions", zap.Int("count", len(subscriptions)))
	return subscriptions, nil
}

// startPolling starts the polling goroutine for a subscription.
func (a *Adapter) startPolling(ctx context.Context, sub *adapter.Subscription) error {
	pollingStateMu.Lock()
	defer pollingStateMu.Unlock()

	// Initialize polling states map if needed
	if a.pollingStates == nil {
		a.pollingStates = make(map[string]*SubscriptionState)
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
	state := &SubscriptionState{
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
func (a *Adapter) pollResourceChanges(ctx context.Context, state *SubscriptionState) {
	defer state.wg.Done()

	a.logger.Info("started polling for subscription",
		zap.String("subscription_id", state.subscription.SubscriptionID),
		zap.Duration("interval", defaultPollingInterval))

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("context canceled, stopping polling",
				zap.String("subscription_id", state.subscription.SubscriptionID))
			return

		case <-state.stopCh:
			a.logger.Info("stopped polling for subscription",
				zap.String("subscription_id", state.subscription.SubscriptionID))
			return

		case <-state.ticker.C:
			if err := a.detectAndNotifyChanges(ctx, state); err != nil {
				a.logger.Error("error detecting changes",
					zap.String("subscription_id", state.subscription.SubscriptionID),
					zap.Error(err))
			}
			state.lastPollTime = time.Now()
		}
	}
}

// detectAndNotifyChanges detects resource changes and sends notifications.
func (a *Adapter) detectAndNotifyChanges(ctx context.Context, state *SubscriptionState) error {
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
			if webhookErr := a.sendWebhookNotification(ctx, state.subscription, change); webhookErr != nil {
				a.logger.Error("failed to send webhook notification",
					zap.String("subscription_id", state.subscription.SubscriptionID),
					zap.String("resource_id", change.ResourceID),
					zap.String("event_type", change.EventType),
					zap.Error(webhookErr))
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
				zap.String("resource_id", change.ResourceID),
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
	// Defense in depth: re-validate the stored callback URL at delivery time.
	// The subscription store could contain a previously valid URL that now
	// resolves to an internal IP (DNS drift, attacker-controlled record, or
	// restore of an old subscription). Rejecting here before the HTTP request
	// is built closes the CodeQL "uncontrolled data used in network request"
	// finding at the source. The SSRF-safe DialContext on the shared webhook
	// client provides the additional connect-time guarantee.
	if err := a.validateCallback(ctx, callbackURL); err != nil {
		return fmt.Errorf("refusing to deliver webhook: %w", err)
	}

	client := a.initWebhookClient()

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
				zap.String("callback", urlredact.Redact(callbackURL)),
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
				zap.String("callback", urlredact.Redact(callbackURL)),
				zap.Int("statusCode", statusCode),
				zap.Duration("duration", duration))
			return nil
		}

		lastErr = err
		if err != nil {
			a.logger.Warn("webhook delivery failed",
				zap.String("callback", urlredact.Redact(callbackURL)),
				zap.Int("attempt", attempt),
				zap.Error(err))
		} else {
			a.logger.Warn("webhook returned non-2xx status",
				zap.String("callback", urlredact.Redact(callbackURL)),
				zap.Int("statusCode", statusCode),
				zap.Int("attempt", attempt))
			lastErr = fmt.Errorf("HTTP %d", statusCode)
		}
	}

	return fmt.Errorf("webhook delivery failed after %d attempts: %w", defaultMaxRetries+1, lastErr)
}

// deliverWebhook performs a single webhook delivery attempt.
//
// The caller guarantees callbackURL has already been validated by
// callbackurl.Validate, and the *http.Client is built with
// webhook.NewHTTPClient, whose DialContext re-resolves the hostname at
// connect time and refuses to dial any IP in the SSRF-banned set. Both
// guarantees are prerequisites for the security posture of this function.
func (a *Adapter) deliverWebhook(
	ctx context.Context,
	client *http.Client,
	callbackURL string,
	payload []byte,
) (int, error) {
	// Re-validate immediately before building the request to keep the
	// tainted-data flow short and analyzable by SAST tooling. The returned
	// *url.URL is the validated, trusted form of the caller-supplied string.
	parsed, err := callbackurl.ValidateAndParse(ctx, callbackURL, callbackurl.Options{
		AllowPrivateNetworks: a.allowPrivateWebhookTargets,
	})
	if err != nil {
		return 0, fmt.Errorf("refusing to deliver webhook: %w", err)
	}

	// Build the request from the parsed URL so the taint flow terminates at
	// the validation boundary above.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.URL = parsed

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
	states := make([]*SubscriptionState, 0, len(a.pollingStates))
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
