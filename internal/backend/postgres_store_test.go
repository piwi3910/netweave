package backend_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	testPostgresImage = "postgres:16-alpine"
	testDBName        = "netweave_test"
	testDBUser        = "testuser"
	testDBPassword    = "testpass"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        testPostgresImage,
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     testDBUser,
			"POSTGRES_PASSWORD": testDBPassword,
			"POSTGRES_DB":       testDBName,
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

	cfg := &database.PostgresConfig{
		Host:           host,
		Port:           mappedPort.Int(),
		Database:       testDBName,
		User:           testDBUser,
		PasswordEnvVar: "TEST_PG_PASSWORD",
		SSLMode:        "disable",
		MaxConns:       5,
		MinConns:       1,
	}

	db, err := database.New(ctx, cfg, testDBPassword)
	require.NoError(t, err)

	err = database.Migrate(ctx, db.Pool)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db.Pool
}

func newTestBackend(id, name, category, adapterType string) *backend.Instance {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &backend.Instance{
		ID:          id,
		Name:        name,
		Category:    category,
		AdapterType: adapterType,
		Description: "test backend",
		Config:      []byte(`{"key":"value"}`),
		Credentials: []byte(`{"secret":"encrypted"}`),
		Status:      "unknown",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestPostgresStore_BackendCRUD(t *testing.T) {
	pool := setupTestDB(t)
	store := backend.NewPostgresStore(pool)
	ctx := context.Background()

	t.Run("create and get backend", func(t *testing.T) {
		b := newTestBackend("be-1", "test-k8s", "ims", "kubernetes")
		err := store.CreateBackend(ctx, b)
		require.NoError(t, err)

		got, err := store.GetBackend(ctx, "be-1")
		require.NoError(t, err)
		assert.Equal(t, b.Name, got.Name)
		assert.Equal(t, b.Category, got.Category)
		assert.Equal(t, b.AdapterType, got.AdapterType)
		assert.Equal(t, b.Config, got.Config)
		assert.Equal(t, b.Credentials, got.Credentials)
	})

	t.Run("create duplicate name returns error", func(t *testing.T) {
		b := newTestBackend("be-dup-1", "duplicate-name", "ims", "kubernetes")
		err := store.CreateBackend(ctx, b)
		require.NoError(t, err)

		b2 := newTestBackend("be-dup-2", "duplicate-name", "dms", "helm")
		err = store.CreateBackend(ctx, b2)
		require.ErrorIs(t, err, backend.ErrBackendExists)
	})

	t.Run("get non-existent backend returns not found", func(t *testing.T) {
		_, err := store.GetBackend(ctx, "non-existent")
		require.ErrorIs(t, err, backend.ErrBackendNotFound)
	})

	t.Run("update backend", func(t *testing.T) {
		b := newTestBackend("be-upd", "update-me", "ims", "aws")
		err := store.CreateBackend(ctx, b)
		require.NoError(t, err)

		b.Name = "updated-name"
		b.Description = "updated description"
		b.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		err = store.UpdateBackend(ctx, b)
		require.NoError(t, err)

		got, err := store.GetBackend(ctx, "be-upd")
		require.NoError(t, err)
		assert.Equal(t, "updated-name", got.Name)
		assert.Equal(t, "updated description", got.Description)
	})

	t.Run("update non-existent backend returns not found", func(t *testing.T) {
		b := newTestBackend("non-existent", "name", "ims", "aws")
		err := store.UpdateBackend(ctx, b)
		require.ErrorIs(t, err, backend.ErrBackendNotFound)
	})

	t.Run("delete backend", func(t *testing.T) {
		b := newTestBackend("be-del", "delete-me", "dms", "helm")
		err := store.CreateBackend(ctx, b)
		require.NoError(t, err)

		err = store.DeleteBackend(ctx, "be-del")
		require.NoError(t, err)

		_, err = store.GetBackend(ctx, "be-del")
		require.ErrorIs(t, err, backend.ErrBackendNotFound)
	})

	t.Run("delete non-existent backend returns not found", func(t *testing.T) {
		err := store.DeleteBackend(ctx, "non-existent")
		require.ErrorIs(t, err, backend.ErrBackendNotFound)
	})
}

func TestPostgresStore_ListBackends(t *testing.T) {
	pool := setupTestDB(t)
	store := backend.NewPostgresStore(pool)
	ctx := context.Background()

	backends := []*backend.Instance{
		newTestBackend("lb-1", "k8s-prod", "ims", "kubernetes"),
		newTestBackend("lb-2", "aws-staging", "ims", "aws"),
		newTestBackend("lb-3", "helm-repo", "dms", "helm"),
	}
	for _, b := range backends {
		err := store.CreateBackend(ctx, b)
		require.NoError(t, err)
	}

	t.Run("list all backends", func(t *testing.T) {
		result, err := store.ListBackends(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result), 3)
	})

	t.Run("list IMS backends", func(t *testing.T) {
		result, err := store.ListBackendsByCategory(ctx, "ims")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result), 2)
		for _, b := range result {
			assert.Equal(t, "ims", b.Category)
		}
	})

	t.Run("list DMS backends", func(t *testing.T) {
		result, err := store.ListBackendsByCategory(ctx, "dms")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result), 1)
		for _, b := range result {
			assert.Equal(t, "dms", b.Category)
		}
	})
}

func TestPostgresStore_BackendStatus(t *testing.T) {
	pool := setupTestDB(t)
	store := backend.NewPostgresStore(pool)
	ctx := context.Background()

	b := newTestBackend("bs-1", "status-test", "ims", "kubernetes")
	err := store.CreateBackend(ctx, b)
	require.NoError(t, err)

	err = store.UpdateBackendStatus(ctx, "bs-1", "connected", "healthy")
	require.NoError(t, err)

	got, err := store.GetBackend(ctx, "bs-1")
	require.NoError(t, err)
	assert.Equal(t, "connected", got.Status)
	assert.Equal(t, "healthy", got.StatusMessage)
	assert.False(t, got.LastHealthCheck.IsZero())
}

func TestPostgresStore_BackendLinks(t *testing.T) {
	pool := setupTestDB(t)
	store := backend.NewPostgresStore(pool)
	ctx := context.Background()

	ims := newTestBackend("link-ims", "my-k8s", "ims", "kubernetes")
	dms1 := newTestBackend("link-dms1", "my-helm", "dms", "helm")
	dms2 := newTestBackend("link-dms2", "my-argocd", "dms", "argocd")

	for _, b := range []*backend.Instance{ims, dms1, dms2} {
		err := store.CreateBackend(ctx, b)
		require.NoError(t, err)
	}

	t.Run("create and list links", func(t *testing.T) {
		err := store.CreateBackendLink(ctx, "link-ims", "link-dms1")
		require.NoError(t, err)

		err = store.CreateBackendLink(ctx, "link-ims", "link-dms2")
		require.NoError(t, err)

		dmsLinks, err := store.ListDMSLinksByIMS(ctx, "link-ims")
		require.NoError(t, err)
		assert.Len(t, dmsLinks, 2)

		imsLinks, err := store.ListIMSLinksByDMS(ctx, "link-dms1")
		require.NoError(t, err)
		assert.Len(t, imsLinks, 1)
		assert.Equal(t, "link-ims", imsLinks[0].ID)
	})

	t.Run("create duplicate link returns error", func(t *testing.T) {
		err := store.CreateBackendLink(ctx, "link-ims", "link-dms1")
		require.ErrorIs(t, err, backend.ErrBackendLinkExists)
	})

	t.Run("delete link", func(t *testing.T) {
		err := store.DeleteBackendLink(ctx, "link-ims", "link-dms2")
		require.NoError(t, err)

		dmsLinks, err := store.ListDMSLinksByIMS(ctx, "link-ims")
		require.NoError(t, err)
		assert.Len(t, dmsLinks, 1)
	})

	t.Run("delete non-existent link returns not found", func(t *testing.T) {
		err := store.DeleteBackendLink(ctx, "link-ims", "non-existent")
		require.ErrorIs(t, err, backend.ErrBackendLinkNotFound)
	})
}

func TestPostgresStore_Access(t *testing.T) {
	pool := setupTestDB(t)
	store := backend.NewPostgresStore(pool)
	ctx := context.Background()

	b := newTestBackend("acc-be", "access-backend", "ims", "kubernetes")
	err := store.CreateBackend(ctx, b)
	require.NoError(t, err)

	t.Run("create and get access", func(t *testing.T) {
		access := &backend.Access{
			ID:          "acc-1",
			TenantID:    "tenant-1",
			BackendID:   "acc-be",
			Permissions: []string{"read", "list"},
			Constraints: map[string]interface{}{"namespace": "default"},
			GrantedAt:   time.Now().UTC().Truncate(time.Microsecond),
			GrantedBy:   "admin-user",
		}
		err := store.CreateBackendAccess(ctx, access)
		require.NoError(t, err)

		got, err := store.GetBackendAccess(ctx, "acc-1")
		require.NoError(t, err)
		assert.Equal(t, "tenant-1", got.TenantID)
		assert.Equal(t, "acc-be", got.BackendID)
		assert.Equal(t, []string{"read", "list"}, got.Permissions)
		assert.Equal(t, "default", got.Constraints["namespace"])
	})

	t.Run("get non-existent access returns not found", func(t *testing.T) {
		_, err := store.GetBackendAccess(ctx, "non-existent")
		require.ErrorIs(t, err, backend.ErrAccessNotFound)
	})

	t.Run("update access", func(t *testing.T) {
		access := &backend.Access{
			ID:          "acc-1",
			Permissions: []string{"read", "list", "write"},
			Constraints: map[string]interface{}{"namespace": "production"},
		}
		err := store.UpdateBackendAccess(ctx, access)
		require.NoError(t, err)

		got, err := store.GetBackendAccess(ctx, "acc-1")
		require.NoError(t, err)
		assert.Equal(t, []string{"read", "list", "write"}, got.Permissions)
		assert.Equal(t, "production", got.Constraints["namespace"])
	})

	t.Run("list access by tenant", func(t *testing.T) {
		result, err := store.ListBackendAccessByTenant(ctx, "tenant-1")
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("list access by backend", func(t *testing.T) {
		result, err := store.ListBackendAccessByBackend(ctx, "acc-be")
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("delete access", func(t *testing.T) {
		err := store.DeleteBackendAccess(ctx, "acc-1")
		require.NoError(t, err)

		_, err = store.GetBackendAccess(ctx, "acc-1")
		require.ErrorIs(t, err, backend.ErrAccessNotFound)
	})

	t.Run("delete non-existent access returns not found", func(t *testing.T) {
		err := store.DeleteBackendAccess(ctx, "non-existent")
		require.ErrorIs(t, err, backend.ErrAccessNotFound)
	})
}

func TestPostgresStore_PingAndClose(t *testing.T) {
	pool := setupTestDB(t)
	store := backend.NewPostgresStore(pool)
	ctx := context.Background()

	err := store.Ping(ctx)
	require.NoError(t, err)

	err = store.Close()
	require.NoError(t, err)
}
