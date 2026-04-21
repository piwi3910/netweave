package storage

import (
	"context"
	"fmt"

	"github.com/piwi3910/netweave/internal/tenant"
)

// TenantEnforcedStore wraps a Store and enforces tenant isolation at every
// read/write. The wrapper extracts the tenant identity from ctx (using the
// internal/tenant package) and:
//
//   - rejects Create/Update/Delete with tenant.ErrMissingTenant when ctx
//     carries no tenant;
//   - filters Get/List/ListByResourcePool/ListByResourceType by tenant,
//     returning ErrSubscriptionNotFound for cross-tenant reads so the
//     caller cannot use response variance as a cross-tenant oracle;
//   - honors tenant.TenantAll as a platform-admin bypass that skips
//     tenant filtering (the ONLY supported bypass mechanism).
//
// This decorator is a defense-in-depth layer: handlers should still put
// tenant into ctx via middleware, but if a handler author forgets to add
// an inline tenant check, the storage layer makes cross-tenant reads
// physically impossible.
//
// TenantEnforcedStore is safe for concurrent use when the underlying
// Store is safe for concurrent use.
type TenantEnforcedStore struct {
	inner Store
}

// NewTenantEnforcedStore wraps an inner Store so that every operation
// requires a tenant identity in ctx (or tenant.TenantAll for admin bypass).
func NewTenantEnforcedStore(inner Store) *TenantEnforcedStore {
	return &TenantEnforcedStore{inner: inner}
}

// Inner returns the wrapped Store. Intended for tests and migration code
// that must bypass enforcement with an explicit platform-admin context.
func (s *TenantEnforcedStore) Inner() Store {
	return s.inner
}

// Create creates a subscription after verifying that the subscription's
// TenantID is consistent with the tenant in ctx. When ctx carries
// tenant.TenantAll, the subscription's TenantID is used as-is.
func (s *TenantEnforcedStore) Create(ctx context.Context, sub *Subscription) error {
	tid, err := tenant.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("storage.Create: %w", err)
	}
	if sub == nil {
		return ErrInvalidID
	}
	if !tenant.IsAll(tid) {
		// Stamp or verify tenant id. Never trust the caller's sub.TenantID
		// to be correct; overwrite it with the ctx tenant.
		sub.TenantID = tid
	} else if sub.TenantID == "" {
		// Platform-admin create must still carry a tenant id because
		// subscriptions belong to a tenant at rest.
		return fmt.Errorf("storage.Create with all-tenants ctx requires explicit TenantID: %w", tenant.ErrMissingTenant)
	}
	return s.inner.Create(ctx, sub)
}

// Get retrieves a subscription only if it belongs to the tenant in ctx
// (or ctx carries tenant.TenantAll). Cross-tenant reads return
// ErrSubscriptionNotFound.
func (s *TenantEnforcedStore) Get(ctx context.Context, id string) (*Subscription, error) {
	tid, err := tenant.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.Get: %w", err)
	}
	sub, err := s.inner.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !tenant.IsAll(tid) && sub.TenantID != tid {
		return nil, ErrSubscriptionNotFound
	}
	return sub, nil
}

// Update updates a subscription only if it belongs to the tenant in ctx
// (or ctx carries tenant.TenantAll). The caller-supplied sub.TenantID is
// overwritten with the ctx tenant to prevent a malicious caller from
// re-homing a subscription into another tenant.
func (s *TenantEnforcedStore) Update(ctx context.Context, sub *Subscription) error {
	tid, err := tenant.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("storage.Update: %w", err)
	}
	if sub == nil {
		return ErrInvalidID
	}
	// Load existing to check ownership.
	existing, err := s.inner.Get(ctx, sub.ID)
	if err != nil {
		return err
	}
	if !tenant.IsAll(tid) {
		if existing.TenantID != tid {
			return ErrSubscriptionNotFound
		}
		sub.TenantID = tid
	}
	return s.inner.Update(ctx, sub)
}

// Delete deletes a subscription only if it belongs to the tenant in ctx
// (or ctx carries tenant.TenantAll). Cross-tenant deletes return
// ErrSubscriptionNotFound.
func (s *TenantEnforcedStore) Delete(ctx context.Context, id string) error {
	tid, err := tenant.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("storage.Delete: %w", err)
	}
	if !tenant.IsAll(tid) {
		existing, gerr := s.inner.Get(ctx, id)
		if gerr != nil {
			return gerr
		}
		if existing.TenantID != tid {
			return ErrSubscriptionNotFound
		}
	}
	return s.inner.Delete(ctx, id)
}

// List returns all subscriptions visible to the tenant in ctx.
// For a normal tenant it is equivalent to ListByTenant(ctx, tenantID).
// For tenant.TenantAll it returns everything.
func (s *TenantEnforcedStore) List(ctx context.Context) ([]*Subscription, error) {
	tid, err := tenant.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.List: %w", err)
	}
	if tenant.IsAll(tid) {
		return s.inner.List(ctx)
	}
	return s.inner.ListByTenant(ctx, tid)
}

// ListByResourcePool returns subscriptions filtered by resource pool and
// further constrained to the tenant in ctx (or all tenants for
// tenant.TenantAll).
func (s *TenantEnforcedStore) ListByResourcePool(ctx context.Context, resourcePoolID string) ([]*Subscription, error) {
	tid, err := tenant.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.ListByResourcePool: %w", err)
	}
	subs, err := s.inner.ListByResourcePool(ctx, resourcePoolID)
	if err != nil {
		return nil, err
	}
	return filterByTenant(subs, tid), nil
}

// ListByResourceType returns subscriptions filtered by resource type and
// further constrained to the tenant in ctx (or all tenants for
// tenant.TenantAll).
func (s *TenantEnforcedStore) ListByResourceType(ctx context.Context, resourceTypeID string) ([]*Subscription, error) {
	tid, err := tenant.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.ListByResourceType: %w", err)
	}
	subs, err := s.inner.ListByResourceType(ctx, resourceTypeID)
	if err != nil {
		return nil, err
	}
	return filterByTenant(subs, tid), nil
}

// ListByTenant validates that the ctx tenant matches the requested tenant
// (or that ctx carries tenant.TenantAll) before delegating.
func (s *TenantEnforcedStore) ListByTenant(ctx context.Context, tenantID string) ([]*Subscription, error) {
	tid, err := tenant.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.ListByTenant: %w", err)
	}
	if !tenant.IsAll(tid) && tid != tenantID {
		// Refuse cross-tenant listing.
		return nil, ErrSubscriptionNotFound
	}
	return s.inner.ListByTenant(ctx, tenantID)
}

// Close closes the underlying store.
func (s *TenantEnforcedStore) Close() error {
	return s.inner.Close()
}

// Ping probes the underlying store.
func (s *TenantEnforcedStore) Ping(ctx context.Context) error {
	return s.inner.Ping(ctx)
}

// filterByTenant returns the subset of subs whose TenantID matches tid.
// When tid is tenant.TenantAll the input is returned unchanged.
func filterByTenant(subs []*Subscription, tid string) []*Subscription {
	if tenant.IsAll(tid) {
		return subs
	}
	out := make([]*Subscription, 0, len(subs))
	for _, sub := range subs {
		if sub != nil && sub.TenantID == tid {
			out = append(out, sub)
		}
	}
	return out
}

// Compile-time check that TenantEnforcedStore satisfies Store.
var _ Store = (*TenantEnforcedStore)(nil)
