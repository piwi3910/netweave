package storage_test

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/piwi3910/netweave/internal/storage"
)

// mockRow implements pgx.Row for testing.
type mockRow struct {
	scanFunc func(dest ...interface{}) error
}

func (r *mockRow) Scan(dest ...interface{}) error {
	return r.scanFunc(dest...)
}

// mockRows implements pgx.Rows for testing.
type mockRows struct {
	data    [][]interface{}
	current int
	err     error
	closed  bool
}

func (r *mockRows) Close()                                       { r.closed = true }
func (r *mockRows) Err() error                                   { return r.err }
func (r *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("") }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockRows) RawValues() [][]byte                          { return nil }
func (r *mockRows) Conn() *pgx.Conn                              { return nil }

func (r *mockRows) Next() bool {
	if r.current >= len(r.data) {
		return false
	}
	r.current++
	return true
}

func (r *mockRows) Scan(dest ...interface{}) error {
	row := r.data[r.current-1]
	if len(dest) != len(row) {
		return fmt.Errorf("scan: expected %d columns, got %d", len(row), len(dest))
	}
	for i, val := range row {
		switch d := dest[i].(type) {
		case *string:
			if v, ok := val.(string); ok {
				*d = v
			}
		case *bool:
			if v, ok := val.(bool); ok {
				*d = v
			}
		case *time.Time:
			if v, ok := val.(time.Time); ok {
				*d = v
			}
		default:
			return fmt.Errorf("unsupported scan type at index %d: %T", i, dest[i])
		}
	}
	return nil
}

func (r *mockRows) Values() ([]interface{}, error) {
	if r.current == 0 || r.current > len(r.data) {
		return nil, fmt.Errorf("no current row")
	}
	return r.data[r.current-1], nil
}

// mockDBTX implements dbsqlc.DBTX for testing.
type mockDBTX struct {
	execFunc     func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	queryFunc    func(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	queryRowFunc func(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

func (m *mockDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag(""), nil
}

func (m *mockDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql, args...)
	}
	return &mockRows{}, nil
}

func (m *mockDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return &mockRow{scanFunc: func(dest ...interface{}) error {
		return pgx.ErrNoRows
	}}
}

// subscriptionRow returns a slice of interface{} matching the Subscription scan order:
// id, tenant_id, callback, consumer_subscription_id,
// filter_resource_pool_id, filter_resource_type_id, filter_resource_id,
// created_at, updated_at.
func subscriptionRow(id, tenantID, callback, consumerSubID, poolID, typeID, resID string, createdAt, updatedAt time.Time) []interface{} {
	return []interface{}{
		id, tenantID, callback, consumerSubID,
		poolID, typeID, resID,
		createdAt, updatedAt,
	}
}

// newMockStore creates a PostgresStore backed by a mockDBTX for testing.
func newMockStore(db *mockDBTX, allowInsecure bool) *storage.PostgresStore {
	return storage.ExportNewTestPostgresStore(db, allowInsecure)
}
