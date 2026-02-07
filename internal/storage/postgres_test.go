package storage_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/piwi3910/netweave/internal/database"
	"github.com/piwi3910/netweave/internal/storage"
)

const (
	pgTestImage    = "postgres:16-alpine"
	pgTestDB       = "netweave_test"
	pgTestUser     = "testuser"
	pgTestPassword = "testpass"
)

// pgTestContainer represents a running PostgreSQL test container.
type pgTestContainer struct {
	host string
	port int
}

// setupPgContainer starts a PostgreSQL container with migrations applied.
func setupPgContainer(t *testing.T) (*pgTestContainer, *database.DB) {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        pgTestImage,
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     pgTestUser,
			"POSTGRES_PASSWORD": pgTestPassword,
			"POSTGRES_DB":       pgTestDB,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithDeadline(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start PostgreSQL container")

	host, err := container.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)

	t.Cleanup(func() {
		if termErr := container.Terminate(ctx); termErr != nil {
			t.Logf("failed to terminate PostgreSQL container: %v", termErr)
		}
	})

	pc := &pgTestContainer{
		host: host,
		port: mappedPort.Int(),
	}

	cfg := &database.PostgresConfig{
		Host:           pc.host,
		Port:           pc.port,
		Database:       pgTestDB,
		User:           pgTestUser,
		PasswordEnvVar: "TEST_PG_PASSWORD",
		SSLMode:        "disable",
		MaxConns:       5,
		MinConns:       1,
	}

	db, err := database.New(ctx, cfg, pgTestPassword)
	require.NoError(t, err)

	err = database.Migrate(ctx, db.Pool)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return pc, db
}

func newTestPostgresStore(t *testing.T, db *database.DB) *storage.PostgresStore {
	t.Helper()
	return storage.NewPostgresStore(db, true)
}

func TestPostgresStore_Create(t *testing.T) {
	_, db := setupPgContainer(t)

	tests := []struct {
		name    string
		sub     *storage.Subscription
		wantErr error
	}{
		{
			name: "valid subscription",
			sub: &storage.Subscription{
				ID:                     "sub-create-1",
				TenantID:               "tenant-1",
				Callback:               "https://smo.example.com/notify",
				ConsumerSubscriptionID: "consumer-1",
				Filter: storage.SubscriptionFilter{
					ResourcePoolID: "pool-1",
					ResourceTypeID: "type-1",
				},
			},
			wantErr: nil,
		},
		{
			name: "valid subscription with http callback",
			sub: &storage.Subscription{
				ID:       "sub-create-2",
				Callback: "http://dev.example.com/notify",
			},
			wantErr: nil,
		},
		{
			name:    "empty ID",
			sub:     &storage.Subscription{ID: "", Callback: "https://example.com/notify"},
			wantErr: storage.ErrInvalidID,
		},
		{
			name:    "empty callback",
			sub:     &storage.Subscription{ID: "sub-create-3", Callback: ""},
			wantErr: storage.ErrInvalidCallback,
		},
		{
			name:    "invalid callback scheme",
			sub:     &storage.Subscription{ID: "sub-create-4", Callback: "ftp://example.com/notify"},
			wantErr: storage.ErrInvalidCallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestPostgresStore(t, db)
			ctx := context.Background()

			err := store.Create(ctx, tt.sub)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.False(t, tt.sub.CreatedAt.IsZero())
				assert.False(t, tt.sub.UpdatedAt.IsZero())
			}
		})
	}
}

func TestPostgresStore_CreateDuplicate(t *testing.T) {
	_, db := setupPgContainer(t)
	store := newTestPostgresStore(t, db)
	ctx := context.Background()

	sub := &storage.Subscription{
		ID:       "sub-dup-1",
		Callback: "https://smo.example.com/notify",
	}

	err := store.Create(ctx, sub)
	require.NoError(t, err)

	err = store.Create(ctx, sub)
	require.ErrorIs(t, err, storage.ErrSubscriptionExists)
}

func TestPostgresStore_Get(t *testing.T) {
	_, db := setupPgContainer(t)
	store := newTestPostgresStore(t, db)
	ctx := context.Background()

	tests := []struct {
		name    string
		setup   func()
		id      string
		wantErr error
	}{
		{
			name: "existing subscription",
			setup: func() {
				sub := &storage.Subscription{
					ID:       "sub-get-1",
					TenantID: "tenant-1",
					Callback: "https://smo.example.com/notify",
					Filter:   storage.SubscriptionFilter{ResourcePoolID: "pool-1"},
				}
				require.NoError(t, store.Create(ctx, sub))
			},
			id:      "sub-get-1",
			wantErr: nil,
		},
		{
			name:    "not found",
			setup:   func() {},
			id:      "sub-nonexistent",
			wantErr: storage.ErrSubscriptionNotFound,
		},
		{
			name:    "empty ID",
			setup:   func() {},
			id:      "",
			wantErr: storage.ErrInvalidID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			got, err := store.Get(ctx, tt.id)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.id, got.ID)
				assert.Equal(t, "tenant-1", got.TenantID)
				assert.Equal(t, "pool-1", got.Filter.ResourcePoolID)
			}
		})
	}
}

func TestPostgresStore_Update(t *testing.T) {
	_, db := setupPgContainer(t)
	store := newTestPostgresStore(t, db)
	ctx := context.Background()

	// Create initial subscription.
	original := &storage.Subscription{
		ID:       "sub-update-1",
		TenantID: "tenant-1",
		Callback: "https://smo.example.com/notify",
		Filter:   storage.SubscriptionFilter{ResourcePoolID: "pool-1"},
	}
	require.NoError(t, store.Create(ctx, original))

	tests := []struct {
		name    string
		sub     *storage.Subscription
		wantErr error
	}{
		{
			name: "valid update",
			sub: &storage.Subscription{
				ID:       "sub-update-1",
				TenantID: "tenant-1",
				Callback: "https://smo.example.com/v2/notify",
				Filter:   storage.SubscriptionFilter{ResourcePoolID: "pool-2"},
			},
			wantErr: nil,
		},
		{
			name: "not found",
			sub: &storage.Subscription{
				ID:       "sub-nonexistent",
				Callback: "https://example.com/notify",
			},
			wantErr: storage.ErrSubscriptionNotFound,
		},
		{
			name: "empty ID",
			sub: &storage.Subscription{
				ID:       "",
				Callback: "https://example.com/notify",
			},
			wantErr: storage.ErrInvalidID,
		},
		{
			name: "invalid callback",
			sub: &storage.Subscription{
				ID:       "sub-update-1",
				Callback: "",
			},
			wantErr: storage.ErrInvalidCallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Update(ctx, tt.sub)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)

				updated, getErr := store.Get(ctx, tt.sub.ID)
				require.NoError(t, getErr)
				assert.Equal(t, "https://smo.example.com/v2/notify", updated.Callback)
				assert.Equal(t, "pool-2", updated.Filter.ResourcePoolID)
			}
		})
	}
}

func TestPostgresStore_Delete(t *testing.T) {
	_, db := setupPgContainer(t)
	store := newTestPostgresStore(t, db)
	ctx := context.Background()

	tests := []struct {
		name    string
		setup   func()
		id      string
		wantErr error
	}{
		{
			name: "existing subscription",
			setup: func() {
				sub := &storage.Subscription{
					ID:       "sub-delete-1",
					Callback: "https://smo.example.com/notify",
				}
				require.NoError(t, store.Create(ctx, sub))
			},
			id:      "sub-delete-1",
			wantErr: nil,
		},
		{
			name:    "not found",
			setup:   func() {},
			id:      "sub-nonexistent",
			wantErr: storage.ErrSubscriptionNotFound,
		},
		{
			name:    "empty ID",
			setup:   func() {},
			id:      "",
			wantErr: storage.ErrInvalidID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			err := store.Delete(ctx, tt.id)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)

				_, getErr := store.Get(ctx, tt.id)
				assert.ErrorIs(t, getErr, storage.ErrSubscriptionNotFound)
			}
		})
	}
}

func TestPostgresStore_List(t *testing.T) {
	_, db := setupPgContainer(t)
	store := newTestPostgresStore(t, db)
	ctx := context.Background()

	// Initially empty.
	subs, err := store.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, subs)

	// Create subscriptions.
	for i := 0; i < 3; i++ {
		sub := &storage.Subscription{
			ID:       "sub-list-" + strconv.Itoa(i),
			TenantID: "tenant-1",
			Callback: "https://smo.example.com/notify",
		}
		require.NoError(t, store.Create(ctx, sub))
	}

	subs, err = store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, subs, 3)
}

func TestPostgresStore_ListByResourcePool(t *testing.T) {
	_, db := setupPgContainer(t)
	store := newTestPostgresStore(t, db)
	ctx := context.Background()

	// Create subscriptions with different pools.
	require.NoError(t, store.Create(ctx, &storage.Subscription{
		ID: "sub-pool-1", Callback: "https://smo.example.com/notify",
		Filter: storage.SubscriptionFilter{ResourcePoolID: "pool-alpha"},
	}))
	require.NoError(t, store.Create(ctx, &storage.Subscription{
		ID: "sub-pool-2", Callback: "https://smo.example.com/notify",
		Filter: storage.SubscriptionFilter{ResourcePoolID: "pool-alpha"},
	}))
	require.NoError(t, store.Create(ctx, &storage.Subscription{
		ID: "sub-pool-3", Callback: "https://smo.example.com/notify",
		Filter: storage.SubscriptionFilter{ResourcePoolID: "pool-beta"},
	}))

	tests := []struct {
		name      string
		poolID    string
		wantLen   int
		wantEmpty bool
	}{
		{name: "pool-alpha", poolID: "pool-alpha", wantLen: 2},
		{name: "pool-beta", poolID: "pool-beta", wantLen: 1},
		{name: "nonexistent pool", poolID: "pool-nonexistent", wantLen: 0},
		{name: "empty pool ID", poolID: "", wantLen: 0, wantEmpty: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subs, listErr := store.ListByResourcePool(ctx, tt.poolID)
			require.NoError(t, listErr)
			assert.Len(t, subs, tt.wantLen)
		})
	}
}

func TestPostgresStore_ListByResourceType(t *testing.T) {
	_, db := setupPgContainer(t)
	store := newTestPostgresStore(t, db)
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, &storage.Subscription{
		ID: "sub-type-1", Callback: "https://smo.example.com/notify",
		Filter: storage.SubscriptionFilter{ResourceTypeID: "type-compute"},
	}))
	require.NoError(t, store.Create(ctx, &storage.Subscription{
		ID: "sub-type-2", Callback: "https://smo.example.com/notify",
		Filter: storage.SubscriptionFilter{ResourceTypeID: "type-storage"},
	}))

	subs, err := store.ListByResourceType(ctx, "type-compute")
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "sub-type-1", subs[0].ID)

	subs, err = store.ListByResourceType(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestPostgresStore_ListByTenant(t *testing.T) {
	_, db := setupPgContainer(t)
	store := newTestPostgresStore(t, db)
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, &storage.Subscription{
		ID: "sub-tenant-1", TenantID: "tenant-a",
		Callback: "https://smo.example.com/notify",
	}))
	require.NoError(t, store.Create(ctx, &storage.Subscription{
		ID: "sub-tenant-2", TenantID: "tenant-a",
		Callback: "https://smo.example.com/notify",
	}))
	require.NoError(t, store.Create(ctx, &storage.Subscription{
		ID: "sub-tenant-3", TenantID: "tenant-b",
		Callback: "https://smo.example.com/notify",
	}))

	subs, err := store.ListByTenant(ctx, "tenant-a")
	require.NoError(t, err)
	assert.Len(t, subs, 2)

	subs, err = store.ListByTenant(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestPostgresStore_Ping(t *testing.T) {
	_, db := setupPgContainer(t)
	store := newTestPostgresStore(t, db)
	ctx := context.Background()

	err := store.Ping(ctx)
	require.NoError(t, err)
}

func TestPostgresStore_ConcurrentAccess(t *testing.T) {
	_, db := setupPgContainer(t)
	store := newTestPostgresStore(t, db)
	ctx := context.Background()

	const workers = 10
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sub := &storage.Subscription{
				ID:       "sub-concurrent-" + strconv.Itoa(idx),
				TenantID: "tenant-concurrent",
				Callback: "https://smo.example.com/notify",
			}
			if createErr := store.Create(ctx, sub); createErr != nil {
				errCh <- createErr
				return
			}
			if _, getErr := store.Get(ctx, sub.ID); getErr != nil {
				errCh <- getErr
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent operation failed: %v", err)
	}

	subs, err := store.ListByTenant(ctx, "tenant-concurrent")
	require.NoError(t, err)
	assert.Len(t, subs, workers)
}

func TestPostgresStore_Close(t *testing.T) {
	_, db := setupPgContainer(t)
	store := storage.NewPostgresStore(db, true)

	err := store.Close()
	require.NoError(t, err)
}
