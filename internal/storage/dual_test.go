package storage

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// memStore is a simple in-memory Store implementation for testing.
type memStore struct {
	mu    sync.RWMutex
	subs  map[string]*Subscription
	err   error // forced error for testing
	pings int
}

func newMemStore() *memStore {
	return &memStore{subs: make(map[string]*Subscription)}
}

func (m *memStore) Create(_ context.Context, sub *Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	if _, exists := m.subs[sub.ID]; exists {
		return ErrSubscriptionExists
	}
	// Store a copy to prevent test mutation issues.
	cp := *sub
	m.subs[sub.ID] = &cp
	return nil
}

func (m *memStore) Get(_ context.Context, id string) (*Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.err != nil {
		return nil, m.err
	}
	s, ok := m.subs[id]
	if !ok {
		return nil, ErrSubscriptionNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *memStore) Update(_ context.Context, sub *Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	if _, exists := m.subs[sub.ID]; !exists {
		return ErrSubscriptionNotFound
	}
	cp := *sub
	m.subs[sub.ID] = &cp
	return nil
}

func (m *memStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	if _, exists := m.subs[id]; !exists {
		return ErrSubscriptionNotFound
	}
	delete(m.subs, id)
	return nil
}

func (m *memStore) List(_ context.Context) ([]*Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*Subscription, 0, len(m.subs))
	for _, s := range m.subs {
		cp := *s
		result = append(result, &cp)
	}
	return result, nil
}

func (m *memStore) ListByResourcePool(_ context.Context, _ string) ([]*Subscription, error) {
	return m.List(context.Background())
}

func (m *memStore) ListByResourceType(_ context.Context, _ string) ([]*Subscription, error) {
	return m.List(context.Background())
}

func (m *memStore) ListByTenant(_ context.Context, _ string) ([]*Subscription, error) {
	return m.List(context.Background())
}

func (m *memStore) Close() error {
	return m.err
}

func (m *memStore) Ping(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pings++
	return m.err
}

func testLogger(t *testing.T) *zap.Logger {
	t.Helper()
	return zaptest.NewLogger(t)
}

func TestDualStore_Create(t *testing.T) {
	tests := []struct {
		name         string
		primaryErr   error
		secondaryErr error
		wantErr      bool
		wantInBoth   bool
	}{
		{
			name:       "writes to both stores",
			wantErr:    false,
			wantInBoth: true,
		},
		{
			name:       "primary error fails the operation",
			primaryErr: errors.New("primary down"),
			wantErr:    true,
			wantInBoth: false,
		},
		{
			name:         "secondary error is logged but not returned",
			secondaryErr: errors.New("secondary down"),
			wantErr:      false,
			wantInBoth:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := newMemStore()
			secondary := newMemStore()
			primary.err = tt.primaryErr
			secondary.err = tt.secondaryErr

			dual := NewDualStore(primary, secondary, testLogger(t))

			sub := &Subscription{ID: "sub-1", Callback: "https://example.com/cb"}
			err := dual.Create(context.Background(), sub)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Primary should always have it when no error.
			_, pErr := primary.Get(context.Background(), "sub-1")
			assert.NoError(t, pErr)

			if tt.wantInBoth {
				_, sErr := secondary.Get(context.Background(), "sub-1")
				assert.NoError(t, sErr)
			}
		})
	}
}

func TestDualStore_Get(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()
	dual := NewDualStore(primary, secondary, testLogger(t))

	sub := &Subscription{ID: "sub-1", Callback: "https://example.com/cb"}
	require.NoError(t, primary.Create(context.Background(), sub))

	got, err := dual.Get(context.Background(), "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "sub-1", got.ID)

	// Reading from dual should not touch secondary.
	_, sErr := secondary.Get(context.Background(), "sub-1")
	assert.ErrorIs(t, sErr, ErrSubscriptionNotFound)
}

func TestDualStore_Update(t *testing.T) {
	tests := []struct {
		name         string
		primaryErr   error
		secondaryErr error
		wantErr      bool
	}{
		{
			name:    "updates both stores",
			wantErr: false,
		},
		{
			name:       "primary error fails the operation",
			primaryErr: ErrSubscriptionNotFound,
			wantErr:    true,
		},
		{
			name:         "secondary error is logged but not returned",
			secondaryErr: ErrSubscriptionNotFound,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := newMemStore()
			secondary := newMemStore()

			sub := &Subscription{ID: "sub-1", Callback: "https://example.com/cb"}
			require.NoError(t, primary.Create(context.Background(), sub))
			require.NoError(t, secondary.Create(context.Background(), sub))

			// Set errors after initial create.
			primary.err = tt.primaryErr
			secondary.err = tt.secondaryErr

			dual := NewDualStore(primary, secondary, testLogger(t))

			updated := &Subscription{ID: "sub-1", Callback: "https://example.com/updated"}
			err := dual.Update(context.Background(), updated)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDualStore_Delete(t *testing.T) {
	tests := []struct {
		name         string
		primaryErr   error
		secondaryErr error
		wantErr      bool
	}{
		{
			name:    "deletes from both stores",
			wantErr: false,
		},
		{
			name:       "primary error fails the operation",
			primaryErr: ErrSubscriptionNotFound,
			wantErr:    true,
		},
		{
			name:         "secondary error is logged but not returned",
			secondaryErr: ErrSubscriptionNotFound,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := newMemStore()
			secondary := newMemStore()

			sub := &Subscription{ID: "sub-1", Callback: "https://example.com/cb"}
			require.NoError(t, primary.Create(context.Background(), sub))
			require.NoError(t, secondary.Create(context.Background(), sub))

			primary.err = tt.primaryErr
			secondary.err = tt.secondaryErr

			dual := NewDualStore(primary, secondary, testLogger(t))
			err := dual.Delete(context.Background(), "sub-1")

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDualStore_List(t *testing.T) {
	primary := newMemStore()
	secondary := newMemStore()

	sub := &Subscription{ID: "sub-1", Callback: "https://example.com/cb"}
	require.NoError(t, primary.Create(context.Background(), sub))

	dual := NewDualStore(primary, secondary, testLogger(t))
	subs, err := dual.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, subs, 1)
}

func TestDualStore_Ping(t *testing.T) {
	tests := []struct {
		name         string
		primaryErr   error
		secondaryErr error
		wantErr      bool
	}{
		{
			name:    "both healthy",
			wantErr: false,
		},
		{
			name:       "primary unhealthy",
			primaryErr: errors.New("down"),
			wantErr:    true,
		},
		{
			name:         "secondary unhealthy",
			secondaryErr: errors.New("down"),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := newMemStore()
			secondary := newMemStore()
			primary.err = tt.primaryErr
			secondary.err = tt.secondaryErr

			dual := NewDualStore(primary, secondary, testLogger(t))
			err := dual.Ping(context.Background())

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDualStore_Close(t *testing.T) {
	t.Run("both close without error", func(t *testing.T) {
		primary := newMemStore()
		secondary := newMemStore()
		dual := NewDualStore(primary, secondary, testLogger(t))
		require.NoError(t, dual.Close())
	})

	t.Run("primary close error", func(t *testing.T) {
		primary := newMemStore()
		primary.err = errors.New("close fail")
		secondary := newMemStore()
		dual := NewDualStore(primary, secondary, testLogger(t))
		err := dual.Close()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "primary close")
	})

	t.Run("secondary close error", func(t *testing.T) {
		primary := newMemStore()
		secondary := newMemStore()
		secondary.err = errors.New("close fail")
		dual := NewDualStore(primary, secondary, testLogger(t))
		err := dual.Close()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "secondary close")
	})

	t.Run("both close error", func(t *testing.T) {
		primary := newMemStore()
		primary.err = errors.New("p fail")
		secondary := newMemStore()
		secondary.err = errors.New("s fail")
		dual := NewDualStore(primary, secondary, testLogger(t))
		err := dual.Close()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "primary close")
		assert.Contains(t, err.Error(), "secondary close")
	})
}

func TestNewDualStore_PanicOnNil(t *testing.T) {
	t.Run("nil primary panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewDualStore(nil, newMemStore(), testLogger(t))
		})
	})
	t.Run("nil secondary panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewDualStore(newMemStore(), nil, testLogger(t))
		})
	})
	t.Run("nil logger panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewDualStore(newMemStore(), newMemStore(), nil)
		})
	})
}
