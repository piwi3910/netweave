package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/storage"
	"github.com/piwi3910/netweave/internal/tenant"
)

// newTestEnforcedRedis returns a TenantEnforcedStore backed by miniredis.
func newTestEnforcedRedis(t *testing.T) *storage.TenantEnforcedStore {
	t.Helper()
	mr := miniredis.RunT(t)
	cfg := storage.DefaultRedisConfig()
	cfg.Addr = mr.Addr()
	cfg.AllowInsecureCallbacks = true
	inner := storage.NewRedisStore(cfg)
	t.Cleanup(func() { _ = inner.Close() })
	return storage.NewTenantEnforcedStore(inner)
}

func seedSub(t *testing.T, store *storage.TenantEnforcedStore, id, tenantID string) {
	t.Helper()
	ctx := tenant.WithAll(context.Background())
	err := store.Create(ctx, &storage.Subscription{
		ID:       id,
		TenantID: tenantID,
		Callback: "http://example.com/cb",
	})
	require.NoError(t, err)
}

func TestTenantEnforcedStore_RejectsMissingTenant(t *testing.T) {
	t.Parallel()

	store := newTestEnforcedRedis(t)
	bare := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"Create", func() error {
			return store.Create(bare, &storage.Subscription{ID: "x", Callback: "http://e.com/c"})
		}},
		{"Get", func() error {
			_, err := store.Get(bare, "x")
			return err
		}},
		{"Update", func() error {
			return store.Update(bare, &storage.Subscription{ID: "x", Callback: "http://e.com/c"})
		}},
		{"Delete", func() error { return store.Delete(bare, "x") }},
		{"List", func() error {
			_, err := store.List(bare)
			return err
		}},
		{"ListByResourcePool", func() error {
			_, err := store.ListByResourcePool(bare, "p")
			return err
		}},
		{"ListByResourceType", func() error {
			_, err := store.ListByResourceType(bare, "t")
			return err
		}},
		{"ListByTenant", func() error {
			_, err := store.ListByTenant(bare, "t1")
			return err
		}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.call()
			require.Error(t, err)
			assert.True(t, errors.Is(err, tenant.ErrMissingTenant),
				"method %s: expected ErrMissingTenant, got %v", tc.name, err)
		})
	}
}

func TestTenantEnforcedStore_GetCrossTenantReturnsNotFound(t *testing.T) {
	t.Parallel()

	store := newTestEnforcedRedis(t)
	seedSub(t, store, "sub-a", "tenant-a")

	// tenant-b must not see tenant-a's subscription.
	ctxB := tenant.WithID(context.Background(), "tenant-b")
	got, err := store.Get(ctxB, "sub-a")
	require.Error(t, err)
	assert.True(t, errors.Is(err, storage.ErrSubscriptionNotFound))
	assert.Nil(t, got)

	// Same tenant can read it.
	ctxA := tenant.WithID(context.Background(), "tenant-a")
	got, err = store.Get(ctxA, "sub-a")
	require.NoError(t, err)
	assert.Equal(t, "tenant-a", got.TenantID)

	// Platform admin can read it.
	ctxAll := tenant.WithAll(context.Background())
	got, err = store.Get(ctxAll, "sub-a")
	require.NoError(t, err)
	assert.Equal(t, "tenant-a", got.TenantID)
}

func TestTenantEnforcedStore_UpdateCrossTenantReturnsNotFound(t *testing.T) {
	t.Parallel()

	store := newTestEnforcedRedis(t)
	seedSub(t, store, "sub-a", "tenant-a")

	// tenant-b tries to update tenant-a's subscription.
	ctxB := tenant.WithID(context.Background(), "tenant-b")
	err := store.Update(ctxB, &storage.Subscription{
		ID:       "sub-a",
		Callback: "http://evil.example.com/pwn",
		TenantID: "tenant-b", // caller-supplied; must be ignored
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, storage.ErrSubscriptionNotFound))

	// Verify tenant-a's sub was not mutated.
	ctxAll := tenant.WithAll(context.Background())
	got, err := store.Get(ctxAll, "sub-a")
	require.NoError(t, err)
	assert.Equal(t, "tenant-a", got.TenantID)
	assert.Equal(t, "http://example.com/cb", got.Callback)
}

func TestTenantEnforcedStore_UpdateOverwritesCallerTenantID(t *testing.T) {
	t.Parallel()

	store := newTestEnforcedRedis(t)
	seedSub(t, store, "sub-a", "tenant-a")

	// Legit update from tenant-a, but the caller tried to set TenantID to
	// something else. The decorator must overwrite TenantID with the ctx
	// tenant so the subscription cannot be re-homed.
	ctxA := tenant.WithID(context.Background(), "tenant-a")
	err := store.Update(ctxA, &storage.Subscription{
		ID:       "sub-a",
		Callback: "http://example.com/new",
		TenantID: "tenant-b", // malicious override attempt
	})
	require.NoError(t, err)

	ctxAll := tenant.WithAll(context.Background())
	got, err := store.Get(ctxAll, "sub-a")
	require.NoError(t, err)
	assert.Equal(t, "tenant-a", got.TenantID, "TenantID must not be re-homed")
	assert.Equal(t, "http://example.com/new", got.Callback)
}

func TestTenantEnforcedStore_DeleteCrossTenantReturnsNotFound(t *testing.T) {
	t.Parallel()

	store := newTestEnforcedRedis(t)
	seedSub(t, store, "sub-a", "tenant-a")

	ctxB := tenant.WithID(context.Background(), "tenant-b")
	err := store.Delete(ctxB, "sub-a")
	require.Error(t, err)
	assert.True(t, errors.Is(err, storage.ErrSubscriptionNotFound))

	// Sub still exists.
	ctxAll := tenant.WithAll(context.Background())
	_, err = store.Get(ctxAll, "sub-a")
	require.NoError(t, err)

	// tenant-a can delete it.
	ctxA := tenant.WithID(context.Background(), "tenant-a")
	err = store.Delete(ctxA, "sub-a")
	require.NoError(t, err)

	_, err = store.Get(ctxAll, "sub-a")
	require.Error(t, err)
	assert.True(t, errors.Is(err, storage.ErrSubscriptionNotFound))
}

func TestTenantEnforcedStore_ListFiltersByTenant(t *testing.T) {
	t.Parallel()

	store := newTestEnforcedRedis(t)
	seedSub(t, store, "sub-a1", "tenant-a")
	seedSub(t, store, "sub-a2", "tenant-a")
	seedSub(t, store, "sub-b1", "tenant-b")

	ctxA := tenant.WithID(context.Background(), "tenant-a")
	gotA, err := store.List(ctxA)
	require.NoError(t, err)
	assert.Len(t, gotA, 2)
	for _, s := range gotA {
		assert.Equal(t, "tenant-a", s.TenantID)
	}

	ctxB := tenant.WithID(context.Background(), "tenant-b")
	gotB, err := store.List(ctxB)
	require.NoError(t, err)
	assert.Len(t, gotB, 1)
	assert.Equal(t, "tenant-b", gotB[0].TenantID)

	// Platform admin sees everything.
	ctxAll := tenant.WithAll(context.Background())
	gotAll, err := store.List(ctxAll)
	require.NoError(t, err)
	assert.Len(t, gotAll, 3)
}

func TestTenantEnforcedStore_ListByTenantRefusesCrossTenant(t *testing.T) {
	t.Parallel()

	store := newTestEnforcedRedis(t)
	seedSub(t, store, "sub-a1", "tenant-a")

	// tenant-b asking for tenant-a's subs is refused, not silently filtered.
	ctxB := tenant.WithID(context.Background(), "tenant-b")
	_, err := store.ListByTenant(ctxB, "tenant-a")
	require.Error(t, err)
	assert.True(t, errors.Is(err, storage.ErrSubscriptionNotFound))

	// Platform admin can ask for any specific tenant.
	ctxAll := tenant.WithAll(context.Background())
	got, err := store.ListByTenant(ctxAll, "tenant-a")
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestTenantEnforcedStore_CreateStampsTenant(t *testing.T) {
	t.Parallel()

	store := newTestEnforcedRedis(t)

	// Caller supplies no TenantID (or wrong one); the decorator stamps it
	// from ctx.
	ctxA := tenant.WithID(context.Background(), "tenant-a")
	err := store.Create(ctxA, &storage.Subscription{
		ID:       "sub-new",
		Callback: "http://example.com/cb",
		TenantID: "tenant-b", // attempted override
	})
	require.NoError(t, err)

	ctxAll := tenant.WithAll(context.Background())
	got, err := store.Get(ctxAll, "sub-new")
	require.NoError(t, err)
	assert.Equal(t, "tenant-a", got.TenantID, "ctx tenant must win over body")
}

func TestTenantEnforcedStore_CreateAllCtxRequiresExplicitTenant(t *testing.T) {
	t.Parallel()

	store := newTestEnforcedRedis(t)

	// Platform admin ctx without a TenantID on the subscription is refused:
	// subscriptions belong to SOME tenant at rest.
	ctxAll := tenant.WithAll(context.Background())
	err := store.Create(ctxAll, &storage.Subscription{
		ID:       "sub-orphan",
		Callback: "http://example.com/cb",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, tenant.ErrMissingTenant))
}

func TestTenantEnforcedStore_ListByResourcePoolFilters(t *testing.T) {
	t.Parallel()

	store := newTestEnforcedRedis(t)
	ctxAll := tenant.WithAll(context.Background())

	err := store.Create(ctxAll, &storage.Subscription{
		ID:       "s1",
		TenantID: "tenant-a",
		Callback: "http://example.com/a",
		Filter:   storage.SubscriptionFilter{ResourcePoolID: "pool-1"},
	})
	require.NoError(t, err)
	err = store.Create(ctxAll, &storage.Subscription{
		ID:       "s2",
		TenantID: "tenant-b",
		Callback: "http://example.com/b",
		Filter:   storage.SubscriptionFilter{ResourcePoolID: "pool-1"},
	})
	require.NoError(t, err)

	ctxA := tenant.WithID(context.Background(), "tenant-a")
	got, err := store.ListByResourcePool(ctxA, "pool-1")
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, "tenant-a", got[0].TenantID)
}
