package starlingx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
)

func createAdapterWithStore(t *testing.T) (*Adapter, func()) {
	t.Helper()

	store := newMockStore()
	keystoneURL, starlingxURL, cleanup := createMockServers(t, nil)

	adp, err := New(&Config{
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
		adp.Close()
		cleanup()
	}

	return adp, fullCleanup
}

func TestCreateSubscription(t *testing.T) {
	adp, cleanup := createAdapterWithStore(t)
	defer cleanup()

	ctx := context.Background()

	sub := &adapter.Subscription{
		Callback:               "https://smo.example.com/notify",
		ConsumerSubscriptionID: "consumer-123",
		Filter: &adapter.SubscriptionFilter{
			ResourcePoolID: "pool-1",
		},
	}

	created, err := adp.CreateSubscription(ctx, sub)
	require.NoError(t, err)
	require.NotNil(t, created)

	assert.NotEmpty(t, created.SubscriptionID)
	assert.Equal(t, "https://smo.example.com/notify", created.Callback)
	assert.Equal(t, "consumer-123", created.ConsumerSubscriptionID)
	assert.Equal(t, "pool-1", created.Filter.ResourcePoolID)
}

func TestGetSubscription(t *testing.T) {
	adp, cleanup := createAdapterWithStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create subscription first
	sub := &adapter.Subscription{
		SubscriptionID: "sub-123",
		Callback:       "https://smo.example.com/notify",
	}

	_, err := adp.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	// Retrieve it
	retrieved, err := adp.GetSubscription(ctx, "sub-123")
	require.NoError(t, err)
	assert.Equal(t, "sub-123", retrieved.SubscriptionID)
	assert.Equal(t, "https://smo.example.com/notify", retrieved.Callback)
}

func TestGetSubscription_NotFound(t *testing.T) {
	adp, cleanup := createAdapterWithStore(t)
	defer cleanup()

	ctx := context.Background()
	_, err := adp.GetSubscription(ctx, "non-existing")

	require.Error(t, err)
	require.ErrorIs(t, err, adapter.ErrSubscriptionNotFound)
}

func TestUpdateSubscription(t *testing.T) {
	adp, cleanup := createAdapterWithStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create subscription
	sub := &adapter.Subscription{
		SubscriptionID: "sub-123",
		Callback:       "https://smo.example.com/notify",
	}

	_, err := adp.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	// Update it
	update := &adapter.Subscription{
		Callback: "https://smo.example.com/new-notify",
		Filter: &adapter.SubscriptionFilter{
			ResourceTypeID: "type-1",
		},
	}

	updated, err := adp.UpdateSubscription(ctx, "sub-123", update)
	require.NoError(t, err)
	assert.Equal(t, "https://smo.example.com/new-notify", updated.Callback)
	assert.Equal(t, "type-1", updated.Filter.ResourceTypeID)
}

func TestDeleteSubscription(t *testing.T) {
	adp, cleanup := createAdapterWithStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create subscription
	sub := &adapter.Subscription{
		SubscriptionID: "sub-123",
		Callback:       "https://smo.example.com/notify",
	}

	_, err := adp.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	// Delete it
	err = adp.DeleteSubscription(ctx, "sub-123")
	require.NoError(t, err)

	// Verify it's gone
	_, err = adp.GetSubscription(ctx, "sub-123")
	require.Error(t, err)
	require.ErrorIs(t, err, adapter.ErrSubscriptionNotFound)
}

func TestSubscription_NoStore(t *testing.T) {
	adp, cleanup := createTestAdapter(t, nil)
	defer cleanup()

	ctx := context.Background()

	// All subscription operations should return ErrNotImplemented when no store
	_, err := adp.CreateSubscription(ctx, &adapter.Subscription{})
	require.ErrorIs(t, err, adapter.ErrNotImplemented)

	_, err = adp.GetSubscription(ctx, "sub-123")
	require.ErrorIs(t, err, adapter.ErrNotImplemented)

	_, err = adp.UpdateSubscription(ctx, "sub-123", &adapter.Subscription{})
	require.ErrorIs(t, err, adapter.ErrNotImplemented)

	err = adp.DeleteSubscription(ctx, "sub-123")
	require.ErrorIs(t, err, adapter.ErrNotImplemented)
}
