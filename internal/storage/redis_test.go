package storage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

// setupTestRedis creates a miniredis instance for testing.
func setupTestRedis(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := &RedisConfig{
		Addr:                   mr.Addr(),
		Password:               "",
		DB:                     0,
		UseSentinel:            false,
		MaxRetries:             1,
		DialTimeout:            1 * time.Second,
		ReadTimeout:            1 * time.Second,
		WriteTimeout:           1 * time.Second,
		PoolSize:               5,
		AllowInsecureCallbacks: true, // Allow HTTP in tests
	}

	store := NewRedisStore(cfg)

	return store, mr
}

func TestRedisStore_Create(t *testing.T) {
	tests := []struct {
		name    string
		sub     *Subscription
		wantErr error
	}{
		{
			name: "valid subscription",
			sub: &Subscription{
				ID:                     "sub-123",
				Callback:               "https://smo.example.com/notify",
				ConsumerSubscriptionID: "consumer-456",
				Filter: SubscriptionFilter{
					ResourcePoolID: "pool-abc",
					ResourceTypeID: "compute-node",
				},
			},
			wantErr: nil,
		},
		{
			name: "valid subscription with minimal fields",
			sub: &Subscription{
				ID:       "sub-minimal",
				Callback: "http://localhost:8080/webhook",
			},
			wantErr: nil,
		},
		{
			name: "empty ID",
			sub: &Subscription{
				ID:       "",
				Callback: "https://smo.example.com/notify",
			},
			wantErr: ErrInvalidID,
		},
		{
			name: "invalid callback - empty",
			sub: &Subscription{
				ID:       "sub-456",
				Callback: "",
			},
			wantErr: ErrInvalidCallback,
		},
		{
			name: "invalid callback - no scheme",
			sub: &Subscription{
				ID:       "sub-789",
				Callback: "smo.example.com/notify",
			},
			wantErr: ErrInvalidCallback,
		},
		{
			name: "invalid callback - invalid scheme",
			sub: &Subscription{
				ID:       "sub-999",
				Callback: "ftp://smo.example.com/notify",
			},
			wantErr: ErrInvalidCallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, mr := setupTestRedis(t)
			t.Cleanup(func() { mr.Close() })
			t.Cleanup(func() { require.NoError(t, store.Close()) })

			ctx := context.Background()
			err := store.Create(ctx, tt.sub)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				// Check if error is or wraps the expected error
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error type %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify subscription was created
			got, err := store.Get(ctx, tt.sub.ID)
			if err != nil {
				t.Fatalf("failed to get created subscription: %v", err)
			}

			// Verify fields
			if got.ID != tt.sub.ID {
				t.Errorf("ID = %v, want %v", got.ID, tt.sub.ID)
			}
			if got.Callback != tt.sub.Callback {
				t.Errorf("Callback = %v, want %v", got.Callback, tt.sub.Callback)
			}
			if got.ConsumerSubscriptionID != tt.sub.ConsumerSubscriptionID {
				t.Errorf("ConsumerSubscriptionID = %v, want %v", got.ConsumerSubscriptionID, tt.sub.ConsumerSubscriptionID)
			}
			if got.CreatedAt.IsZero() {
				t.Error("CreatedAt should be set")
			}
			if got.UpdatedAt.IsZero() {
				t.Error("UpdatedAt should be set")
			}
		})
	}
}

func TestRedisStore_Create_Duplicate(t *testing.T) {
	store, mr := setupTestRedis(t)
	t.Cleanup(func() { mr.Close() })
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	ctx := context.Background()

	sub := &Subscription{
		ID:       "sub-duplicate",
		Callback: "https://smo.example.com/notify",
	}

	// Create first time
	err := store.Create(ctx, sub)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	// Attempt duplicate
	err = store.Create(ctx, sub)
	if !errors.Is(err, ErrSubscriptionExists) {
		t.Errorf("expected ErrSubscriptionExists, got %v", err)
	}
}

func TestRedisStore_Get(t *testing.T) {
	store, mr := setupTestRedis(t)
	t.Cleanup(func() { mr.Close() })
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	ctx := context.Background()

	// Create a subscription first
	sub := &Subscription{
		ID:       "sub-get-test",
		Callback: "https://smo.example.com/notify",
		Filter: SubscriptionFilter{
			ResourcePoolID: "pool-123",
		},
	}

	err := store.Create(ctx, sub)
	if err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{
			name:    "existing subscription",
			id:      "sub-get-test",
			wantErr: nil,
		},
		{
			name:    "non-existent subscription",
			id:      "sub-nonexistent",
			wantErr: ErrSubscriptionNotFound,
		},
		{
			name:    "empty ID",
			id:      "",
			wantErr: ErrInvalidID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.Get(ctx, tt.id)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error type %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.ID != tt.id {
				t.Errorf("ID = %v, want %v", got.ID, tt.id)
			}
		})
	}
}

func TestRedisStore_Update(t *testing.T) {
	store, mr := setupTestRedis(t)
	t.Cleanup(func() { mr.Close() })
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	ctx := context.Background()

	// Create initial subscription
	sub := &Subscription{
		ID:       "sub-update-test",
		Callback: "https://smo.example.com/notify",
		Filter: SubscriptionFilter{
			ResourcePoolID: "pool-old",
		},
	}

	err := store.Create(ctx, sub)
	if err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	// Wait a bit to ensure timestamp difference
	time.Sleep(10 * time.Millisecond)

	tests := []struct {
		name    string
		sub     *Subscription
		wantErr error
	}{
		{
			name: "valid update",
			sub: &Subscription{
				ID:       "sub-update-test",
				Callback: "https://smo-new.example.com/notify",
				Filter: SubscriptionFilter{
					ResourcePoolID: "pool-new",
					ResourceTypeID: "compute-node",
				},
			},
			wantErr: nil,
		},
		{
			name: "non-existent subscription",
			sub: &Subscription{
				ID:       "sub-nonexistent",
				Callback: "https://smo.example.com/notify",
			},
			wantErr: ErrSubscriptionNotFound,
		},
		{
			name: "invalid callback",
			sub: &Subscription{
				ID:       "sub-update-test",
				Callback: "invalid-url",
			},
			wantErr: ErrInvalidCallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Update(ctx, tt.sub)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error type %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify update
			got, err := store.Get(ctx, tt.sub.ID)
			if err != nil {
				t.Fatalf("failed to get updated subscription: %v", err)
			}

			if got.Callback != tt.sub.Callback {
				t.Errorf("Callback = %v, want %v", got.Callback, tt.sub.Callback)
			}
			if got.UpdatedAt.IsZero() {
				t.Error("UpdatedAt should be set")
			}
			if !got.UpdatedAt.After(got.CreatedAt) {
				t.Error("UpdatedAt should be after CreatedAt")
			}
		})
	}
}

func TestRedisStore_Delete(t *testing.T) {
	store, mr := setupTestRedis(t)
	t.Cleanup(func() { mr.Close() })
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	ctx := context.Background()

	// Create subscription
	sub := &Subscription{
		ID:       "sub-delete-test",
		Callback: "https://smo.example.com/notify",
		Filter: SubscriptionFilter{
			ResourcePoolID: "pool-123",
			ResourceTypeID: "compute-node",
		},
	}

	err := store.Create(ctx, sub)
	if err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{
			name:    "existing subscription",
			id:      "sub-delete-test",
			wantErr: nil,
		},
		{
			name:    "non-existent subscription",
			id:      "sub-nonexistent",
			wantErr: ErrSubscriptionNotFound,
		},
		{
			name:    "empty ID",
			id:      "",
			wantErr: ErrInvalidID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Delete(ctx, tt.id)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error type %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify deletion
			_, err = store.Get(ctx, tt.id)
			if !errors.Is(err, ErrSubscriptionNotFound) {
				t.Errorf("expected ErrSubscriptionNotFound after delete, got %v", err)
			}
		})
	}
}

func TestRedisStore_List(t *testing.T) {
	store, mr := setupTestRedis(t)
	t.Cleanup(func() { mr.Close() })
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	ctx := context.Background()

	// Create multiple subscriptions
	subs := []*Subscription{
		{
			ID:       "sub-1",
			Callback: "https://smo1.example.com/notify",
		},
		{
			ID:       "sub-2",
			Callback: "https://smo2.example.com/notify",
		},
		{
			ID:       "sub-3",
			Callback: "https://smo3.example.com/notify",
		},
	}

	for _, sub := range subs {
		err := store.Create(ctx, sub)
		if err != nil {
			t.Fatalf("failed to create subscription %s: %v", sub.ID, err)
		}
	}

	// Test list
	result, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(result) != len(subs) {
		t.Errorf("List returned %d subscriptions, want %d", len(result), len(subs))
	}

	// Verify all IDs present
	ids := make(map[string]bool)
	for _, sub := range result {
		ids[sub.ID] = true
	}

	for _, sub := range subs {
		if !ids[sub.ID] {
			t.Errorf("subscription %s not found in list", sub.ID)
		}
	}
}

func TestRedisStore_List_Empty(t *testing.T) {
	store, mr := setupTestRedis(t)
	t.Cleanup(func() { mr.Close() })
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	ctx := context.Background()

	result, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("List returned %d subscriptions, want 0", len(result))
	}
}

func TestRedisStore_ListByResourcePool(t *testing.T) {
	store, mr := setupTestRedis(t)
	t.Cleanup(func() { mr.Close() })
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	ctx := context.Background()

	// Create subscriptions with different pool filters
	subs := []*Subscription{
		{
			ID:       "sub-pool-1",
			Callback: "https://smo.example.com/notify",
			Filter: SubscriptionFilter{
				ResourcePoolID: "pool-a",
			},
		},
		{
			ID:       "sub-pool-2",
			Callback: "https://smo.example.com/notify",
			Filter: SubscriptionFilter{
				ResourcePoolID: "pool-a",
			},
		},
		{
			ID:       "sub-pool-3",
			Callback: "https://smo.example.com/notify",
			Filter: SubscriptionFilter{
				ResourcePoolID: "pool-b",
			},
		},
		{
			ID:       "sub-pool-4",
			Callback: "https://smo.example.com/notify",
			// No filter
		},
	}

	for _, sub := range subs {
		err := store.Create(ctx, sub)
		if err != nil {
			t.Fatalf("failed to create subscription %s: %v", sub.ID, err)
		}
	}

	tests := []struct {
		name           string
		resourcePoolID string
		wantCount      int
		wantIDs        []string
	}{
		{
			name:           "pool-a subscriptions",
			resourcePoolID: "pool-a",
			wantCount:      2,
			wantIDs:        []string{"sub-pool-1", "sub-pool-2"},
		},
		{
			name:           "pool-b subscriptions",
			resourcePoolID: "pool-b",
			wantCount:      1,
			wantIDs:        []string{"sub-pool-3"},
		},
		{
			name:           "non-existent pool",
			resourcePoolID: "pool-nonexistent",
			wantCount:      0,
			wantIDs:        []string{},
		},
		{
			name:           "empty pool ID",
			resourcePoolID: "",
			wantCount:      0,
			wantIDs:        []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := store.ListByResourcePool(ctx, tt.resourcePoolID)
			if err != nil {
				t.Fatalf("ListByResourcePool failed: %v", err)
			}

			if len(result) != tt.wantCount {
				t.Errorf("got %d subscriptions, want %d", len(result), tt.wantCount)
			}

			// Verify expected IDs
			gotIDs := make(map[string]bool)
			for _, sub := range result {
				gotIDs[sub.ID] = true
			}

			for _, wantID := range tt.wantIDs {
				if !gotIDs[wantID] {
					t.Errorf("expected subscription %s not found", wantID)
				}
			}
		})
	}
}

func TestRedisStore_ListByResourceType(t *testing.T) {
	store, mr := setupTestRedis(t)
	t.Cleanup(func() { mr.Close() })
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	ctx := context.Background()

	// Create subscriptions with different type filters
	subs := []*Subscription{
		{
			ID:       "sub-type-1",
			Callback: "https://smo.example.com/notify",
			Filter: SubscriptionFilter{
				ResourceTypeID: "compute-node",
			},
		},
		{
			ID:       "sub-type-2",
			Callback: "https://smo.example.com/notify",
			Filter: SubscriptionFilter{
				ResourceTypeID: "compute-node",
			},
		},
		{
			ID:       "sub-type-3",
			Callback: "https://smo.example.com/notify",
			Filter: SubscriptionFilter{
				ResourceTypeID: "storage-node",
			},
		},
	}

	for _, sub := range subs {
		err := store.Create(ctx, sub)
		if err != nil {
			t.Fatalf("failed to create subscription %s: %v", sub.ID, err)
		}
	}

	tests := []struct {
		name           string
		resourceTypeID string
		wantCount      int
		wantIDs        []string
	}{
		{
			name:           "compute-node subscriptions",
			resourceTypeID: "compute-node",
			wantCount:      2,
			wantIDs:        []string{"sub-type-1", "sub-type-2"},
		},
		{
			name:           "storage-node subscriptions",
			resourceTypeID: "storage-node",
			wantCount:      1,
			wantIDs:        []string{"sub-type-3"},
		},
		{
			name:           "non-existent type",
			resourceTypeID: "network-node",
			wantCount:      0,
			wantIDs:        []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := store.ListByResourceType(ctx, tt.resourceTypeID)
			if err != nil {
				t.Fatalf("ListByResourceType failed: %v", err)
			}

			if len(result) != tt.wantCount {
				t.Errorf("got %d subscriptions, want %d", len(result), tt.wantCount)
			}

			// Verify expected IDs
			gotIDs := make(map[string]bool)
			for _, sub := range result {
				gotIDs[sub.ID] = true
			}

			for _, wantID := range tt.wantIDs {
				if !gotIDs[wantID] {
					t.Errorf("expected subscription %s not found", wantID)
				}
			}
		})
	}
}

func TestRedisStore_Ping(t *testing.T) {
	store, mr := setupTestRedis(t)
	t.Cleanup(func() { mr.Close() })
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	ctx := context.Background()

	// Test successful ping
	err := store.Ping(ctx)
	if err != nil {
		t.Errorf("Ping failed: %v", err)
	}

	// Test ping after closing miniredis
	mr.Close()
	err = store.Ping(ctx)
	if err == nil {
		t.Error("expected error after closing Redis, got nil")
	}
	if !errors.Is(err, ErrStorageUnavailable) {
		t.Errorf("expected ErrStorageUnavailable, got %v", err)
	}
}

func TestRedisStore_Close(t *testing.T) {
	store, mr := setupTestRedis(t)
	t.Cleanup(func() { mr.Close() })

	err := store.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestDefaultRedisConfig(t *testing.T) {
	cfg := DefaultRedisConfig()

	if cfg.Addr != "localhost:6379" {
		t.Errorf("default Addr = %v, want localhost:6379", cfg.Addr)
	}
	if cfg.DB != 0 {
		t.Errorf("default DB = %v, want 0", cfg.DB)
	}
	if cfg.UseSentinel {
		t.Error("default UseSentinel should be false")
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("default MaxRetries = %v, want 3", cfg.MaxRetries)
	}
	if cfg.PoolSize != 10 {
		t.Errorf("default PoolSize = %v, want 10", cfg.PoolSize)
	}
}

func TestSubscriptionFilter_MatchesFilter(t *testing.T) {
	tests := []struct {
		name           string
		filter         SubscriptionFilter
		resourcePoolID string
		resourceTypeID string
		resourceID     string
		want           bool
	}{
		{
			name:           "empty filter matches all",
			filter:         SubscriptionFilter{},
			resourcePoolID: "pool-123",
			resourceTypeID: "compute-node",
			resourceID:     "node-456",
			want:           true,
		},
		{
			name: "pool filter matches",
			filter: SubscriptionFilter{
				ResourcePoolID: "pool-123",
			},
			resourcePoolID: "pool-123",
			resourceTypeID: "compute-node",
			resourceID:     "node-456",
			want:           true,
		},
		{
			name: "pool filter does not match",
			filter: SubscriptionFilter{
				ResourcePoolID: "pool-123",
			},
			resourcePoolID: "pool-456",
			resourceTypeID: "compute-node",
			resourceID:     "node-789",
			want:           false,
		},
		{
			name: "type filter matches",
			filter: SubscriptionFilter{
				ResourceTypeID: "compute-node",
			},
			resourcePoolID: "pool-123",
			resourceTypeID: "compute-node",
			resourceID:     "node-456",
			want:           true,
		},
		{
			name: "multiple filters all match",
			filter: SubscriptionFilter{
				ResourcePoolID: "pool-123",
				ResourceTypeID: "compute-node",
			},
			resourcePoolID: "pool-123",
			resourceTypeID: "compute-node",
			resourceID:     "node-456",
			want:           true,
		},
		{
			name: "multiple filters one does not match",
			filter: SubscriptionFilter{
				ResourcePoolID: "pool-123",
				ResourceTypeID: "storage-node",
			},
			resourcePoolID: "pool-123",
			resourceTypeID: "compute-node",
			resourceID:     "node-456",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.MatchesFilter(tt.resourcePoolID, tt.resourceTypeID, tt.resourceID)
			if got != tt.want {
				t.Errorf("MatchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSubscription_MarshalBinary tests the MarshalBinary method.
func TestSubscription_MarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		sub     *Subscription
		wantErr bool
	}{
		{
			name: "valid subscription",
			sub: &Subscription{
				ID:       "sub-123",
				Callback: "https://example.com/callback",
				Filter: SubscriptionFilter{
					ResourceTypeID: "compute",
				},
			},
			wantErr: false,
		},
		{
			name: "subscription with all fields",
			sub: &Subscription{
				ID:       "sub-456",
				Callback: "https://example.com/notify",
				Filter: SubscriptionFilter{
					ResourcePoolID: "pool-1",
					ResourceTypeID: "storage",
					ResourceID:     "res-1",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.sub.MarshalBinary()

			if tt.wantErr {
				if err == nil {
					t.Error("MarshalBinary() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("MarshalBinary() unexpected error: %v", err)
				return
			}

			if len(data) == 0 {
				t.Error("MarshalBinary() returned empty data")
			}

			// Verify we can unmarshal it back
			var decoded Subscription
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("Failed to unmarshal marshaled data: %v", err)
			}
		})
	}
}

// TestSubscription_UnmarshalBinary tests the UnmarshalBinary method.
func TestSubscription_UnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		wantID  string
	}{
		{
			name:    "valid subscription data",
			data:    []byte(`{"subscriptionId":"sub-123","callback":"https://example.com/callback","filter":{"resourceTypeId":"compute"}}`),
			wantErr: false,
			wantID:  "sub-123",
		},
		{
			name:    "invalid JSON",
			data:    []byte(`{invalid json}`),
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte(``),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sub Subscription
			err := sub.UnmarshalBinary(tt.data)

			if tt.wantErr {
				if err == nil {
					t.Error("UnmarshalBinary() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("UnmarshalBinary() unexpected error: %v", err)
				return
			}

			if sub.ID != tt.wantID {
				t.Errorf("UnmarshalBinary() ID = %v, want %v", sub.ID, tt.wantID)
			}
		})
	}
}

// TestRedisStore_ListByTenant tests the ListByTenant method.
func TestRedisStore_ListByTenant(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	// Create test subscriptions with different tenants
	testSubs := []*Subscription{
		{
			ID:       "sub-tenant1-1",
			Callback: "https://tenant1.example.com/callback",
			TenantID: "tenant-1",
		},
		{
			ID:       "sub-tenant1-2",
			Callback: "https://tenant1.example.com/callback2",
			TenantID: "tenant-1",
		},
		{
			ID:       "sub-tenant2-1",
			Callback: "https://tenant2.example.com/callback",
			TenantID: "tenant-2",
		},
	}

	ctx := context.Background()
	for _, sub := range testSubs {
		if err := store.Create(ctx, sub); err != nil {
			t.Fatalf("Failed to create test subscription: %v", err)
		}
	}

	tests := []struct {
		name      string
		tenantID  string
		wantCount int
	}{
		{
			name:      "tenant with 2 subscriptions",
			tenantID:  "tenant-1",
			wantCount: 2,
		},
		{
			name:      "tenant with 1 subscription",
			tenantID:  "tenant-2",
			wantCount: 1,
		},
		{
			name:      "tenant with no subscriptions",
			tenantID:  "tenant-3",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subs, err := store.ListByTenant(ctx, tt.tenantID)
			if err != nil {
				t.Fatalf("ListByTenant() error = %v", err)
			}

			if len(subs) != tt.wantCount {
				t.Errorf("ListByTenant() returned %d subscriptions, want %d", len(subs), tt.wantCount)
			}

			// Verify all returned subscriptions belong to the tenant
			for _, sub := range subs {
				if sub.TenantID != tt.tenantID {
					t.Errorf("ListByTenant() returned subscription with TenantID %s, want %s", sub.TenantID, tt.tenantID)
				}
			}
		})
	}
}

// TestRedisStore_Client tests the Client method.
func TestRedisStore_Client(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	client := store.Client()
	if client == nil {
		t.Error("Client() returned nil")
	}

	// Verify the client works
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Errorf("Client() returned non-functional client: %v", err)
	}
}

// TestRedisStore_UpdateResourceTypeIndex tests edge cases in updateResourceTypeIndex.
func TestRedisStore_UpdateResourceTypeIndex(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()

	// Create a subscription with resource type filter
	sub := &Subscription{
		ID:       "sub-update-type",
		Callback: "https://example.com/callback",
		Filter: SubscriptionFilter{
			ResourceTypeID: "compute",
		},
	}
	err := store.Create(ctx, sub)
	require.NoError(t, err)

	// Update to a different resource type
	sub.Filter.ResourceTypeID = "storage"
	err = store.Update(ctx, sub)
	require.NoError(t, err)

	// Verify new index
	subs, err := store.ListByResourceType(ctx, "storage")
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, "sub-update-type", subs[0].ID)

	// Verify old index is empty
	subs, err = store.ListByResourceType(ctx, "compute")
	require.NoError(t, err)
	require.Len(t, subs, 0)
}

// TestNewRedisStore_InvalidAddress tests error handling in NewRedisStore.
func TestNewRedisStore_InvalidAddress(t *testing.T) {
	// Test with invalid configuration
	cfg := &RedisConfig{
		Addr: "invalid://address:999",
	}
	store := NewRedisStore(cfg)
	require.NotNil(t, store)

	// Operations should fail with invalid address
	ctx := context.Background()
	_, err := store.Get(ctx, "test")
	require.Error(t, err)

	store.Close()
}
