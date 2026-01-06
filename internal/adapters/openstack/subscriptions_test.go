package openstack

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// TestCreateSubscription tests subscription creation.
func TestCreateSubscription(t *testing.T) {
	adp := &OpenStackAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}

	ctx := context.Background()

	t.Run("valid subscription", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback:               "https://callback.example.com/notify",
			ConsumerSubscriptionID: "consumer-sub-123",
			Filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-1",
			},
		}

		created, err := adp.CreateSubscription(ctx, sub)

		require.NoError(t, err)
		assert.NotEmpty(t, created.SubscriptionID)
		assert.Equal(t, sub.Callback, created.Callback)
		assert.Equal(t, sub.ConsumerSubscriptionID, created.ConsumerSubscriptionID)
		assert.NotNil(t, created.Filter)
		assert.Equal(t, "pool-1", created.Filter.ResourcePoolID)

		// Verify subscription is stored
		stored, err := adp.GetSubscription(ctx, created.SubscriptionID)
		require.NoError(t, err)
		assert.Equal(t, created.SubscriptionID, stored.SubscriptionID)
	})

	t.Run("subscription with provided ID", func(t *testing.T) {
		subID := "custom-sub-id"
		sub := &adapter.Subscription{
			SubscriptionID: subID,
			Callback:       "https://callback.example.com/notify",
		}

		created, err := adp.CreateSubscription(ctx, sub)

		require.NoError(t, err)
		assert.Equal(t, subID, created.SubscriptionID)
	})

	t.Run("missing callback URL", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback: "",
		}

		_, err := adp.CreateSubscription(ctx, sub)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "callback URL is required")
	})

	t.Run("subscription ID generation", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback: "https://callback.example.com/notify",
		}

		created, err := adp.CreateSubscription(ctx, sub)

		require.NoError(t, err)
		assert.NotEmpty(t, created.SubscriptionID)
		assert.Contains(t, created.SubscriptionID, "openstack-sub-")

		// Verify it's a valid UUID suffix
		parts := created.SubscriptionID[len("openstack-sub-"):]
		_, err = uuid.Parse(parts)
		assert.NoError(t, err, "subscription ID should contain a valid UUID")
	})
}

// TestGetSubscription tests subscription retrieval.
func TestGetSubscription(t *testing.T) {
	adp := &OpenStackAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}

	ctx := context.Background()

	t.Run("get existing subscription", func(t *testing.T) {
		// Create a subscription first
		sub := &adapter.Subscription{
			Callback: "https://callback.example.com/notify",
		}

		created, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Retrieve the subscription
		retrieved, err := adp.GetSubscription(ctx, created.SubscriptionID)

		require.NoError(t, err)
		assert.Equal(t, created.SubscriptionID, retrieved.SubscriptionID)
		assert.Equal(t, created.Callback, retrieved.Callback)
	})

	t.Run("get non-existent subscription", func(t *testing.T) {
		_, err := adp.GetSubscription(ctx, "non-existent-id")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})
}

// TestDeleteSubscription tests subscription deletion.
func TestDeleteSubscription(t *testing.T) {
	adp := &OpenStackAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}

	ctx := context.Background()

	t.Run("delete existing subscription", func(t *testing.T) {
		// Create a subscription first
		sub := &adapter.Subscription{
			Callback: "https://callback.example.com/notify",
		}

		created, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Verify it exists
		_, err = adp.GetSubscription(ctx, created.SubscriptionID)
		require.NoError(t, err)

		// Delete the subscription
		err = adp.DeleteSubscription(ctx, created.SubscriptionID)
		require.NoError(t, err)

		// Verify it's gone
		_, err = adp.GetSubscription(ctx, created.SubscriptionID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})

	t.Run("delete non-existent subscription", func(t *testing.T) {
		err := adp.DeleteSubscription(ctx, "non-existent-id")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})
}

// TestListSubscriptions tests listing all subscriptions.
func TestListSubscriptions(t *testing.T) {
	adp := &OpenStackAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}

	ctx := context.Background()

	t.Run("list empty subscriptions", func(t *testing.T) {
		subs, err := adp.ListSubscriptions(ctx)

		require.NoError(t, err)
		assert.Empty(t, subs)
	})

	t.Run("list multiple subscriptions", func(t *testing.T) {
		// Create multiple subscriptions
		for i := 0; i < 3; i++ {
			sub := &adapter.Subscription{
				Callback: fmt.Sprintf("https://callback-%d.example.com/notify", i),
			}
			_, err := adp.CreateSubscription(ctx, sub)
			require.NoError(t, err)
		}

		// List all subscriptions
		subs, err := adp.ListSubscriptions(ctx)

		require.NoError(t, err)
		assert.Len(t, subs, 3)

		// Verify all have valid callbacks
		for _, sub := range subs {
			assert.NotEmpty(t, sub.SubscriptionID)
			assert.NotEmpty(t, sub.Callback)
			assert.Contains(t, sub.Callback, "callback-")
		}
	})
}

// TestSubscriptionFilters tests subscription filter handling.
func TestSubscriptionFilters(t *testing.T) {
	adp := &OpenStackAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}

	ctx := context.Background()

	tests := []struct {
		name   string
		filter *adapter.SubscriptionFilter
	}{
		{
			name: "filter by resource pool",
			filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-123",
			},
		},
		{
			name: "filter by resource type",
			filter: &adapter.SubscriptionFilter{
				ResourceTypeID: "type-456",
			},
		},
		{
			name: "filter by resource",
			filter: &adapter.SubscriptionFilter{
				ResourceID: "resource-789",
			},
		},
		{
			name: "filter by all",
			filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-123",
				ResourceTypeID: "type-456",
				ResourceID:     "resource-789",
			},
		},
		{
			name:   "no filter",
			filter: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &adapter.Subscription{
				Callback: "https://callback.example.com/notify",
				Filter:   tt.filter,
			}

			created, err := adp.CreateSubscription(ctx, sub)

			require.NoError(t, err)
			assert.NotEmpty(t, created.SubscriptionID)

			if tt.filter != nil {
				require.NotNil(t, created.Filter)
				assert.Equal(t, tt.filter.ResourcePoolID, created.Filter.ResourcePoolID)
				assert.Equal(t, tt.filter.ResourceTypeID, created.Filter.ResourceTypeID)
				assert.Equal(t, tt.filter.ResourceID, created.Filter.ResourceID)
			}

			// Clean up
			err = adp.DeleteSubscription(ctx, created.SubscriptionID)
			require.NoError(t, err)
		})
	}
}

// TestSubscriptionConcurrency tests concurrent subscription operations.
func TestSubscriptionConcurrency(t *testing.T) {
	adp := &OpenStackAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}

	ctx := context.Background()

	t.Run("concurrent creates", func(t *testing.T) {
		const numGoroutines = 10

		done := make(chan string, numGoroutines)

		// Create subscriptions concurrently
		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				sub := &adapter.Subscription{
					Callback: fmt.Sprintf("https://callback-%d.example.com/notify", index),
				}

				created, err := adp.CreateSubscription(ctx, sub)
				if err != nil {
					done <- ""
					return
				}

				done <- created.SubscriptionID
			}(i)
		}

		// Collect results
		ids := make(map[string]bool)
		for i := 0; i < numGoroutines; i++ {
			id := <-done
			if id != "" {
				ids[id] = true
			}
		}

		// All subscriptions should have unique IDs
		assert.Equal(t, numGoroutines, len(ids))
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		// Create a subscription
		sub := &adapter.Subscription{
			Callback: "https://callback.example.com/notify",
		}

		created, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		const numReaders = 10
		done := make(chan bool, numReaders)

		// Read concurrently
		for i := 0; i < numReaders; i++ {
			go func() {
				_, err := adp.GetSubscription(ctx, created.SubscriptionID)
				done <- err == nil
			}()
		}

		// All reads should succeed
		for i := 0; i < numReaders; i++ {
			success := <-done
			assert.True(t, success)
		}

		// Clean up
		err = adp.DeleteSubscription(ctx, created.SubscriptionID)
		require.NoError(t, err)
	})
}

// BenchmarkCreateSubscription benchmarks subscription creation.
func BenchmarkCreateSubscription(b *testing.B) {
	adp := &OpenStackAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}

	ctx := context.Background()

	sub := &adapter.Subscription{
		Callback: "https://callback.example.com/notify",
		Filter: &adapter.SubscriptionFilter{
			ResourcePoolID: "pool-1",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = adp.CreateSubscription(ctx, sub)
	}
}

// BenchmarkGetSubscription benchmarks subscription retrieval.
func BenchmarkGetSubscription(b *testing.B) {
	adp := &OpenStackAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}

	ctx := context.Background()

	// Create a subscription
	sub := &adapter.Subscription{
		Callback: "https://callback.example.com/notify",
	}

	created, _ := adp.CreateSubscription(ctx, sub)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = adp.GetSubscription(ctx, created.SubscriptionID)
	}
}
