package auth

import (
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
)

// ExportNewTestPostgresStore creates a PostgresStore backed by a custom DBTX for testing.
// This allows unit testing of PostgresStore methods without a real database.
func ExportNewTestPostgresStore(db dbsqlc.DBTX) *PostgresStore {
	return &PostgresStore{
		queries: dbsqlc.New(db),
	}
}

// ExportNewPgError creates a pgconn.PgError for testing.
func ExportNewPgError(code string) error {
	return &pgconn.PgError{Code: code}
}
