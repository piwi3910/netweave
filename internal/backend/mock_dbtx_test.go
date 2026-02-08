package backend_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
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
		case *int64:
			if v, ok := val.(int64); ok {
				*d = v
			}
		case *int32:
			if v, ok := val.(int32); ok {
				*d = v
			}
		case *bool:
			if v, ok := val.(bool); ok {
				*d = v
			}
		case *[]byte:
			if v, ok := val.([]byte); ok {
				*d = v
			}
		case *time.Time:
			if v, ok := val.(time.Time); ok {
				*d = v
			}
		case *pgtype.Timestamptz:
			if v, ok := val.(pgtype.Timestamptz); ok {
				*d = v
			}
		case *json.RawMessage:
			if v, ok := val.(json.RawMessage); ok {
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

// backendInstanceRow returns a slice of interface{} matching the BackendInstance scan order.
func backendInstanceRow(inst *dbsqlc.BackendInstance) []interface{} {
	return []interface{}{
		inst.ID,
		inst.Name,
		inst.Category,
		inst.AdapterType,
		inst.Description,
		inst.Config,
		inst.Credentials,
		inst.Status,
		inst.StatusMessage,
		inst.LastHealthCheck,
		inst.CreatedAt,
		inst.UpdatedAt,
	}
}

// backendAccessRow returns a slice of interface{} matching the BackendAccess scan order.
func backendAccessRow(acc *dbsqlc.BackendAccess) []interface{} {
	return []interface{}{
		acc.ID,
		acc.TenantID,
		acc.BackendID,
		acc.Permissions,
		acc.Constraints,
		acc.GrantedAt,
		acc.GrantedBy,
	}
}

// newMockStore creates a PostgresStore backed by a mockDBTX.
func newMockStore(db *mockDBTX) *backend.PostgresStore {
	return backend.ExportNewTestPostgresStore(db)
}
