package backend

import (
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
)

// ExportNewTestPostgresStore creates a PostgresStore backed by a custom DBTX for testing.
// This allows unit testing of PostgresStore methods without a real database.
func ExportNewTestPostgresStore(db dbsqlc.DBTX) *PostgresStore {
	return &PostgresStore{
		queries: dbsqlc.New(db),
	}
}

// ExportIsUniqueViolation exposes isUniqueViolation for testing.
func ExportIsUniqueViolation(err error) bool {
	return isUniqueViolation(err)
}

// ExportTimestamptzFromTime exposes timestamptzFromTime for testing.
func ExportTimestamptzFromTime(t time.Time) pgtype.Timestamptz {
	return timestamptzFromTime(t)
}

// ExportTimeFromTimestamptz exposes timeFromTimestamptz for testing.
func ExportTimeFromTimestamptz(ts pgtype.Timestamptz) time.Time {
	return timeFromTimestamptz(ts)
}

// ExportDbBackendToModel exposes dbBackendToModel for testing.
func ExportDbBackendToModel(row *dbsqlc.BackendInstance) *Instance {
	return dbBackendToModel(row)
}

// ExportDbBackendsToModels exposes dbBackendsToModels for testing.
func ExportDbBackendsToModels(rows []dbsqlc.BackendInstance) []*Instance {
	return dbBackendsToModels(rows)
}

// ExportDbAccessToModel exposes dbAccessToModel for testing.
func ExportDbAccessToModel(row *dbsqlc.BackendAccess) (*Access, error) {
	return dbAccessToModel(row)
}

// ExportDbAccessListToModels exposes dbAccessListToModels for testing.
func ExportDbAccessListToModels(rows []dbsqlc.BackendAccess) ([]*Access, error) {
	return dbAccessListToModels(rows)
}

// ExportNewPgError creates a pgconn.PgError for testing.
func ExportNewPgError(code string) error {
	return &pgconn.PgError{Code: code}
}
