package starlingx_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/starlingx"
)

func createAdapterWithStore(t *testing.T) (*starlingx.Adapter, func()) {
	t.Helper()

	store := newMockStore()
	keystoneURL, starlingxURL, cleanup := starlingx.CreateMockServers(t, nil)

	adp, err := starlingx.New(&starlingx.Config{
		Endpoint:            starlingxURL,
		KeystoneEndpoint:    keystoneURL,
		Username:            "testuser",
		Password:            "testpass",
		OCloudID:            "test-ocloud",
		DeploymentManagerID: "test-dm",
		Store:               store,
	})

	if err != nil {
		cleanup()
		t.Fatalf("failed to create adapter: %v", err)
	}

	fullCleanup := func() {
		if closeErr := adp.Close(); closeErr != nil {
			t.Logf("close error: %v", closeErr)
		}
		cleanup()
	}

	return adp, fullCleanup
}

func TestCreateSubscription(t *testing.T) {
	adp, cleanup := createAdapterWithStore(t)
	defer cleanup()

	ctx := context.Background()

	subscription := &adapter.Subscription{
		Callback:               "https://example.com/notify",
		ConsumerSubscriptionID: "consumer-sub-1",
		Filter: &adapter.SubscriptionFilter{
			ResourcePoolID: "pool-1",
		},
	}

	created, err := adp.CreateSubscription(ctx, subscription)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotEmpty(t, created.SubscriptionID)
	assert.Equal(t, subscription.Callback, created.Callback)
	assert.Equal(t, subscription.ConsumerSubscriptionID, created.ConsumerSubscriptionID)
}

func TestGetSubscription(t *testing.T) {
	adp, cleanup := createAdapterWithStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a subscription first
	subscription := &adapter.Subscription{
		Callback: "https://example.com/notify",
	}

	created, err := adp.CreateSubscription(ctx, subscription)
	require.NoError(t, err)

	// Get the subscription
	retrieved, err := adp.GetSubscription(ctx, created.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, created.SubscriptionID, retrieved.SubscriptionID)
	assert.Equal(t, created.Callback, retrieved.Callback)
}

func TestGetSubscription_NotFound(t *testing.T) {
	adp, cleanup := createAdapterWithStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := adp.GetSubscription(ctx, "nonexistent-id")
	require.Error(t, err)
	assert.Equal(t, adapter.ErrSubscriptionNotFound, err)
}

func TestUpdateSubscription(t *testing.T) {
	adp, cleanup := createAdapterWithStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a subscription first
	subscription := &adapter.Subscription{
		Callback: "https://example.com/notify",
	}

	created, err := adp.CreateSubscription(ctx, subscription)
	require.NoError(t, err)

	// Update the subscription
	update := &adapter.Subscription{
		Callback: "https://example.com/updated-notify",
	}

	updated, err := adp.UpdateSubscription(ctx, created.SubscriptionID, update)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, created.SubscriptionID, updated.SubscriptionID)
	assert.Equal(t, update.Callback, updated.Callback)
}

func TestDeleteSubscription(t *testing.T) {
	adp, cleanup := createAdapterWithStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a subscription first
	subscription := &adapter.Subscription{
		Callback: "https://example.com/notify",
	}

	created, err := adp.CreateSubscription(ctx, subscription)
	require.NoError(t, err)

	// Delete the subscription
	err = adp.DeleteSubscription(ctx, created.SubscriptionID)
	require.NoError(t, err)

	// Verify it's gone
	_, err = adp.GetSubscription(ctx, created.SubscriptionID)
	require.Error(t, err)
	assert.Equal(t, adapter.ErrSubscriptionNotFound, err)
}

func TestSubscriptions_NoStore(t *testing.T) {
	adp, cleanup := starlingx.CreateTestAdapter(t, nil)
	defer cleanup()

	ctx := context.Background()

	// All subscription operations should return ErrNotImplemented
	subscription := &adapter.Subscription{
		Callback: "https://example.com/notify",
	}

	_, err := adp.CreateSubscription(ctx, subscription)
	assert.Equal(t, adapter.ErrNotImplemented, err)

	_, err = adp.GetSubscription(ctx, "some-id")
	assert.Equal(t, adapter.ErrNotImplemented, err)

	_, err = adp.UpdateSubscription(ctx, "some-id", subscription)
	assert.Equal(t, adapter.ErrNotImplemented, err)

	err = adp.DeleteSubscription(ctx, "some-id")
	assert.Equal(t, adapter.ErrNotImplemented, err)
}
