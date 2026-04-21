package gcp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/security/urlredact"
)

// adapterName is used for metrics labels.
const adapterName = "gcp"

// CreateSubscription creates a new event subscription.
// GCP adapter uses polling-based subscriptions since Pub/Sub
// integration would require additional GCP infrastructure setup.
func (a *Adapter) CreateSubscription(ctx context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	start := time.Now()
	var err error
	defer func() { adapter.ObserveOperation(adapterName, "CreateSubscription", start, err) }()

	a.logger.Debug("CreateSubscription called",
		zap.String("callback", urlredact.Redact(sub.Callback)))

	if sub.Callback == "" {
		err = fmt.Errorf("callback URL is required")
		return nil, err
	}

	subscriptionID := sub.SubscriptionID
	if subscriptionID == "" {
		subscriptionID = fmt.Sprintf("gcp-sub-%s", uuid.New().String())
	}

	newSub := &adapter.Subscription{
		SubscriptionID:         subscriptionID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	if err = a.subs.Create(ctx, newSub); err != nil {
		return nil, err
	}

	adapter.UpdateSubscriptionCount(adapterName, a.subs.Len())

	a.logger.Info("created subscription",
		zap.String("subscriptionId", subscriptionID),
		zap.String("callback", urlredact.Redact(sub.Callback)))

	return newSub, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *Adapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	start := time.Now()
	var err error
	defer func() { adapter.ObserveOperation(adapterName, "GetSubscription", start, err) }()

	a.logger.Debug("GetSubscription called", zap.String("id", id))

	sub, err := a.subs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// UpdateSubscription updates an existing subscription.
func (a *Adapter) UpdateSubscription(
	ctx context.Context,
	id string,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	start := time.Now()
	var err error
	defer func() { adapter.ObserveOperation(adapterName, "UpdateSubscription", start, err) }()

	a.logger.Debug("UpdateSubscription called",
		zap.String("id", id),
		zap.String("callback", urlredact.Redact(sub.Callback)))

	if sub.Callback == "" {
		err = fmt.Errorf("callback URL is required")
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

	if err = a.subs.Update(ctx, id, updated); err != nil {
		return nil, err
	}

	a.logger.Info("updated subscription",
		zap.String("subscription_id", id),
		zap.String("old_callback", existing.Callback),
		zap.String("new_callback", sub.Callback))

	return updated, nil
}

// DeleteSubscription deletes a subscription by ID.
func (a *Adapter) DeleteSubscription(ctx context.Context, id string) error {
	start := time.Now()
	var err error
	defer func() { adapter.ObserveOperation(adapterName, "DeleteSubscription", start, err) }()

	a.logger.Debug("DeleteSubscription called", zap.String("id", id))

	if err = a.subs.Delete(ctx, id); err != nil {
		return err
	}

	adapter.UpdateSubscriptionCount(adapterName, a.subs.Len())

	a.logger.Info("deleted subscription", zap.String("subscription_id", id))
	return nil
}

// ListSubscriptions returns all active subscriptions.
// This is a helper method not part of the Adapter interface.
func (a *Adapter) ListSubscriptions() []*adapter.Subscription {
	return a.subs.Snapshot()
}
