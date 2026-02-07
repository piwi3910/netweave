package database

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all pending SQL migration files against the database.
// Migrations are tracked in a schema_migrations table.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if err := createMigrationsTable(ctx, pool); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	files, err := listMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to list migration files: %w", err)
	}

	for _, filename := range files {
		applied, err := isMigrationApplied(ctx, pool, filename)
		if err != nil {
			return fmt.Errorf("failed to check migration %s: %w", filename, err)
		}
		if applied {
			continue
		}

		if err := runMigration(ctx, pool, filename); err != nil {
			return fmt.Errorf("failed to run migration %s: %w", filename, err)
		}
	}

	return nil
}

func createMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename    TEXT PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to exec create migrations table: %w", err)
	}
	return nil
}

func listMigrationFiles() ([]string, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func isMigrationApplied(ctx context.Context, pool *pgxpool.Pool, filename string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)",
		filename,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if migration applied: %w", err)
	}
	return exists, nil
}

func runMigration(ctx context.Context, pool *pgxpool.Pool, filename string) error {
	content, err := migrationsFS.ReadFile("migrations/" + filename)
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && rbErr.Error() != "tx is closed" {
			// Log rollback error but don't override primary error.
			_ = rbErr
		}
	}()

	if _, err := tx.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (filename) VALUES ($1)",
		filename,
	); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}
	return nil
}
