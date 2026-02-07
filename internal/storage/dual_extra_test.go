package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDualStore_ListByResourcePool(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()

	sub := &Subscription{
		ID:       "sub-1",
		Callback: "https://example.com/cb",
		Filter:   SubscriptionFilter{ResourcePoolID: "pool-1"},
	}
	require.NoError(t, primary.Create(context.Background(), sub))

	dual := NewDualStore(primary, secondary, testLogger(t))
	subs, err := dual.ListByResourcePool(context.Background(), "pool-1")
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "sub-1", subs[0].ID)
}

func TestDualStore_ListByResourceType(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()

	sub := &Subscription{
		ID:       "sub-1",
		Callback: "https://example.com/cb",
		Filter:   SubscriptionFilter{ResourceTypeID: "type-1"},
	}
	require.NoError(t, primary.Create(context.Background(), sub))

	dual := NewDualStore(primary, secondary, testLogger(t))
	subs, err := dual.ListByResourceType(context.Background(), "type-1")
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "sub-1", subs[0].ID)
}

func TestDualStore_ListByTenant(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()

	sub := &Subscription{
		ID:       "sub-1",
		TenantID: "tenant-1",
		Callback: "https://example.com/cb",
	}
	require.NoError(t, primary.Create(context.Background(), sub))

	dual := NewDualStore(primary, secondary, testLogger(t))
	subs, err := dual.ListByTenant(context.Background(), "tenant-1")
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "sub-1", subs[0].ID)
}
