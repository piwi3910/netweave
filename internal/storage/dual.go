package storage

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// DualStore implements Store by writing to both a primary and secondary store.
// Reads always come from the primary store. Write errors on the secondary
// are logged but do not cause the operation to fail, allowing safe migration
// from one backend to another while keeping both stores in sync on a
// best-effort basis.
//
// Example usage during migration from Redis to PostgreSQL:
//
//	primary := storage.NewRedisStore(redisCfg)
//	secondary := storage.NewPostgresStore(pgDB, true)
//	dual := storage.NewDualStore(primary, secondary, logger)
//	// All writes go to both; reads from Redis only
type DualStore struct {
	primary   Store
	secondary Store
	logger    *zap.Logger
}

// NewDualStore creates a new DualStore that writes to both stores and reads
// from the primary. Secondary write errors are logged as warnings.
func NewDualStore(primary, secondary Store, logger *zap.Logger) *DualStore {
	if primary == nil {
		panic("primary store cannot be nil")
	}
	if secondary == nil {
		panic("secondary store cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}
	return &DualStore{
		primary:   primary,
		secondary: secondary,
		logger:    logger,
	}
}

// Create creates a subscription in both the primary and secondary stores.
// If the primary write fails, the error is returned immediately.
// If the secondary write fails, the error is logged but not returned.
func (d *DualStore) Create(ctx context.Context, sub *Subscription) error {
	if err := d.primary.Create(ctx, sub); err != nil {
		return fmt.Errorf("primary store create: %w", err)
	}
	if err := d.secondary.Create(ctx, sub); err != nil {
		d.logger.Warn("secondary store create failed",
			zap.String("subscriptionID", sub.ID),
			zap.Error(err),
		)
	}
	return nil
}

// Get retrieves a subscription from the primary store.
func (d *DualStore) Get(ctx context.Context, id string) (*Subscription, error) {
	return d.primary.Get(ctx, id)
}

// Update updates a subscription in both stores.
// If the primary write fails, the error is returned immediately.
// If the secondary write fails, the error is logged but not returned.
func (d *DualStore) Update(ctx context.Context, sub *Subscription) error {
	if err := d.primary.Update(ctx, sub); err != nil {
		return fmt.Errorf("primary store update: %w", err)
	}
	if err := d.secondary.Update(ctx, sub); err != nil {
		d.logger.Warn("secondary store update failed",
			zap.String("subscriptionID", sub.ID),
			zap.Error(err),
		)
	}
	return nil
}

// Delete deletes a subscription from both stores.
// If the primary delete fails, the error is returned immediately.
// If the secondary delete fails, the error is logged but not returned.
func (d *DualStore) Delete(ctx context.Context, id string) error {
	if err := d.primary.Delete(ctx, id); err != nil {
		return fmt.Errorf("primary store delete: %w", err)
	}
	if err := d.secondary.Delete(ctx, id); err != nil {
		d.logger.Warn("secondary store delete failed",
			zap.String("subscriptionID", id),
			zap.Error(err),
		)
	}
	return nil
}

// List retrieves all subscriptions from the primary store.
func (d *DualStore) List(ctx context.Context) ([]*Subscription, error) {
	return d.primary.List(ctx)
}

// ListByResourcePool retrieves subscriptions filtered by resource pool ID from the primary store.
func (d *DualStore) ListByResourcePool(ctx context.Context, resourcePoolID string) ([]*Subscription, error) {
	return d.primary.ListByResourcePool(ctx, resourcePoolID)
}

// ListByResourceType retrieves subscriptions filtered by resource type ID from the primary store.
func (d *DualStore) ListByResourceType(ctx context.Context, resourceTypeID string) ([]*Subscription, error) {
	return d.primary.ListByResourceType(ctx, resourceTypeID)
}

// ListByTenant retrieves subscriptions filtered by tenant ID from the primary store.
func (d *DualStore) ListByTenant(ctx context.Context, tenantID string) ([]*Subscription, error) {
	return d.primary.ListByTenant(ctx, tenantID)
}

// Close closes both the primary and secondary stores.
// Errors from both are aggregated.
func (d *DualStore) Close() error {
	primaryErr := d.primary.Close()
	secondaryErr := d.secondary.Close()
	if primaryErr != nil && secondaryErr != nil {
		return fmt.Errorf("primary close: %w; secondary close: %w", primaryErr, secondaryErr)
	}
	if primaryErr != nil {
		return fmt.Errorf("primary close: %w", primaryErr)
	}
	if secondaryErr != nil {
		return fmt.Errorf("secondary close: %w", secondaryErr)
	}
	return nil
}

// Ping pings both stores and returns an error if either is unavailable.
func (d *DualStore) Ping(ctx context.Context) error {
	if err := d.primary.Ping(ctx); err != nil {
		return fmt.Errorf("primary store ping: %w", err)
	}
	if err := d.secondary.Ping(ctx); err != nil {
		return fmt.Errorf("secondary store ping: %w", err)
	}
	return nil
}
