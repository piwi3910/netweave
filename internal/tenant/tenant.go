// Package tenant provides context propagation of tenant identity across
// the request/handler/storage stack.
//
// Tenant isolation is a cross-cutting concern: every storage read/write must
// be scoped to exactly one tenant, or explicitly declared to span all
// tenants (a platform-admin operation). Handlers that "forget" to filter by
// tenant are a class of bug that this package and the storage layer's
// enforcement are designed to make impossible.
//
// Usage:
//
//	// In middleware after authenticating the caller:
//	ctx = tenant.WithID(ctx, user.TenantID)
//	// or, for a platform-admin caller:
//	ctx = tenant.WithAll(ctx)
//
//	// In storage:
//	tid, err := tenant.FromContext(ctx)
//	if err != nil {
//	    return fmt.Errorf("storage: %w", err) // ErrMissingTenant
//	}
//	if tenant.IsAll(tid) {
//	    // platform-admin: skip filter
//	} else {
//	    // filter by tid
//	}
package tenant

import (
	"context"
	"errors"
)

// TenantAll is the sentinel tenant ID for platform-admin operations that
// must span every tenant (for example, global audit or cross-tenant listing).
//
// It is intentionally not a UUID or empty string so that no real tenant
// record can ever collide with it, and so that reviewers spot bypass sites
// when reading code.
const TenantAll = "*"

// ErrMissingTenant is returned when a storage operation is invoked without
// a tenant identity in context. This is a programmer error and must be
// treated as a 500-level failure by callers: it means middleware did not
// stamp tenant identity before the call reached storage.
var ErrMissingTenant = errors.New("tenant id required in context")

type ctxKey struct{}

// WithID returns a derived context carrying the given tenant ID.
// An empty id is rejected by FromContext with ErrMissingTenant.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// WithAll returns a derived context flagged as spanning all tenants.
// Callers that pass this context to storage bypass tenant filtering.
// Use ONLY for platform-admin-authenticated requests.
func WithAll(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKey{}, TenantAll)
}

// FromContext extracts the tenant ID from ctx. Returns ErrMissingTenant if
// no tenant has been stamped into the context.
func FromContext(ctx context.Context) (string, error) {
	v, ok := ctx.Value(ctxKey{}).(string)
	if !ok || v == "" {
		return "", ErrMissingTenant
	}
	return v, nil
}

// IsAll reports whether the given tenant id is the platform-admin bypass
// sentinel. Storage implementations MUST honor this by skipping tenant
// filtering.
func IsAll(id string) bool {
	return id == TenantAll
}
