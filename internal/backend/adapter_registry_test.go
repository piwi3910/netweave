package backend_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/backend"
)

func newTestRegistry(t *testing.T) *backend.AdapterRegistry {
	t.Helper()
	factory := backend.NewAdapterFactory()
	logger := zap.NewNop()
	return backend.NewAdapterRegistry(factory, logger)
}

func TestAdapterRegistry_GetAdapter(t *testing.T) {
	t.Run("returns error for missing adapter", func(t *testing.T) {
		registry := newTestRegistry(t)
		adp, err := registry.GetAdapter("nonexistent")
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.ErrorIs(t, err, backend.ErrAdapterNotFound)
	})

	t.Run("returns adapter after load", func(t *testing.T) {
		registry := newTestRegistry(t)
		store := &fakeBackendStore{
			backends: []*backend.Instance{
				{
					ID:          "backend-1",
					AdapterType: "mock",
					Status:      "active",
				},
			},
		}

		loaded, err := registry.LoadFromStore(
			context.Background(), store,
		)
		require.NoError(t, err)
		assert.Equal(t, 1, loaded)
		assert.Equal(t, 1, registry.Count())

		adp, err := registry.GetAdapter("backend-1")
		require.NoError(t, err)
		require.NotNil(t, adp)
		assert.Equal(t, "mock", adp.Name())
	})
}

func TestAdapterRegistry_LoadFromStore(t *testing.T) {
	t.Run("loads multiple backends", func(t *testing.T) {
		registry := newTestRegistry(t)
		store := &fakeBackendStore{
			backends: []*backend.Instance{
				{
					ID:          "b1",
					AdapterType: "mock",
					Status:      "active",
				},
				{
					ID:          "b2",
					AdapterType: "mock",
					Status:      "active",
				},
				{
					ID:          "b3",
					AdapterType: "mock",
					Status:      "active",
				},
			},
		}

		loaded, err := registry.LoadFromStore(
			context.Background(), store,
		)
		require.NoError(t, err)
		assert.Equal(t, 3, loaded)
		assert.Equal(t, 3, registry.Count())
	})

	t.Run("skips disabled and error backends", func(t *testing.T) {
		registry := newTestRegistry(t)
		store := &fakeBackendStore{
			backends: []*backend.Instance{
				{
					ID:          "b1",
					AdapterType: "mock",
					Status:      "active",
				},
				{
					ID:          "b2",
					AdapterType: "mock",
					Status:      "disabled",
				},
				{
					ID:          "b3",
					AdapterType: "mock",
					Status:      "error",
				},
			},
		}

		loaded, err := registry.LoadFromStore(
			context.Background(), store,
		)
		require.NoError(t, err)
		assert.Equal(t, 1, loaded)
		assert.Equal(t, 1, registry.Count())
	})

	t.Run("skips unsupported adapter types", func(t *testing.T) {
		registry := newTestRegistry(t)
		store := &fakeBackendStore{
			backends: []*backend.Instance{
				{
					ID:          "b1",
					AdapterType: "mock",
					Status:      "active",
				},
				{
					ID:          "b2",
					AdapterType: "unsupported",
					Status:      "active",
				},
			},
		}

		loaded, err := registry.LoadFromStore(
			context.Background(), store,
		)
		require.NoError(t, err)
		assert.Equal(t, 1, loaded)
	})
}

func TestAdapterRegistry_GetAdapterForTenant(t *testing.T) {
	t.Run("returns error when no backend access", func(t *testing.T) {
		registry := newTestRegistry(t)
		store := &fakeBackendStore{
			accesses: []*backend.Access{},
		}

		adp, err := registry.GetAdapterForTenant(
			context.Background(), "tenant-1", store,
		)
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.ErrorIs(t, err, backend.ErrNoBackendAccess)
	})

	t.Run("returns adapter for tenant with access", func(t *testing.T) {
		registry := newTestRegistry(t)
		store := &fakeBackendStore{
			backends: []*backend.Instance{
				{
					ID:          "b1",
					AdapterType: "mock",
					Status:      "active",
				},
			},
			accesses: []*backend.Access{
				{TenantID: "tenant-1", BackendID: "b1"},
			},
		}

		loaded, err := registry.LoadFromStore(
			context.Background(), store,
		)
		require.NoError(t, err)
		assert.Equal(t, 1, loaded)

		adp, err := registry.GetAdapterForTenant(
			context.Background(), "tenant-1", store,
		)
		require.NoError(t, err)
		require.NotNil(t, adp)
		assert.Equal(t, "mock", adp.Name())
	})

	t.Run("returns error when backend not in registry", func(t *testing.T) {
		registry := newTestRegistry(t)
		store := &fakeBackendStore{
			accesses: []*backend.Access{
				{
					TenantID:  "tenant-1",
					BackendID: "nonexistent-backend",
				},
			},
		}

		adp, err := registry.GetAdapterForTenant(
			context.Background(), "tenant-1", store,
		)
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.ErrorIs(t, err, backend.ErrAdapterNotFound)
	})
}

func TestAdapterRegistry_RefreshAdapter(t *testing.T) {
	t.Run("refreshes existing adapter", func(t *testing.T) {
		registry := newTestRegistry(t)
		store := &fakeBackendStore{
			backends: []*backend.Instance{
				{
					ID:          "b1",
					AdapterType: "mock",
					Status:      "active",
				},
			},
		}

		loaded, err := registry.LoadFromStore(
			context.Background(), store,
		)
		require.NoError(t, err)
		assert.Equal(t, 1, loaded)

		err = registry.RefreshAdapter(
			context.Background(), store, "b1",
		)
		require.NoError(t, err)
		assert.Equal(t, 1, registry.Count())
	})

	t.Run("registers new adapter without prior load", func(t *testing.T) {
		registry := newTestRegistry(t)
		assert.Equal(t, 0, registry.Count())

		store := &fakeBackendStore{
			backends: []*backend.Instance{
				{
					ID:          "new-backend",
					AdapterType: "mock",
					Status:      "active",
				},
			},
		}

		err := registry.RefreshAdapter(
			context.Background(), store, "new-backend",
		)
		require.NoError(t, err)
		assert.Equal(t, 1, registry.Count())

		adp, err := registry.GetAdapter("new-backend")
		require.NoError(t, err)
		require.NotNil(t, adp)
		assert.Equal(t, "mock", adp.Name())
	})
}

func TestAdapterRegistry_RemoveAdapter(t *testing.T) {
	t.Run("removes existing adapter", func(t *testing.T) {
		registry := newTestRegistry(t)
		store := &fakeBackendStore{
			backends: []*backend.Instance{
				{
					ID:          "b1",
					AdapterType: "mock",
					Status:      "active",
				},
			},
		}

		loaded, err := registry.LoadFromStore(
			context.Background(), store,
		)
		require.NoError(t, err)
		assert.Equal(t, 1, loaded)

		registry.RemoveAdapter("b1")
		assert.Equal(t, 0, registry.Count())

		_, err = registry.GetAdapter("b1")
		assert.ErrorIs(t, err, backend.ErrAdapterNotFound)
	})

	t.Run("no-op for non-existent adapter", func(t *testing.T) {
		registry := newTestRegistry(t)
		assert.Equal(t, 0, registry.Count())

		registry.RemoveAdapter("nonexistent")
		assert.Equal(t, 0, registry.Count())
	})
}

func TestAdapterRegistry_Close(t *testing.T) {
	t.Run("closes all adapters", func(t *testing.T) {
		registry := newTestRegistry(t)
		store := &fakeBackendStore{
			backends: []*backend.Instance{
				{
					ID:          "b1",
					AdapterType: "mock",
					Status:      "active",
				},
				{
					ID:          "b2",
					AdapterType: "mock",
					Status:      "active",
				},
			},
		}

		loaded, err := registry.LoadFromStore(
			context.Background(), store,
		)
		require.NoError(t, err)
		assert.Equal(t, 2, loaded)

		err = registry.Close()
		require.NoError(t, err)
		assert.Equal(t, 0, registry.Count())
	})
}

// fakeBackendStore implements backend.Store for testing the adapter registry.
type fakeBackendStore struct {
	backends []*backend.Instance
	accesses []*backend.Access
}

func (s *fakeBackendStore) CreateBackend(
	_ context.Context, _ *backend.Instance,
) error {
	return nil
}

func (s *fakeBackendStore) GetBackend(
	_ context.Context, id string,
) (*backend.Instance, error) {
	for _, b := range s.backends {
		if b.ID == id {
			return b, nil
		}
	}
	return nil, backend.ErrAdapterNotFound
}

func (s *fakeBackendStore) UpdateBackend(
	_ context.Context, _ *backend.Instance,
) error {
	return nil
}

func (s *fakeBackendStore) DeleteBackend(
	_ context.Context, _ string,
) error {
	return nil
}

func (s *fakeBackendStore) ListBackends(
	_ context.Context,
) ([]*backend.Instance, error) {
	return s.backends, nil
}

func (s *fakeBackendStore) ListBackendsByCategory(
	_ context.Context, _ string,
) ([]*backend.Instance, error) {
	return s.backends, nil
}

func (s *fakeBackendStore) UpdateBackendStatus(
	_ context.Context, _, _, _ string,
) error {
	return nil
}

func (s *fakeBackendStore) CreateBackendLink(
	_ context.Context, _, _ string,
) error {
	return nil
}

func (s *fakeBackendStore) DeleteBackendLink(
	_ context.Context, _, _ string,
) error {
	return nil
}

func (s *fakeBackendStore) ListDMSLinksByIMS(
	_ context.Context, _ string,
) ([]*backend.Instance, error) {
	return nil, nil
}

func (s *fakeBackendStore) ListIMSLinksByDMS(
	_ context.Context, _ string,
) ([]*backend.Instance, error) {
	return nil, nil
}

func (s *fakeBackendStore) CreateBackendAccess(
	_ context.Context, _ *backend.Access,
) error {
	return nil
}

func (s *fakeBackendStore) GetBackendAccess(
	_ context.Context, _ string,
) (*backend.Access, error) {
	return &backend.Access{}, nil
}

func (s *fakeBackendStore) UpdateBackendAccess(
	_ context.Context, _ *backend.Access,
) error {
	return nil
}

func (s *fakeBackendStore) DeleteBackendAccess(
	_ context.Context, _ string,
) error {
	return nil
}

func (s *fakeBackendStore) ListBackendAccessByTenant(
	_ context.Context, _ string,
) ([]*backend.Access, error) {
	return s.accesses, nil
}

func (s *fakeBackendStore) ListBackendAccessByBackend(
	_ context.Context, _ string,
) ([]*backend.Access, error) {
	return s.accesses, nil
}

func (s *fakeBackendStore) Close() error {
	return nil
}

func (s *fakeBackendStore) Ping(_ context.Context) error {
	return nil
}
