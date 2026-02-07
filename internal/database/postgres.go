package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with convenience methods.
type DB struct {
	Pool *pgxpool.Pool
}

// New creates a new DB with a pgxpool.Pool from the given config and password.
func New(ctx context.Context, cfg *PostgresConfig, password string) (*DB, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid postgres config: %w", err)
	}

	connString := cfg.ConnectionString(password)
	poolCfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres connection string: %w", err)
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Close closes the connection pool.
func (db *DB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}

// Ping checks if the database is reachable.
func (db *DB) Ping(ctx context.Context) error {
	if err := db.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping failed: %w", err)
	}
	return nil
}
