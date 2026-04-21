package server_test

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

// TestStorageBoundary_HandlerForgettingFilterCannotLeak simulates a handler
// that forgets to add an inline tenant check before calling the store. The
// storage layer's tenant enforcer must still prevent cross-tenant access,
// guaranteeing defense in depth for the class of bug described in issue #482.
func TestStorageBoundary_HandlerForgettingFilterCannotLeak(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	cfg := storage.DefaultRedisConfig()
	cfg.Addr = mr.Addr()
	cfg.AllowInsecureCallbacks = true
	inner := storage.NewRedisStore(cfg)
	t.Cleanup(func() { _ = inner.Close() })
	enforcer := storage.NewTenantEnforcedStore(inner)

	// Seed two tenants' subscriptions using an admin context.
	admin := tenant.WithAll(context.Background())
	require.NoError(t, enforcer.Create(admin, &storage.Subscription{
		ID:       "sub-alice",
		TenantID: "alice",
		Callback: "http://example.com/alice",
	}))
	require.NoError(t, enforcer.Create(admin, &storage.Subscription{
		ID:       "sub-bob",
		TenantID: "bob",
		Callback: "http://example.com/bob",
	}))

	// Case 1: a handler stamps tenant correctly via middleware; storage
	// returns only alice's subscriptions.
	aliceCtx := tenant.WithID(context.Background(), "alice")
	got, err := enforcer.List(aliceCtx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "alice", got[0].TenantID)

	// Case 2: a handler "forgets" to filter and calls Get with bob's id from
	// alice's context. Storage returns ErrSubscriptionNotFound — not the
	// subscription — even though the handler didn't add any inline check.
	sub, err := enforcer.Get(aliceCtx, "sub-bob")
	require.Error(t, err)
	assert.True(t, errors.Is(err, storage.ErrSubscriptionNotFound))
	assert.Nil(t, sub)

	// Case 3: a handler's ctx has no tenant stamped at all (middleware bug
	// or test wiring oversight). Storage refuses every operation with
	// tenant.ErrMissingTenant instead of silently returning data.
	bareCtx := context.Background()
	_, err = enforcer.List(bareCtx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, tenant.ErrMissingTenant))

	_, err = enforcer.Get(bareCtx, "sub-alice")
	require.Error(t, err)
	assert.True(t, errors.Is(err, tenant.ErrMissingTenant))

	err = enforcer.Delete(bareCtx, "sub-alice")
	require.Error(t, err)
	assert.True(t, errors.Is(err, tenant.ErrMissingTenant))

	// Case 4: a handler's ctx specifies the wrong tenant. Delete reports
	// not-found (identical shape to a genuine missing row) so the caller
	// cannot use response variance to enumerate sibling tenants' ids.
	wrongCtx := tenant.WithID(context.Background(), "alice")
	err = enforcer.Delete(wrongCtx, "sub-bob")
	require.Error(t, err)
	assert.True(t, errors.Is(err, storage.ErrSubscriptionNotFound))

	// Confirm bob's subscription is still intact.
	bobSub, err := enforcer.Get(admin, "sub-bob")
	require.NoError(t, err)
	assert.Equal(t, "bob", bobSub.TenantID)
}
