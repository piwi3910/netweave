package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryHubStore_Create(t *testing.T) {
	tests := []struct {
		name    string
		hub     *HubRegistration
		wantErr error
	}{
		{
			name: "valid hub registration",
			hub: &HubRegistration{
				HubID:          "hub-123",
				Callback:       "https://smo.example.com/notify",
				Query:          "eventType=ResourceCreationNotification",
				SubscriptionID: "sub-456",
				CreatedAt:      time.Now(),
			},
			wantErr: nil,
		},
		{
			name:    "nil hub registration",
			hub:     nil,
			wantErr: assert.AnError,
		},
		{
			name: "empty hub ID",
			hub: &HubRegistration{
				Callback: "https://smo.example.com/notify",
			},
			wantErr: assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewInMemoryHubStore()
			defer func() { _ = store.Close() }()

			ctx := context.Background()
			err := store.Create(ctx, tt.hub)

			if tt.wantErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify hub was stored
				retrieved, err := store.Get(ctx, tt.hub.HubID)
				require.NoError(t, err)
				assert.Equal(t, tt.hub.HubID, retrieved.HubID)
				assert.Equal(t, tt.hub.Callback, retrieved.Callback)
				assert.Equal(t, tt.hub.Query, retrieved.Query)
				assert.Equal(t, tt.hub.SubscriptionID, retrieved.SubscriptionID)
			}
		})
	}
}

func TestInMemoryHubStore_Create_Duplicate(t *testing.T) {
	store := NewInMemoryHubStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	hub := &HubRegistration{
		HubID:          "hub-123",
		Callback:       "https://smo.example.com/notify",
		Query:          "eventType=ResourceCreationNotification",
		SubscriptionID: "sub-456",
		CreatedAt:      time.Now(),
	}

	// Create first time - should succeed
	err := store.Create(ctx, hub)
	require.NoError(t, err)

	// Create second time - should fail
	err = store.Create(ctx, hub)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrHubExists)
}

func TestInMemoryHubStore_Get(t *testing.T) {
	store := NewInMemoryHubStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create test hub
	hub := &HubRegistration{
		HubID:          "hub-123",
		Callback:       "https://smo.example.com/notify",
		Query:          "eventType=ResourceCreationNotification",
		SubscriptionID: "sub-456",
		CreatedAt:      time.Now(),
		Extensions: map[string]interface{}{
			"test": "value",
		},
	}
	err := store.Create(ctx, hub)
	require.NoError(t, err)

	tests := []struct {
		name    string
		hubID   string
		wantErr error
	}{
		{
			name:    "existing hub",
			hubID:   "hub-123",
			wantErr: nil,
		},
		{
			name:    "non-existent hub",
			hubID:   "hub-999",
			wantErr: ErrHubNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := store.Get(ctx, tt.hubID)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, retrieved)
			} else {
				require.NoError(t, err)
				require.NotNil(t, retrieved)
				assert.Equal(t, hub.HubID, retrieved.HubID)
				assert.Equal(t, hub.Callback, retrieved.Callback)
				assert.Equal(t, hub.Query, retrieved.Query)
				assert.Equal(t, hub.SubscriptionID, retrieved.SubscriptionID)
				assert.Equal(t, hub.Extensions, retrieved.Extensions)
			}
		})
	}
}

func TestInMemoryHubStore_List(t *testing.T) {
	store := NewInMemoryHubStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Test empty store
	t.Run("empty store", func(t *testing.T) {
		hubs, err := store.List(ctx)
		require.NoError(t, err)
		assert.Empty(t, hubs)
	})

	// Add multiple hubs
	hubs := []*HubRegistration{
		{
			HubID:          "hub-1",
			Callback:       "https://smo1.example.com/notify",
			Query:          "eventType=ResourceCreationNotification",
			SubscriptionID: "sub-1",
			CreatedAt:      time.Now(),
		},
		{
			HubID:          "hub-2",
			Callback:       "https://smo2.example.com/notify",
			Query:          "eventType=ResourceStateChangeNotification",
			SubscriptionID: "sub-2",
			CreatedAt:      time.Now(),
		},
		{
			HubID:          "hub-3",
			Callback:       "https://smo3.example.com/notify",
			Query:          "resourceId=res-123",
			SubscriptionID: "sub-3",
			CreatedAt:      time.Now(),
		},
	}

	for _, hub := range hubs {
		err := store.Create(ctx, hub)
		require.NoError(t, err)
	}

	// Test list with multiple hubs
	t.Run("multiple hubs", func(t *testing.T) {
		retrieved, err := store.List(ctx)
		require.NoError(t, err)
		assert.Len(t, retrieved, 3)

		// Verify all hubs are present (order not guaranteed)
		hubMap := make(map[string]*HubRegistration)
		for _, h := range retrieved {
			hubMap[h.HubID] = h
		}

		for _, original := range hubs {
			retrieved, exists := hubMap[original.HubID]
			require.True(t, exists)
			assert.Equal(t, original.Callback, retrieved.Callback)
			assert.Equal(t, original.Query, retrieved.Query)
			assert.Equal(t, original.SubscriptionID, retrieved.SubscriptionID)
		}
	})
}

func TestInMemoryHubStore_Delete(t *testing.T) {
	store := NewInMemoryHubStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create test hub
	hub := &HubRegistration{
		HubID:          "hub-123",
		Callback:       "https://smo.example.com/notify",
		Query:          "eventType=ResourceCreationNotification",
		SubscriptionID: "sub-456",
		CreatedAt:      time.Now(),
	}
	err := store.Create(ctx, hub)
	require.NoError(t, err)

	tests := []struct {
		name    string
		hubID   string
		wantErr error
	}{
		{
			name:    "existing hub",
			hubID:   "hub-123",
			wantErr: nil,
		},
		{
			name:    "non-existent hub",
			hubID:   "hub-999",
			wantErr: ErrHubNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Delete(ctx, tt.hubID)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)

				// Verify hub was deleted
				_, err := store.Get(ctx, tt.hubID)
				require.Error(t, err)
				require.ErrorIs(t, err, ErrHubNotFound)
			}
		})
	}
}

func TestInMemoryHubStore_Close(t *testing.T) {
	store := NewInMemoryHubStore()

	ctx := context.Background()

	// Add a hub
	hub := &HubRegistration{
		HubID:          "hub-123",
		Callback:       "https://smo.example.com/notify",
		Query:          "eventType=ResourceCreationNotification",
		SubscriptionID: "sub-456",
		CreatedAt:      time.Now(),
	}
	err := store.Create(ctx, hub)
	require.NoError(t, err)

	// Close the store
	err = store.Close()
	require.NoError(t, err)

	// Verify store is empty after close
	assert.Nil(t, store.hubs)
}

func TestInMemoryHubStore_ConcurrentAccess(t *testing.T) {
	store := NewInMemoryHubStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Concurrent creates
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			hub := &HubRegistration{
				HubID:          string(rune('a' + id)),
				Callback:       "https://smo.example.com/notify",
				SubscriptionID: string(rune('a' + id)),
				CreatedAt:      time.Now(),
			}
			err := store.Create(ctx, hub)
			if err != nil && !errors.Is(err, ErrHubExists) {
				t.Errorf("unexpected error: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify we have hubs stored
	hubs, err := store.List(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, hubs)
}
