package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/piwi3910/netweave/internal/auth"
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

// tenantRow returns a slice of interface{} matching the Tenant scan order:
// id, name, description, status, quota, usage, contact_email, metadata, created_at, updated_at.
func tenantRow(t *dbsqlc.Tenant) []interface{} {
	return []interface{}{
		t.ID,
		t.Name,
		t.Description,
		t.Status,
		t.Quota,
		t.Usage,
		t.ContactEmail,
		t.Metadata,
		t.CreatedAt,
		t.UpdatedAt,
	}
}

// userRow returns a slice of interface{} matching the User scan order:
// id, tenant_id, subject, common_name, email, oauth_subject, oauth_provider,
// role_id, is_active, last_login_at, created_at, updated_at.
func userRow(u *dbsqlc.User) []interface{} {
	return []interface{}{
		u.ID,
		u.TenantID,
		u.Subject,
		u.CommonName,
		u.Email,
		u.OauthSubject,
		u.OauthProvider,
		u.RoleID,
		u.IsActive,
		u.LastLoginAt,
		u.CreatedAt,
		u.UpdatedAt,
	}
}

// roleRow returns a slice of interface{} matching the Role scan order:
// id, name, type, description, permissions, tenant_id, created_at, updated_at.
func roleRow(r *dbsqlc.Role) []interface{} {
	return []interface{}{
		r.ID,
		r.Name,
		r.Type,
		r.Description,
		r.Permissions,
		r.TenantID,
		r.CreatedAt,
		r.UpdatedAt,
	}
}

// auditEventRow returns a slice of interface{} matching the AuditEvent scan order:
// id, type, tenant_id, user_id, subject, resource_type, resource_id,
// action, details, client_ip, user_agent, timestamp.
func auditEventRow(e *dbsqlc.AuditEvent) []interface{} {
	return []interface{}{
		e.ID,
		e.Type,
		e.TenantID,
		e.UserID,
		e.Subject,
		e.ResourceType,
		e.ResourceID,
		e.Action,
		e.Details,
		e.ClientIp,
		e.UserAgent,
		e.Timestamp,
	}
}

// newMockStore creates a PostgresStore backed by a mockDBTX.
func newMockPgStore(db *mockDBTX) *auth.PostgresStore {
	return auth.ExportNewTestPostgresStore(db)
}

// scanRowInto copies row data from a typed slice into scan destination pointers.
func scanRowInto(dest []interface{}, row []interface{}) error {
	if len(dest) != len(row) {
		return fmt.Errorf("scan: expected %d columns, got %d", len(row), len(dest))
	}
	for i, val := range row {
		switch d := dest[i].(type) {
		case *string:
			*d = val.(string)
		case *bool:
			*d = val.(bool)
		case *int32:
			*d = val.(int32)
		case *int64:
			*d = val.(int64)
		case *[]byte:
			*d = val.([]byte)
		case *time.Time:
			*d = val.(time.Time)
		case *pgtype.Timestamptz:
			*d = val.(pgtype.Timestamptz)
		case *json.RawMessage:
			*d = val.(json.RawMessage)
		default:
			return fmt.Errorf("unsupported scan type at index %d: %T", i, dest[i])
		}
	}
	return nil
}

// boolExistsRow returns a mockRow that scans a boolean value (for *Exists queries).
func boolExistsRow(exists bool) pgx.Row {
	return &mockRow{scanFunc: func(dest ...interface{}) error {
		if d, ok := dest[0].(*bool); ok {
			*d = exists
		}
		return nil
	}}
}

// errorRow returns a mockRow that always returns the given error.
func errorRow(err error) pgx.Row {
	return &mockRow{scanFunc: func(_ ...interface{}) error {
		return err
	}}
}
