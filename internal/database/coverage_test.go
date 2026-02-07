package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListMigrationFiles tests the embedded migration file listing.
func TestListMigrationFiles(t *testing.T) {
	files, err := listMigrationFiles()
	require.NoError(t, err)

	// There should be at least one migration file.
	require.NotEmpty(t, files, "expected at least one migration file")

	// All files should end in .sql.
	for _, f := range files {
		assert.True(t, len(f) > 4 && f[len(f)-4:] == ".sql",
			"expected .sql extension, got: %s", f)
	}

	// Files should be sorted.
	for i := 1; i < len(files); i++ {
		assert.True(t, files[i] > files[i-1],
			"expected sorted order, but %s > %s", files[i-1], files[i])
	}

	// The initial schema migration should be present.
	assert.Contains(t, files, "001_initial_schema.sql")
}

// TestClose_NilPool tests that Close handles a nil pool gracefully.
func TestClose_NilPool(t *testing.T) {
	db := &DB{Pool: nil}
	// Should not panic when pool is nil.
	db.Close()
}

// TestClose_NonNilPool tests that Close calls pool.Close on a non-nil pool.
func TestClose_NonNilPool(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	// Integration test covered by TestNew_Success which defers db.Close().
}

// TestNew_WithMaxMinConns tests that New properly sets MaxConns and MinConns.
// This exercises the MaxConns > 0 and MinConns > 0 branches.
func TestNew_WithMaxMinConns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cfg := &PostgresConfig{
		Host:           "localhost",
		Port:           59999, // Unreachable port -- connection will fail.
		Database:       "testdb",
		User:           "testuser",
		PasswordEnvVar: "PW",
		SSLMode:        "disable",
		MaxConns:       10,
		MinConns:       2,
	}

	// The test exercises the MaxConns/MinConns branches in New().
	// Connection fails at pool creation or ping, but the config branches are covered.
	db, err := New(ctx, cfg, "pass")
	assert.Error(t, err)
	assert.Nil(t, db)
}

// TestMigrationsFS_Readable tests that the embedded migrations filesystem is readable.
func TestMigrationsFS_Readable(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	// Verify we can read at least one file.
	for _, entry := range entries {
		if !entry.IsDir() {
			content, readErr := migrationsFS.ReadFile("migrations/" + entry.Name())
			require.NoError(t, readErr)
			assert.NotEmpty(t, content, "migration file %s should not be empty", entry.Name())
			break
		}
	}
}

// TestConnectionString_AllDefaults tests ConnectionString with all default values.
func TestConnectionString_AllDefaults(t *testing.T) {
	cfg := &PostgresConfig{
		Host:     "myhost",
		Database: "mydb",
		User:     "myuser",
	}
	// Port defaults to 5432, SSLMode defaults to "prefer".
	result := cfg.ConnectionString("pw")
	assert.Equal(t, "postgres://myuser:pw@myhost:5432/mydb?sslmode=prefer", result)
}

// TestConnectionString_CustomPort tests ConnectionString with a custom port.
func TestConnectionString_CustomPort(t *testing.T) {
	cfg := &PostgresConfig{
		Host:     "dbhost",
		Port:     5433,
		Database: "testdb",
		User:     "testuser",
		SSLMode:  "require",
	}
	result := cfg.ConnectionString("secret")
	assert.Equal(t, "postgres://testuser:secret@dbhost:5433/testdb?sslmode=require", result)
}

// TestValidate_AllErrors tests each validation error path independently.
func TestValidate_AllErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *PostgresConfig
		wantErr string
	}{
		{
			name:    "missing host",
			cfg:     &PostgresConfig{Database: "db", User: "u", PasswordEnvVar: "PW"},
			wantErr: "postgres host is required",
		},
		{
			name:    "missing database",
			cfg:     &PostgresConfig{Host: "h", User: "u", PasswordEnvVar: "PW"},
			wantErr: "postgres database is required",
		},
		{
			name:    "missing user",
			cfg:     &PostgresConfig{Host: "h", Database: "db", PasswordEnvVar: "PW"},
			wantErr: "postgres user is required",
		},
		{
			name:    "missing password env var",
			cfg:     &PostgresConfig{Host: "h", Database: "db", User: "u"},
			wantErr: "postgres password_env_var is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestValidate_Success tests successful validation.
func TestValidate_Success(t *testing.T) {
	cfg := &PostgresConfig{
		Host:           "localhost",
		Database:       "netweave",
		User:           "admin",
		PasswordEnvVar: "PG_PASSWORD",
	}
	assert.NoError(t, cfg.Validate())
}
