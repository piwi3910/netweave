package starlingx

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/storage"
	"go.uber.org/zap"
)

// CreateSubscription creates a new event subscription.
func (a *Adapter) CreateSubscription(ctx context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	if a.store == nil {
		return nil, adapter.ErrNotImplemented
	}

	// Generate subscription ID if not provided
	if sub.SubscriptionID == "" {
		sub.SubscriptionID = uuid.New().String()
	}

	// Convert to storage subscription
	storageSub := convertToStorageSubscription(sub)

	// Store subscription
	if err := a.store.Create(ctx, storageSub); err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	a.logger.Info("created subscription",
		zap.String("subscription_id", sub.SubscriptionID),
		zap.String("callback", sub.Callback),
	)

	return sub, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *Adapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	if a.store == nil {
		return nil, adapter.ErrNotImplemented
	}

	storageSub, err := a.store.Get(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			return nil, adapter.ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	sub := convertToAdapterSubscription(storageSub)

	a.logger.Debug("retrieved subscription",
		zap.String("subscription_id", id),
	)

	return sub, nil
}

// UpdateSubscription updates an existing subscription.
func (a *Adapter) UpdateSubscription(ctx context.Context, id string, sub *adapter.Subscription) (*adapter.Subscription, error) {
	if a.store == nil {
		return nil, adapter.ErrNotImplemented
	}

	// Verify subscription exists
	existing, err := a.store.Get(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			return nil, adapter.ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	// Update fields
	if sub.Callback != "" {
		existing.Callback = sub.Callback
	}
	if sub.ConsumerSubscriptionID != "" {
		existing.ConsumerSubscriptionID = sub.ConsumerSubscriptionID
	}
	if sub.Filter != nil {
		existing.Filter = convertSubscriptionFilterToStorage(sub.Filter)
	}

	// Update in store
	if err := a.store.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	updatedSub := convertToAdapterSubscription(existing)

	a.logger.Info("updated subscription",
		zap.String("subscription_id", id),
	)

	return updatedSub, nil
}

// DeleteSubscription deletes a subscription by ID.
func (a *Adapter) DeleteSubscription(ctx context.Context, id string) error {
	if a.store == nil {
		return adapter.ErrNotImplemented
	}

	if err := a.store.Delete(ctx, id); err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			return adapter.ErrSubscriptionNotFound
		}
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	a.logger.Info("deleted subscription",
		zap.String("subscription_id", id),
	)

	return nil
}

// Conversion helpers

func convertToStorageSubscription(sub *adapter.Subscription) *storage.Subscription {
	storageSub := &storage.Subscription{
		ID:                     sub.SubscriptionID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
	}

	if sub.Filter != nil {
		storageSub.Filter = convertSubscriptionFilterToStorage(sub.Filter)
	}

	return storageSub
}

func convertSubscriptionFilterToStorage(filter *adapter.SubscriptionFilter) storage.SubscriptionFilter {
	if filter == nil {
		return storage.SubscriptionFilter{}
	}

	return storage.SubscriptionFilter{
		ResourcePoolID: filter.ResourcePoolID,
		ResourceTypeID: filter.ResourceTypeID,
		ResourceID:     filter.ResourceID,
	}
}

func convertToAdapterSubscription(storageSub *storage.Subscription) *adapter.Subscription {
	sub := &adapter.Subscription{
		SubscriptionID:         storageSub.ID,
		Callback:               storageSub.Callback,
		ConsumerSubscriptionID: storageSub.ConsumerSubscriptionID,
	}

	// Convert filter if any field is set
	if storageSub.Filter.ResourcePoolID != "" || storageSub.Filter.ResourceTypeID != "" || storageSub.Filter.ResourceID != "" {
		sub.Filter = &adapter.SubscriptionFilter{
			ResourcePoolID: storageSub.Filter.ResourcePoolID,
			ResourceTypeID: storageSub.Filter.ResourceTypeID,
			ResourceID:     storageSub.Filter.ResourceID,
		}
	}

	return sub
}
