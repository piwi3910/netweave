package storage

import (
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
)

// ExportNewTestPostgresStore creates a PostgresStore backed by a custom DBTX for testing.
// This allows unit testing of PostgresStore methods without a real database.
func ExportNewTestPostgresStore(db dbsqlc.DBTX, allowInsecureCallbacks bool) *PostgresStore {
	return &PostgresStore{
		queries:                dbsqlc.New(db),
		allowInsecureCallbacks: allowInsecureCallbacks,
	}
}

// ExportIsPgUniqueViolation exposes isPgUniqueViolation for testing.
func ExportIsPgUniqueViolation(err error) bool {
	return isPgUniqueViolation(err)
}
