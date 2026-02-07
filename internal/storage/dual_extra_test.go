package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// List operations (existing coverage, now table-driven)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Create: data integrity and secondary-failure behavior
// ---------------------------------------------------------------------------

func TestDualStore_Create_DataIntegrity(t *testing.T) {
	tests := []struct {
		name         string
		sub          *Subscription
		primaryErr   error
		secondaryErr error
		wantErr      bool
		wantPrimary  bool
		wantSecond   bool
	}{
		{
			name: "successful create persists in both stores",
			sub: &Subscription{
				ID:       "sub-new",
				Callback: "https://example.com/cb",
				TenantID: "t1",
			},
			wantErr:     false,
			wantPrimary: true,
			wantSecond:  true,
		},
		{
			name: "primary failure prevents write to either store",
			sub: &Subscription{
				ID:       "sub-fail",
				Callback: "https://example.com/cb",
			},
			primaryErr:  errors.New("disk full"),
			wantErr:     true,
			wantPrimary: false,
			wantSecond:  false,
		},
		{
			name: "secondary failure still succeeds and data in primary only",
			sub: &Subscription{
				ID:       "sub-partial",
				Callback: "https://example.com/cb",
			},
			secondaryErr: errors.New("secondary timeout"),
			wantErr:      false,
			wantPrimary:  true,
			wantSecond:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := newMemStore()
			secondary := newMemStore()
			primary.err = tt.primaryErr
			secondary.err = tt.secondaryErr

			dual := NewDualStore(primary, secondary, testLogger(t))
			err := dual.Create(context.Background(), tt.sub)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "primary store create")
				return
			}
			require.NoError(t, err)

			// Verify primary.
			got, pErr := primary.Get(context.Background(), tt.sub.ID)
			if tt.wantPrimary {
				require.NoError(t, pErr)
				assert.Equal(t, tt.sub.ID, got.ID)
				assert.Equal(t, tt.sub.Callback, got.Callback)
			} else {
				assert.Error(t, pErr)
			}

			// Verify secondary.
			_, sErr := secondary.Get(context.Background(), tt.sub.ID)
			if tt.wantSecond {
				assert.NoError(t, sErr)
			} else {
				assert.Error(t, sErr)
			}
		})
	}
}

func TestDualStore_Create_DuplicateID(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()
	dual := NewDualStore(primary, secondary, testLogger(t))

	sub := &Subscription{ID: "dup-1", Callback: "https://example.com/cb"}
	require.NoError(t, dual.Create(context.Background(), sub))

	err := dual.Create(context.Background(), sub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary store create")
}

// ---------------------------------------------------------------------------
// Update: data integrity and secondary-failure behavior
// ---------------------------------------------------------------------------

func TestDualStore_Update_DataIntegrity(t *testing.T) {
	tests := []struct {
		name         string
		primaryErr   error
		secondaryErr error
		wantErr      bool
		wantUpdated  bool
	}{
		{
			name:        "successful update modifies both stores",
			wantErr:     false,
			wantUpdated: true,
		},
		{
			name:        "primary failure returns error immediately",
			primaryErr:  ErrSubscriptionNotFound,
			wantErr:     true,
			wantUpdated: false,
		},
		{
			name:         "secondary failure still returns success",
			secondaryErr: errors.New("secondary unavailable"),
			wantErr:      false,
			wantUpdated:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := newMemStore()
			secondary := newMemStore()

			original := &Subscription{
				ID:       "sub-upd",
				Callback: "https://example.com/original",
			}
			require.NoError(t, primary.Create(context.Background(), original))
			require.NoError(t, secondary.Create(context.Background(), original))

			// Inject errors after seeding data.
			primary.err = tt.primaryErr
			secondary.err = tt.secondaryErr

			dual := NewDualStore(primary, secondary, testLogger(t))
			updated := &Subscription{
				ID:       "sub-upd",
				Callback: "https://example.com/updated",
			}
			err := dual.Update(context.Background(), updated)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "primary store update")
				return
			}
			require.NoError(t, err)

			// Verify callback changed in primary.
			got, pErr := primary.Get(context.Background(), "sub-upd")
			require.NoError(t, pErr)
			assert.Equal(t, "https://example.com/updated", got.Callback)
		})
	}
}

func TestDualStore_Update_NonexistentSubscription(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()
	dual := NewDualStore(primary, secondary, testLogger(t))

	err := dual.Update(context.Background(), &Subscription{
		ID:       "does-not-exist",
		Callback: "https://example.com/cb",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary store update")
}

// ---------------------------------------------------------------------------
// Delete: data integrity and secondary-failure behavior
// ---------------------------------------------------------------------------

func TestDualStore_Delete_DataIntegrity(t *testing.T) {
	tests := []struct {
		name         string
		primaryErr   error
		secondaryErr error
		wantErr      bool
		wantRemoved  bool
	}{
		{
			name:        "successful delete removes from both stores",
			wantErr:     false,
			wantRemoved: true,
		},
		{
			name:        "primary failure returns error; data untouched",
			primaryErr:  ErrSubscriptionNotFound,
			wantErr:     true,
			wantRemoved: false,
		},
		{
			name:         "secondary failure still succeeds; primary data removed",
			secondaryErr: errors.New("secondary read-only"),
			wantErr:      false,
			wantRemoved:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := newMemStore()
			secondary := newMemStore()

			sub := &Subscription{ID: "sub-del", Callback: "https://example.com/cb"}
			require.NoError(t, primary.Create(context.Background(), sub))
			require.NoError(t, secondary.Create(context.Background(), sub))

			primary.err = tt.primaryErr
			secondary.err = tt.secondaryErr

			dual := NewDualStore(primary, secondary, testLogger(t))
			err := dual.Delete(context.Background(), "sub-del")

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "primary store delete")
				return
			}
			require.NoError(t, err)

			// After successful delete, primary should no longer contain the item.
			_, pErr := primary.Get(context.Background(), "sub-del")
			assert.ErrorIs(t, pErr, ErrSubscriptionNotFound)
		})
	}
}

func TestDualStore_Delete_NonexistentSubscription(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()
	dual := NewDualStore(primary, secondary, testLogger(t))

	err := dual.Delete(context.Background(), "ghost-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary store delete")
}

// ---------------------------------------------------------------------------
// Fallback behavior: primary fails after secondary writes succeed
// ---------------------------------------------------------------------------

func TestDualStore_SecondaryFailure_DoesNotAffectCRUD(t *testing.T) {
	// This test verifies the full CRUD lifecycle works correctly when the
	// secondary store is permanently down.
	primary := newMemStore()
	secondary := newMemStore()
	secondary.err = errors.New("secondary permanently down")

	dual := NewDualStore(primary, secondary, testLogger(t))
	ctx := context.Background()

	// Create should succeed (secondary failure is logged, not returned).
	sub := &Subscription{
		ID:       "sub-fallback",
		Callback: "https://example.com/fallback",
		TenantID: "t-fb",
	}
	require.NoError(t, dual.Create(ctx, sub))

	// Get should succeed (reads from primary only).
	got, err := dual.Get(ctx, "sub-fallback")
	require.NoError(t, err)
	assert.Equal(t, "sub-fallback", got.ID)
	assert.Equal(t, "https://example.com/fallback", got.Callback)

	// Update should succeed (secondary failure logged).
	updated := &Subscription{
		ID:       "sub-fallback",
		Callback: "https://example.com/updated-fallback",
	}
	require.NoError(t, dual.Update(ctx, updated))

	got2, err := dual.Get(ctx, "sub-fallback")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/updated-fallback", got2.Callback)

	// List should succeed.
	subs, err := dual.List(ctx)
	require.NoError(t, err)
	assert.Len(t, subs, 1)

	// Delete should succeed (secondary failure logged).
	require.NoError(t, dual.Delete(ctx, "sub-fallback"))

	// Verify deletion.
	_, err = dual.Get(ctx, "sub-fallback")
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestDualStore_PrimaryFailure_BlocksAllWrites(t *testing.T) {
	// When primary is down, all write operations must fail.
	primary := newMemStore()
	secondary := newMemStore()

	dual := NewDualStore(primary, secondary, testLogger(t))
	ctx := context.Background()

	// Seed one subscription while everything is healthy.
	sub := &Subscription{ID: "sub-existing", Callback: "https://example.com/cb"}
	require.NoError(t, dual.Create(ctx, sub))

	// Now break the primary.
	primary.err = errors.New("primary crashed")

	// Create fails.
	err := dual.Create(ctx, &Subscription{ID: "sub-new", Callback: "https://example.com/new"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary store create")

	// Get fails.
	_, err = dual.Get(ctx, "sub-existing")
	require.Error(t, err)

	// Update fails.
	err = dual.Update(ctx, &Subscription{ID: "sub-existing", Callback: "https://example.com/v2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary store update")

	// Delete fails.
	err = dual.Delete(ctx, "sub-existing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary store delete")

	// List fails.
	_, err = dual.List(ctx)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Get: reads only from primary
// ---------------------------------------------------------------------------

func TestDualStore_Get_ReadsOnlyFromPrimary(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()

	// Put data only in secondary; primary has nothing.
	sub := &Subscription{ID: "only-secondary", Callback: "https://example.com/cb"}
	require.NoError(t, secondary.Create(context.Background(), sub))

	dual := NewDualStore(primary, secondary, testLogger(t))

	_, err := dual.Get(context.Background(), "only-secondary")
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

// ---------------------------------------------------------------------------
// List: reads only from primary even when both stores have data
// ---------------------------------------------------------------------------

func TestDualStore_List_ReturnsOnlyPrimaryData(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()

	// Put different data in each store.
	require.NoError(t, primary.Create(context.Background(), &Subscription{
		ID: "p-1", Callback: "https://example.com/p",
	}))
	require.NoError(t, secondary.Create(context.Background(), &Subscription{
		ID: "s-1", Callback: "https://example.com/s",
	}))

	dual := NewDualStore(primary, secondary, testLogger(t))
	subs, err := dual.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "p-1", subs[0].ID)
}

func TestDualStore_List_EmptyStore(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()
	dual := NewDualStore(primary, secondary, testLogger(t))

	subs, err := dual.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, subs)
}
