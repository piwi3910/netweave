package adapter_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
)

func newSub(id, cb string, filter *adapter.SubscriptionFilter) *adapter.Subscription {
	return &adapter.Subscription{
		SubscriptionID: id,
		Callback:       cb,
		Filter:         filter,
	}
}

func TestInMemorySubscriptionStore_Create(t *testing.T) {
	tests := []struct {
		name    string
		sub     *adapter.Subscription
		setup   func(s *adapter.InMemorySubscriptionStore)
		wantErr error
	}{
		{
			name: "valid subscription",
			sub:  newSub("sub-1", "https://example.com/hook", nil),
		},
		{
			name:    "nil subscription",
			sub:     nil,
			wantErr: errors.New("subscription is nil"),
		},
		{
			name:    "empty ID",
			sub:     newSub("", "https://example.com/hook", nil),
			wantErr: errors.New("subscription ID is required"),
		},
		{
			name: "duplicate subscription",
			sub:  newSub("sub-1", "https://example.com/hook", nil),
			setup: func(s *adapter.InMemorySubscriptionStore) {
				_ = s.Create(context.Background(), newSub("sub-1", "https://existing.example.com/hook", nil))
			},
			wantErr: adapter.ErrSubscriptionExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := adapter.NewInMemorySubscriptionStore()
			if tt.setup != nil {
				tt.setup(store)
			}

			err := store.Create(context.Background(), tt.sub)
			if tt.wantErr == nil {
				require.NoError(t, err)
				got, err := store.Get(context.Background(), tt.sub.SubscriptionID)
				require.NoError(t, err)
				assert.Equal(t, tt.sub.Callback, got.Callback)
				return
			}
			require.Error(t, err)
			if errors.Is(tt.wantErr, adapter.ErrSubscriptionExists) {
				assert.ErrorIs(t, err, adapter.ErrSubscriptionExists)
			} else {
				assert.Contains(t, err.Error(), tt.wantErr.Error())
			}
		})
	}
}

func TestInMemorySubscriptionStore_Get(t *testing.T) {
	store := adapter.NewInMemorySubscriptionStore()
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, newSub("sub-1", "https://example.com/hook", nil)))

	got, err := store.Get(ctx, "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "sub-1", got.SubscriptionID)

	_, err = store.Get(ctx, "missing")
	assert.ErrorIs(t, err, adapter.ErrSubscriptionNotFound)
}

func TestInMemorySubscriptionStore_Update(t *testing.T) {
	store := adapter.NewInMemorySubscriptionStore()
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, newSub("sub-1", "https://old.example.com/hook", nil)))

	// Missing subscription
	err := store.Update(ctx, "missing", newSub("missing", "https://new.example.com/hook", nil))
	assert.ErrorIs(t, err, adapter.ErrSubscriptionNotFound)

	// Nil subscription
	err = store.Update(ctx, "sub-1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subscription is nil")

	// Successful update
	updated := newSub("sub-1", "https://new.example.com/hook", nil)
	require.NoError(t, store.Update(ctx, "sub-1", updated))

	got, err := store.Get(ctx, "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "https://new.example.com/hook", got.Callback)
}

func TestInMemorySubscriptionStore_Delete(t *testing.T) {
	store := adapter.NewInMemorySubscriptionStore()
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, newSub("sub-1", "https://example.com/hook", nil)))

	assert.ErrorIs(t, store.Delete(ctx, "missing"), adapter.ErrSubscriptionNotFound)
	require.NoError(t, store.Delete(ctx, "sub-1"))

	_, err := store.Get(ctx, "sub-1")
	assert.ErrorIs(t, err, adapter.ErrSubscriptionNotFound)
}

func TestInMemorySubscriptionStore_List(t *testing.T) {
	store := adapter.NewInMemorySubscriptionStore()
	ctx := context.Background()

	f1 := &adapter.SubscriptionFilter{ResourcePoolID: "pool-a", ResourceTypeID: "type-a"}
	f2 := &adapter.SubscriptionFilter{ResourcePoolID: "pool-b", ResourceTypeID: "type-b", ResourceID: "res-b"}

	require.NoError(t, store.Create(ctx, newSub("sub-1", "https://a.example.com", f1)))
	require.NoError(t, store.Create(ctx, newSub("sub-2", "https://b.example.com", f2)))
	require.NoError(t, store.Create(ctx, newSub("sub-3", "https://c.example.com", nil)))

	tests := []struct {
		name    string
		filter  *adapter.SubscriptionFilter
		wantLen int
	}{
		{name: "nil filter returns all", filter: nil, wantLen: 3},
		{name: "pool filter", filter: &adapter.SubscriptionFilter{ResourcePoolID: "pool-a"}, wantLen: 1},
		{name: "type filter", filter: &adapter.SubscriptionFilter{ResourceTypeID: "type-b"}, wantLen: 1},
		{name: "resource filter", filter: &adapter.SubscriptionFilter{ResourceID: "res-b"}, wantLen: 1},
		{name: "no matches", filter: &adapter.SubscriptionFilter{ResourcePoolID: "nonexistent"}, wantLen: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.List(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}

func TestInMemorySubscriptionStore_ContextCancellation(t *testing.T) {
	store := adapter.NewInMemorySubscriptionStore()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.Error(t, store.Create(ctx, newSub("sub-1", "https://example.com", nil)))
	_, err := store.Get(ctx, "sub-1")
	assert.Error(t, err)
	assert.Error(t, store.Update(ctx, "sub-1", newSub("sub-1", "https://example.com", nil)))
	assert.Error(t, store.Delete(ctx, "sub-1"))
	_, err = store.List(ctx, nil)
	assert.Error(t, err)
}

func TestInMemorySubscriptionStore_LenReset(t *testing.T) {
	store := adapter.NewInMemorySubscriptionStore()
	ctx := context.Background()
	assert.Equal(t, 0, store.Len())

	require.NoError(t, store.Create(ctx, newSub("sub-1", "https://a.example.com", nil)))
	require.NoError(t, store.Create(ctx, newSub("sub-2", "https://b.example.com", nil)))
	assert.Equal(t, 2, store.Len())

	store.Reset()
	assert.Equal(t, 0, store.Len())
}

func TestInMemorySubscriptionStore_RawMap(t *testing.T) {
	store := adapter.NewInMemorySubscriptionStore()
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, newSub("sub-1", "https://a.example.com", nil)))

	m := store.RawMap()
	require.NotNil(t, m)
	assert.Contains(t, m, "sub-1")

	// Seeding via the raw map is observable via Get.
	m["sub-2"] = newSub("sub-2", "https://b.example.com", nil)
	got, err := store.Get(ctx, "sub-2")
	require.NoError(t, err)
	assert.Equal(t, "https://b.example.com", got.Callback)
}

func TestInMemorySubscriptionStore_ConcurrentAccess(t *testing.T) {
	store := adapter.NewInMemorySubscriptionStore()
	ctx := context.Background()

	const goroutines = 50
	const perGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				id := fmt.Sprintf("sub-%d-%d", i, j)
				if err := store.Create(ctx, newSub(id, "https://example.com", nil)); err != nil {
					t.Errorf("create: %v", err)
					return
				}
				if _, err := store.Get(ctx, id); err != nil {
					t.Errorf("get: %v", err)
					return
				}
				if err := store.Update(ctx, id, newSub(id, "https://updated.example.com", nil)); err != nil {
					t.Errorf("update: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	assert.Equal(t, goroutines*perGoroutine, store.Len())

	// Concurrent list + delete
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < goroutines; i++ {
			_, err := store.List(ctx, nil)
			if err != nil {
				t.Errorf("list: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < goroutines; i++ {
			for j := 0; j < perGoroutine; j++ {
				_ = store.Delete(ctx, fmt.Sprintf("sub-%d-%d", i, j))
			}
		}
	}()
	wg.Wait()

	assert.Equal(t, 0, store.Len())
}

// --- StorageBackedSubscriptionStore tests (with fake backend) ---

type fakeBackend struct {
	mu   sync.Mutex
	subs map[string]*adapter.Subscription
	// counters for verifying delegation
	createCalls int
	getCalls    int
	updateCalls int
	deleteCalls int
	listCalls   int
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{subs: make(map[string]*adapter.Subscription)}
}

func (f *fakeBackend) Create(_ context.Context, id, consumerID, callback string, filter *adapter.SubscriptionFilter) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if _, ok := f.subs[id]; ok {
		return fmt.Errorf("%w: %s", adapter.ErrSubscriptionExists, id)
	}
	f.subs[id] = &adapter.Subscription{
		SubscriptionID:         id,
		ConsumerSubscriptionID: consumerID,
		Callback:               callback,
		Filter:                 filter,
	}
	return nil
}

func (f *fakeBackend) Get(_ context.Context, id string) (*adapter.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	sub, ok := f.subs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}
	return sub, nil
}

func (f *fakeBackend) Update(_ context.Context, id, consumerID, callback string, filter *adapter.SubscriptionFilter) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls++
	if _, ok := f.subs[id]; !ok {
		return fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}
	f.subs[id] = &adapter.Subscription{
		SubscriptionID:         id,
		ConsumerSubscriptionID: consumerID,
		Callback:               callback,
		Filter:                 filter,
	}
	return nil
}

func (f *fakeBackend) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls++
	if _, ok := f.subs[id]; !ok {
		return fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}
	delete(f.subs, id)
	return nil
}

func (f *fakeBackend) List(_ context.Context, _ *adapter.SubscriptionFilter) ([]*adapter.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	result := make([]*adapter.Subscription, 0, len(f.subs))
	for _, sub := range f.subs {
		result = append(result, sub)
	}
	return result, nil
}

func TestStorageBackedSubscriptionStore_DelegatesCRUD(t *testing.T) {
	backend := newFakeBackend()
	store := adapter.NewStorageBackedSubscriptionStore(backend)

	ctx := context.Background()
	sub := newSub("sub-1", "https://example.com", nil)

	require.NoError(t, store.Create(ctx, sub))
	assert.Equal(t, 1, backend.createCalls)

	got, err := store.Get(ctx, "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "sub-1", got.SubscriptionID)
	assert.Equal(t, 1, backend.getCalls)

	require.NoError(t, store.Update(ctx, "sub-1", newSub("sub-1", "https://updated.example.com", nil)))
	assert.Equal(t, 1, backend.updateCalls)

	list, err := store.List(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, 1, backend.listCalls)

	require.NoError(t, store.Delete(ctx, "sub-1"))
	assert.Equal(t, 1, backend.deleteCalls)

	_, err = store.Get(ctx, "sub-1")
	assert.ErrorIs(t, err, adapter.ErrSubscriptionNotFound)
}

func TestStorageBackedSubscriptionStore_Validation(t *testing.T) {
	backend := newFakeBackend()
	store := adapter.NewStorageBackedSubscriptionStore(backend)
	ctx := context.Background()

	assert.Error(t, store.Create(ctx, nil))
	assert.Error(t, store.Create(ctx, newSub("", "https://example.com", nil)))
	assert.Error(t, store.Update(ctx, "sub-1", nil))
	// No backend calls should have been made for invalid input.
	assert.Equal(t, 0, backend.createCalls)
	assert.Equal(t, 0, backend.updateCalls)
}

func TestStorageBackedSubscriptionStore_ContextCancellation(t *testing.T) {
	backend := newFakeBackend()
	store := adapter.NewStorageBackedSubscriptionStore(backend)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.Error(t, store.Create(ctx, newSub("sub-1", "https://example.com", nil)))
	_, err := store.Get(ctx, "sub-1")
	assert.Error(t, err)
	assert.Error(t, store.Update(ctx, "sub-1", newSub("sub-1", "https://example.com", nil)))
	assert.Error(t, store.Delete(ctx, "sub-1"))
	_, err = store.List(ctx, nil)
	assert.Error(t, err)

	// Backend must not be invoked when context is already canceled.
	assert.Equal(t, 0, backend.createCalls)
	assert.Equal(t, 0, backend.getCalls)
	assert.Equal(t, 0, backend.updateCalls)
	assert.Equal(t, 0, backend.deleteCalls)
	assert.Equal(t, 0, backend.listCalls)
}
