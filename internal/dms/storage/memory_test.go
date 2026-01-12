package storage

import (
	"context"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/dms/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_Create(t *testing.T) {
	store := NewMemoryStore()
	defer func() {
		require.NoError(t, store.Close())
	}()

	sub := &models.DMSSubscription{
		SubscriptionID:         "sub-1",
		Callback:               "https://example.com/webhook",
		ConsumerSubscriptionID: "consumer-1",
		CreatedAt:              time.Now(),
	}

	err := store.Create(context.Background(), sub)
	require.NoError(t, err)

	// Verify it was stored.
	retrieved, err := store.Get(context.Background(), "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "sub-1", retrieved.SubscriptionID)
	assert.Equal(t, "https://example.com/webhook", retrieved.Callback)
}

func TestMemoryStore_CreateDuplicate(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	sub := &models.DMSSubscription{
		SubscriptionID: "sub-1",
		Callback:       "https://example.com/webhook",
	}

	err := store.Create(context.Background(), sub)
	require.NoError(t, err)

	// Try to create duplicate.
	err = store.Create(context.Background(), sub)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSubscriptionExists)
}

func TestMemoryStore_Get(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	sub := &models.DMSSubscription{
		SubscriptionID: "sub-1",
		Callback:       "https://example.com/webhook",
	}

	err := store.Create(context.Background(), sub)
	require.NoError(t, err)

	retrieved, err := store.Get(context.Background(), "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "sub-1", retrieved.SubscriptionID)
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	_, err := store.Get(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestMemoryStore_List(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	// Create multiple subscriptions.
	for i := 0; i < 3; i++ {
		sub := &models.DMSSubscription{
			SubscriptionID: "sub-" + string(rune('1'+i)),
			Callback:       "https://example.com/webhook",
		}
		err := store.Create(context.Background(), sub)
		require.NoError(t, err)
	}

	subs, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, subs, 3)
}

func TestMemoryStore_ListEmpty(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	subs, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestMemoryStore_Update(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	sub := &models.DMSSubscription{
		SubscriptionID: "sub-1",
		Callback:       "https://example.com/webhook",
	}

	err := store.Create(context.Background(), sub)
	require.NoError(t, err)

	// Update the subscription.
	sub.Callback = "https://example.com/new-webhook"
	sub.UpdatedAt = time.Now()

	err = store.Update(context.Background(), sub)
	require.NoError(t, err)

	// Verify the update.
	retrieved, err := store.Get(context.Background(), "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/new-webhook", retrieved.Callback)
}

func TestMemoryStore_UpdateNotFound(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	sub := &models.DMSSubscription{
		SubscriptionID: "nonexistent",
		Callback:       "https://example.com/webhook",
	}

	err := store.Update(context.Background(), sub)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	sub := &models.DMSSubscription{
		SubscriptionID: "sub-1",
		Callback:       "https://example.com/webhook",
	}

	err := store.Create(context.Background(), sub)
	require.NoError(t, err)

	err = store.Delete(context.Background(), "sub-1")
	require.NoError(t, err)

	// Verify it's deleted.
	_, err = store.Get(context.Background(), "sub-1")
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestMemoryStore_DeleteNotFound(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	err := store.Delete(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestMemoryStore_Ping(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	err := store.Ping(context.Background())
	assert.NoError(t, err)
}

func TestMemoryStore_Close(t *testing.T) {
	store := NewMemoryStore()

	// Add some data.
	sub := &models.DMSSubscription{
		SubscriptionID: "sub-1",
		Callback:       "https://example.com/webhook",
	}
	err := store.Create(context.Background(), sub)
	require.NoError(t, err)

	// Close the store.
	err = store.Close()
	require.NoError(t, err)

	// Verify data is cleared.
	subs, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	// Create multiple subscriptions concurrently.
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			sub := &models.DMSSubscription{
				SubscriptionID: "sub-" + string(rune('a'+idx)),
				Callback:       "https://example.com/webhook",
			}
			_ = store.Create(context.Background(), sub)
			done <- true
		}(i)
	}

	// Wait for all goroutines.
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify subscriptions were created.
	subs, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, subs, 10)
}

func TestMemoryStore_IsolatedCopies(t *testing.T) {
	store := NewMemoryStore()
	defer func() { require.NoError(t, store.Close()) }()

	sub := &models.DMSSubscription{
		SubscriptionID: "sub-1",
		Callback:       "https://example.com/webhook",
	}

	err := store.Create(context.Background(), sub)
	require.NoError(t, err)

	// Modify the original subscription.
	sub.Callback = "modified"

	// Retrieve and verify it wasn't affected.
	retrieved, err := store.Get(context.Background(), "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/webhook", retrieved.Callback)
}
