package storage

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRedisStoreWithMini creates a RedisStore backed by a miniredis instance.
// Returns both so callers can manipulate miniredis directly (e.g., close it, inject data).
func newTestRedisStoreWithMini(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		MaxRetries:   0,
		DialTimeout:  100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	})
	store := &RedisStore{
		Client: client,
		config: &RedisConfig{AllowInsecureCallbacks: true},
	}
	return store, mr
}

// createTestSubscription is a helper that creates a subscription in the store for testing.
func createTestSubscription(t *testing.T, store *RedisStore, id, callback string) {
	t.Helper()
	sub := &Subscription{
		ID:       id,
		Callback: callback,
	}
	require.NoError(t, store.Create(context.Background(), sub))
}

// ---------------------------------------------------------------------------
// ListByResourceType: empty ID returns empty slice
// ---------------------------------------------------------------------------

func TestRedisStore_ListByResourceType_EmptyID(t *testing.T) {
	t.Parallel()
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	subs, err := store.ListByResourceType(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

// ---------------------------------------------------------------------------
// ListByTenant: empty ID returns empty slice
// ---------------------------------------------------------------------------

func TestRedisStore_ListByTenant_EmptyID(t *testing.T) {
	t.Parallel()
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	subs, err := store.ListByTenant(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

// ---------------------------------------------------------------------------
// Get: corrupted data in Redis triggers unmarshal error
// ---------------------------------------------------------------------------

func TestRedisStore_Get_CorruptedData(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)
	defer func() { _ = store.Close() }()

	// Inject corrupted (non-JSON) data directly into Redis.
	mr.Set(subscriptionKeyPrefix+"corrupt-1", "this is not json")

	_, err := store.Get(context.Background(), "corrupt-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal subscription")
}

// ---------------------------------------------------------------------------
// List: corrupted data in active set is skipped gracefully
// ---------------------------------------------------------------------------

func TestRedisStore_List_SkipsCorruptedEntries(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a valid subscription normally.
	createTestSubscription(t, store, "valid-sub", "https://example.com/cb")

	// Manually add a corrupted entry: ID in the active set but corrupted data.
	mr.SAdd(subscriptionSetKey, "corrupt-sub")
	mr.Set(subscriptionKeyPrefix+"corrupt-sub", "not-valid-json")

	subs, err := store.List(ctx)
	require.NoError(t, err)
	// Only the valid subscription should be returned.
	assert.Len(t, subs, 1)
	assert.Equal(t, "valid-sub", subs[0].ID)
}

// ---------------------------------------------------------------------------
// ListByResourcePool: corrupted data is skipped
// ---------------------------------------------------------------------------

func TestRedisStore_ListByResourcePool_SkipsCorruptedEntries(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a valid subscription with a pool filter.
	sub := &Subscription{
		ID:       "pool-valid",
		Callback: "https://example.com/cb",
		Filter:   SubscriptionFilter{ResourcePoolID: "pool-test"},
	}
	require.NoError(t, store.Create(ctx, sub))

	// Inject a corrupted entry in the same pool index.
	poolKey := subscriptionPoolIndexPrefix + "pool-test"
	mr.SAdd(poolKey, "pool-corrupt")
	mr.Set(subscriptionKeyPrefix+"pool-corrupt", "{bad json")

	subs, err := store.ListByResourcePool(ctx, "pool-test")
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "pool-valid", subs[0].ID)
}

// ---------------------------------------------------------------------------
// ListByResourceType: corrupted data is skipped
// ---------------------------------------------------------------------------

func TestRedisStore_ListByResourceType_SkipsCorruptedEntries(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a valid subscription with a type filter.
	sub := &Subscription{
		ID:       "type-valid",
		Callback: "https://example.com/cb",
		Filter:   SubscriptionFilter{ResourceTypeID: "type-test"},
	}
	require.NoError(t, store.Create(ctx, sub))

	// Inject a corrupted entry in the same type index.
	typeKey := subscriptionTypeIndexPrefix + "type-test"
	mr.SAdd(typeKey, "type-corrupt")
	mr.Set(subscriptionKeyPrefix+"type-corrupt", "corrupted")

	subs, err := store.ListByResourceType(ctx, "type-test")
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "type-valid", subs[0].ID)
}

// ---------------------------------------------------------------------------
// ListByTenant: corrupted data is skipped
// ---------------------------------------------------------------------------

func TestRedisStore_ListByTenant_SkipsCorruptedEntries(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a valid subscription with tenant.
	sub := &Subscription{
		ID:       "tenant-valid",
		Callback: "https://example.com/cb",
		TenantID: "tenant-test",
	}
	require.NoError(t, store.Create(ctx, sub))

	// Inject a corrupted entry in the same tenant index.
	tenantKey := subscriptionTenantIndexPrefix + "tenant-test"
	mr.SAdd(tenantKey, "tenant-corrupt")
	mr.Set(subscriptionKeyPrefix+"tenant-corrupt", "not json data")

	subs, err := store.ListByTenant(ctx, "tenant-test")
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "tenant-valid", subs[0].ID)
}

// ---------------------------------------------------------------------------
// Close: error path when underlying client returns an error
// ---------------------------------------------------------------------------

func TestRedisStore_Close_ReturnsError(t *testing.T) {
	t.Parallel()
	store := newTestRedisStore(t)

	// Close once successfully.
	require.NoError(t, store.Close())

	// Closing again should return an error since the client is already closed.
	err := store.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to close Redis client")
}

// ---------------------------------------------------------------------------
// Redis unavailable: Create Exists check error
// ---------------------------------------------------------------------------

func TestRedisStore_Create_RedisUnavailable(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)

	// Close miniredis to simulate Redis being unavailable.
	mr.Close()

	sub := &Subscription{
		ID:       "sub-unavail",
		Callback: "https://example.com/cb",
	}
	err := store.Create(context.Background(), sub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check subscription existence")
}

// ---------------------------------------------------------------------------
// Redis unavailable: validateUpdate Exists check error
// ---------------------------------------------------------------------------

func TestRedisStore_ValidateUpdate_RedisUnavailable(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)

	// Create a subscription while Redis is up.
	createTestSubscription(t, store, "sub-validate-unavail", "https://example.com/cb")

	// Now close miniredis to make Exists fail.
	mr.Close()

	updated := &Subscription{
		ID:       "sub-validate-unavail",
		Callback: "https://example.com/cb-updated",
	}
	err := store.Update(context.Background(), updated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check subscription existence")
}

// ---------------------------------------------------------------------------
// List: SMembers error when Redis is unavailable
// ---------------------------------------------------------------------------

func TestRedisStore_List_RedisUnavailable(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)

	mr.Close()

	_, err := store.List(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list subscription IDs")
}

// ---------------------------------------------------------------------------
// ListByResourcePool: SMembers error when Redis is unavailable
// ---------------------------------------------------------------------------

func TestRedisStore_ListByResourcePool_RedisUnavailable(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)

	mr.Close()

	_, err := store.ListByResourcePool(context.Background(), "some-pool")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list subscriptions by pool")
}

// ---------------------------------------------------------------------------
// ListByResourceType: SMembers error when Redis is unavailable
// ---------------------------------------------------------------------------

func TestRedisStore_ListByResourceType_RedisUnavailable(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)

	mr.Close()

	_, err := store.ListByResourceType(context.Background(), "some-type")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list subscriptions by type")
}

// ---------------------------------------------------------------------------
// ListByTenant: SMembers error when Redis is unavailable
// ---------------------------------------------------------------------------

func TestRedisStore_ListByTenant_RedisUnavailable(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)

	mr.Close()

	_, err := store.ListByTenant(context.Background(), "some-tenant")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list subscriptions by tenant")
}

// ---------------------------------------------------------------------------
// Delete: pipeline Exec error when Redis goes down after Get succeeds
// ---------------------------------------------------------------------------

func TestRedisStore_Delete_PipelineExecError(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)

	ctx := context.Background()

	// Create a subscription normally.
	createTestSubscription(t, store, "sub-del-pipe", "https://example.com/cb")

	// Close miniredis so subsequent operations fail.
	mr.Close()

	err := store.Delete(ctx, "sub-del-pipe")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Create: pipeline Exec error when Redis is unavailable
// ---------------------------------------------------------------------------

func TestRedisStore_Create_PipelineExecError(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		MaxRetries:   0,
		DialTimeout:  100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	})
	store := &RedisStore{
		Client: client,
		config: &RedisConfig{AllowInsecureCallbacks: true},
	}

	mr.Close()

	sub := &Subscription{
		ID:       "sub-pipe-fail",
		Callback: "https://example.com/cb",
	}
	err := store.Create(context.Background(), sub)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Update: error propagation from Get after validateUpdate passes
// This covers line 275: existing, err := r.Get(ctx, sub.ID)
// ---------------------------------------------------------------------------

func TestRedisStore_Update_GetFailsAfterValidation(t *testing.T) {
	t.Parallel()
	store, mr := newTestRedisStoreWithMini(t)

	ctx := context.Background()

	// Create a subscription so the Exists check in validateUpdate will pass.
	createTestSubscription(t, store, "sub-update-get-fail", "https://example.com/cb")

	// Now inject corrupted data so that Get's unmarshal fails.
	// The validateUpdate only checks Exists (key exists), but Get needs to unmarshal.
	mr.Set(subscriptionKeyPrefix+"sub-update-get-fail", "corrupted-data")

	updated := &Subscription{
		ID:       "sub-update-get-fail",
		Callback: "https://example.com/cb-new",
	}
	err := store.Update(ctx, updated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal subscription")
}
