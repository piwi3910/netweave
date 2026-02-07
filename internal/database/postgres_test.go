package database

import (
	"context"
	"testing"
	"time"

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

// postgresContainer represents a running PostgreSQL test container.
type postgresContainer struct {
	container testcontainers.Container
	host      string
	port      int
}

// setupPostgresContainer starts a PostgreSQL container for integration testing.
func setupPostgresContainer(t *testing.T) *postgresContainer {
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
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate PostgreSQL container: %v", err)
		}
	})

	return &postgresContainer{
		container: container,
		host:      host,
		port:      mappedPort.Int(),
	}
}

// testConfig returns a PostgresConfig pointing at the test container.
func (pc *postgresContainer) testConfig() *PostgresConfig {
	return &PostgresConfig{
		Host:           pc.host,
		Port:           pc.port,
		Database:       testDBName,
		User:           testDBUser,
		PasswordEnvVar: "TEST_PG_PASSWORD",
		SSLMode:        "disable",
		MaxConns:       5,
		MinConns:       1,
	}
}

func TestNew_Success(t *testing.T) {
	pc := setupPostgresContainer(t)
	ctx := context.Background()

	db, err := New(ctx, pc.testConfig(), testDBPassword)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	err = db.Ping(ctx)
	assert.NoError(t, err)
}

func TestNew_InvalidConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		cfg  *PostgresConfig
	}{
		{
			name: "missing host",
			cfg:  &PostgresConfig{Database: "db", User: "user", PasswordEnvVar: "PW"},
		},
		{
			name: "missing database",
			cfg:  &PostgresConfig{Host: "localhost", User: "user", PasswordEnvVar: "PW"},
		},
		{
			name: "missing user",
			cfg:  &PostgresConfig{Host: "localhost", Database: "db", PasswordEnvVar: "PW"},
		},
		{
			name: "missing password env var",
			cfg:  &PostgresConfig{Host: "localhost", Database: "db", User: "user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := New(ctx, tt.cfg, "password")
			assert.Error(t, err)
			assert.Nil(t, db)
		})
	}
}

func TestNew_ConnectionFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cfg := &PostgresConfig{
		Host:           "localhost",
		Port:           59999,
		Database:       "nonexistent",
		User:           "nobody",
		PasswordEnvVar: "PW",
		SSLMode:        "disable",
	}

	db, err := New(ctx, cfg, "wrongpass")
	assert.Error(t, err)
	assert.Nil(t, db)
}

func TestMigrate_Success(t *testing.T) {
	pc := setupPostgresContainer(t)
	ctx := context.Background()

	db, err := New(ctx, pc.testConfig(), testDBPassword)
	require.NoError(t, err)
	defer db.Close()

	err = Migrate(ctx, db.Pool)
	require.NoError(t, err)

	// Verify tables were created by querying pg_tables.
	var count int
	err = db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name IN (
			'subscriptions', 'tenants', 'users', 'roles',
			'audit_events', 'notification_deliveries',
			'backend_instances', 'backend_links', 'backend_access',
			'schema_migrations'
		)
	`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 10, count, "expected 10 tables (9 data + 1 migrations)")
}

func TestMigrate_Idempotent(t *testing.T) {
	pc := setupPostgresContainer(t)
	ctx := context.Background()

	db, err := New(ctx, pc.testConfig(), testDBPassword)
	require.NoError(t, err)
	defer db.Close()

	// Run migrations twice — should be idempotent.
	err = Migrate(ctx, db.Pool)
	require.NoError(t, err)

	err = Migrate(ctx, db.Pool)
	require.NoError(t, err)
}

func TestConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		cfg      PostgresConfig
		password string
		want     string
	}{
		{
			name: "all fields",
			cfg: PostgresConfig{
				Host:     "db.example.com",
				Port:     5433,
				Database: "mydb",
				User:     "myuser",
				SSLMode:  "require",
			},
			password: "secret",
			want:     "postgres://myuser:secret@db.example.com:5433/mydb?sslmode=require",
		},
		{
			name: "defaults",
			cfg: PostgresConfig{
				Host:     "localhost",
				Database: "netweave",
				User:     "admin",
			},
			password: "pw",
			want:     "postgres://admin:pw@localhost:5432/netweave?sslmode=prefer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.ConnectionString(tt.password)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidate(t *testing.T) {
	valid := &PostgresConfig{
		Host:           "localhost",
		Database:       "netweave",
		User:           "admin",
		PasswordEnvVar: "PG_PASSWORD",
	}
	assert.NoError(t, valid.Validate())
}
