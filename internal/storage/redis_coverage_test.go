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

func newTestRedisStore(t *testing.T) *RedisStore {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &RedisStore{
		Client: client,
		config: &RedisConfig{AllowInsecureCallbacks: true},
	}
}

func TestRedisStore_Update_ResourcePoolChange(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a subscription with a resource pool filter.
	sub := &Subscription{
		ID:       "sub-pool-change",
		Callback: "https://example.com/cb",
		Filter:   SubscriptionFilter{ResourcePoolID: "pool-old"},
	}
	require.NoError(t, store.Create(ctx, sub))

	// Update with new resource pool.
	updated, err := store.Get(ctx, "sub-pool-change")
	require.NoError(t, err)

	updated.Callback = "https://example.com/cb-new"
	updated.Filter.ResourcePoolID = "pool-new"
	err = store.Update(ctx, updated)
	require.NoError(t, err)

	// Verify new pool index.
	subs, err := store.ListByResourcePool(ctx, "pool-new")
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "sub-pool-change", subs[0].ID)

	// Old pool index should be empty.
	subs, err = store.ListByResourcePool(ctx, "pool-old")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestRedisStore_Update_ResourceTypeChange(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a subscription with a resource type filter.
	sub := &Subscription{
		ID:       "sub-type-change",
		Callback: "https://example.com/cb",
		Filter:   SubscriptionFilter{ResourceTypeID: "type-old"},
	}
	require.NoError(t, store.Create(ctx, sub))

	// Update with new resource type.
	updated, err := store.Get(ctx, "sub-type-change")
	require.NoError(t, err)

	updated.Callback = "https://example.com/cb-new"
	updated.Filter.ResourceTypeID = "type-new"
	err = store.Update(ctx, updated)
	require.NoError(t, err)

	// Verify new type index.
	subs, err := store.ListByResourceType(ctx, "type-new")
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "sub-type-change", subs[0].ID)

	// Old type index should be empty.
	subs, err = store.ListByResourceType(ctx, "type-old")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestRedisStore_ValidateUpdate_InvalidCallback(t *testing.T) {
	store := newTestRedisStore(t)
	store.config.AllowInsecureCallbacks = false
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a subscription.
	sub := &Subscription{
		ID:       "sub-validate-cb",
		Callback: "https://example.com/cb",
	}
	require.NoError(t, store.Create(ctx, sub))

	// Try to update with invalid callback.
	updated := &Subscription{
		ID:       "sub-validate-cb",
		Callback: "ftp://invalid-scheme.com/cb",
	}
	err := store.Update(ctx, updated)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCallback)
}

func TestRedisStore_ValidateUpdate_EmptyID(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	sub := &Subscription{
		ID:       "",
		Callback: "https://example.com/cb",
	}
	err := store.Update(ctx, sub)
	assert.ErrorIs(t, err, ErrInvalidID)
}

func TestRedisStore_ValidateUpdate_NotFound(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	sub := &Subscription{
		ID:       "non-existent",
		Callback: "https://example.com/cb",
	}
	err := store.Update(ctx, sub)
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestRedisStore_Create_EmptyID(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	sub := &Subscription{
		ID:       "",
		Callback: "https://example.com/cb",
	}
	err := store.Create(ctx, sub)
	assert.ErrorIs(t, err, ErrInvalidID)
}

func TestRedisStore_Create_InvalidCallback(t *testing.T) {
	store := newTestRedisStore(t)
	store.config.AllowInsecureCallbacks = false
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	sub := &Subscription{
		ID:       "sub-bad-cb",
		Callback: "not-a-url",
	}
	err := store.Create(ctx, sub)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCallback)
}

func TestRedisStore_Get_EmptyID(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	_, err := store.Get(ctx, "")
	assert.ErrorIs(t, err, ErrInvalidID)
}

func TestRedisStore_Get_NotFound(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	_, err := store.Get(ctx, "non-existent")
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestRedisStore_Delete_EmptyID(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err := store.Delete(ctx, "")
	assert.ErrorIs(t, err, ErrInvalidID)
}

func TestRedisStore_Delete_NotFound(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err := store.Delete(ctx, "non-existent")
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestRedisStore_ListByResourcePool_EmptyID(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	subs, err := store.ListByResourcePool(ctx, "non-existent-pool")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestRedisStore_ListByResourceType_EmptyResult(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	subs, err := store.ListByResourceType(ctx, "non-existent-type")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestRedisStore_ListByTenant_EmptyResult(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	subs, err := store.ListByTenant(ctx, "non-existent-tenant")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestNewRedisStore_NilConfig(t *testing.T) {
	store := NewRedisStore(nil)
	require.NotNil(t, store)
	assert.NotNil(t, store.config)
	assert.Equal(t, "localhost:6379", store.config.Addr)
}

func TestNewRedisStore_SentinelMode(t *testing.T) {
	cfg := &RedisConfig{
		UseSentinel:      true,
		MasterName:       "mymaster",
		SentinelAddrs:    []string{"localhost:26379"},
		SentinelPassword: "sentinel-pass",
		Password:         "redis-pass",
		DB:               0,
		MaxRetries:       3,
		DialTimeout:      1 * time.Second,
		ReadTimeout:      1 * time.Second,
		WriteTimeout:     1 * time.Second,
		PoolSize:         5,
	}

	store := NewRedisStore(cfg)
	require.NotNil(t, store)
	assert.NotNil(t, store.Client)
	// Store was created with sentinel config.
	assert.True(t, store.config.UseSentinel)
}

func TestRedisStore_Update_BothFiltersChange(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a subscription with both filters.
	sub := &Subscription{
		ID:       "sub-both-change",
		Callback: "https://example.com/cb",
		Filter: SubscriptionFilter{
			ResourcePoolID: "pool-a",
			ResourceTypeID: "type-a",
		},
		TenantID: "tenant-both",
	}
	require.NoError(t, store.Create(ctx, sub))

	// Update with both filters changed.
	updated, err := store.Get(ctx, "sub-both-change")
	require.NoError(t, err)

	updated.Callback = "https://example.com/cb-updated"
	updated.Filter.ResourcePoolID = "pool-b"
	updated.Filter.ResourceTypeID = "type-b"
	err = store.Update(ctx, updated)
	require.NoError(t, err)

	// Verify new indices.
	subs, err := store.ListByResourcePool(ctx, "pool-b")
	require.NoError(t, err)
	assert.Len(t, subs, 1)

	subs, err = store.ListByResourceType(ctx, "type-b")
	require.NoError(t, err)
	assert.Len(t, subs, 1)

	// Old indices empty.
	subs, err = store.ListByResourcePool(ctx, "pool-a")
	require.NoError(t, err)
	assert.Empty(t, subs)

	subs, err = store.ListByResourceType(ctx, "type-a")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestRedisStore_Delete_WithAllIndices(t *testing.T) {
	store := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a subscription with all filter fields and tenant.
	sub := &Subscription{
		ID:       "sub-del-all",
		Callback: "https://example.com/cb",
		TenantID: "tenant-del-all",
		Filter: SubscriptionFilter{
			ResourcePoolID: "pool-del",
			ResourceTypeID: "type-del",
		},
	}
	require.NoError(t, store.Create(ctx, sub))

	// Verify indices are populated.
	subs, err := store.ListByResourcePool(ctx, "pool-del")
	require.NoError(t, err)
	assert.Len(t, subs, 1)

	// Delete.
	err = store.Delete(ctx, "sub-del-all")
	require.NoError(t, err)

	// All indices should be empty.
	subs, err = store.ListByResourcePool(ctx, "pool-del")
	require.NoError(t, err)
	assert.Empty(t, subs)

	subs, err = store.ListByResourceType(ctx, "type-del")
	require.NoError(t, err)
	assert.Empty(t, subs)

	subs, err = store.ListByTenant(ctx, "tenant-del-all")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestRedisStore_Close_NoClient(t *testing.T) {
	// Test Close with a real store (not nil).
	store := newTestRedisStore(t)
	err := store.Close()
	assert.NoError(t, err)
}
