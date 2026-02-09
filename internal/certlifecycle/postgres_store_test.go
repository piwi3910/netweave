package certlifecycle_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/certlifecycle"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
)

// --- Mock infrastructure ---

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
	return scanRowInto(dest, row)
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

// newMockPgStore creates a PostgresStore backed by a mockDBTX.
func newMockPgStore(db *mockDBTX) *certlifecycle.PostgresStore {
	return certlifecycle.ExportNewTestPostgresStore(db)
}

// --- Helper functions ---

// certMetaRow returns a slice matching the CertificateMetadatum scan order:
// serial_number, user_id, tenant_id, common_name, role_name, status,
// issued_at, expires_at, revoked_at, renewed_at, renewed_from, renewed_to,
// renewal_count, last_error, retry_count, next_retry_at, created_at, updated_at.
func certMetaRow(c *dbsqlc.CertificateMetadatum) []interface{} {
	return []interface{}{
		c.SerialNumber,
		c.UserID,
		c.TenantID,
		c.CommonName,
		c.RoleName,
		c.Status,
		c.IssuedAt,
		c.ExpiresAt,
		c.RevokedAt,
		c.RenewedAt,
		c.RenewedFrom,
		c.RenewedTo,
		c.RenewalCount,
		c.LastError,
		c.RetryCount,
		c.NextRetryAt,
		c.CreatedAt,
		c.UpdatedAt,
	}
}

// scanRowInto copies row data from a typed slice into scan destination pointers.
func scanRowInto(dest []interface{}, row []interface{}) error {
	if len(dest) != len(row) {
		return fmt.Errorf("scan: expected %d columns, got %d", len(row), len(dest))
	}
	for i, val := range row {
		switch d := dest[i].(type) {
		case *string:
			if v, ok := val.(string); ok {
				*d = v
			}
		case *int32:
			if v, ok := val.(int32); ok {
				*d = v
			}
		case *int64:
			if v, ok := val.(int64); ok {
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
		case *pgtype.Timestamptz:
			if v, ok := val.(pgtype.Timestamptz); ok {
				*d = v
			}
		default:
			return fmt.Errorf("unsupported scan type at index %d: %T", i, dest[i])
		}
	}
	return nil
}

// errorRow returns a mockRow that always returns the given error.
func errorRow(err error) pgx.Row {
	return &mockRow{scanFunc: func(_ ...interface{}) error {
		return err
	}}
}

// --- Test data ---

var (
	testNow   = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	testLater = time.Date(2027, 1, 15, 10, 0, 0, 0, time.UTC)
)

func testCertMeta() *dbsqlc.CertificateMetadatum {
	return &dbsqlc.CertificateMetadatum{
		SerialNumber: "aa:bb:cc:dd",
		UserID:       "user-1",
		TenantID:     "tenant-1",
		CommonName:   "test.example.com",
		RoleName:     "web-server",
		Status:       "active",
		IssuedAt:     testNow,
		ExpiresAt:    testLater,
		RevokedAt:    pgtype.Timestamptz{Valid: false},
		RenewedAt:    pgtype.Timestamptz{Valid: false},
		RenewedFrom:  "",
		RenewedTo:    "",
		RenewalCount: 0,
		LastError:    "",
		RetryCount:   0,
		NextRetryAt:  pgtype.Timestamptz{Valid: false},
		CreatedAt:    testNow,
		UpdatedAt:    testNow,
	}
}

// --- Tests ---

func TestPostgresStore_Create(t *testing.T) {
	tests := []struct {
		name    string
		meta    *certlifecycle.CertificateMetadata
		setup   func(*mockDBTX)
		wantErr error
	}{
		{
			name: "valid certificate",
			meta: &certlifecycle.CertificateMetadata{
				SerialNumber: "aa:bb:cc:dd",
				UserID:       "user-1",
				TenantID:     "tenant-1",
				CommonName:   "test.example.com",
				RoleName:     "web-server",
				Status:       certlifecycle.StatusActive,
				IssuedAt:     testNow,
				ExpiresAt:    testLater,
			},
			setup:   func(_ *mockDBTX) {},
			wantErr: nil,
		},
		{
			name: "empty serial number",
			meta: &certlifecycle.CertificateMetadata{
				SerialNumber: "",
			},
			setup:   func(_ *mockDBTX) {},
			wantErr: certlifecycle.ErrInvalidSerial,
		},
		{
			name: "duplicate serial number",
			meta: &certlifecycle.CertificateMetadata{
				SerialNumber: "aa:bb:cc:dd",
				Status:       certlifecycle.StatusActive,
				IssuedAt:     testNow,
				ExpiresAt:    testLater,
			},
			setup: func(db *mockDBTX) {
				db.execFunc = func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, certlifecycle.ExportNewPgError("23505")
				}
			},
			wantErr: certlifecycle.ErrCertificateExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockDBTX{}
			tt.setup(db)
			store := newMockPgStore(db)

			err := store.Create(context.Background(), tt.meta)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPostgresStore_Get(t *testing.T) {
	tests := []struct {
		name    string
		serial  string
		setup   func(*mockDBTX)
		wantErr error
	}{
		{
			name:   "existing certificate",
			serial: "aa:bb:cc:dd",
			setup: func(db *mockDBTX) {
				meta := testCertMeta()
				db.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return &mockRow{scanFunc: func(dest ...interface{}) error {
						return scanRowInto(dest, certMetaRow(meta))
					}}
				}
			},
			wantErr: nil,
		},
		{
			name:    "empty serial number",
			serial:  "",
			setup:   func(_ *mockDBTX) {},
			wantErr: certlifecycle.ErrInvalidSerial,
		},
		{
			name:   "not found",
			serial: "not-exist",
			setup: func(db *mockDBTX) {
				db.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(pgx.ErrNoRows)
				}
			},
			wantErr: certlifecycle.ErrCertificateNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockDBTX{}
			tt.setup(db)
			store := newMockPgStore(db)

			meta, err := store.Get(context.Background(), tt.serial)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, meta)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, meta)
				assert.Equal(t, "aa:bb:cc:dd", meta.SerialNumber)
				assert.Equal(t, certlifecycle.StatusActive, meta.Status)
			}
		})
	}
}

func TestPostgresStore_UpdateStatus(t *testing.T) {
	tests := []struct {
		name    string
		serial  string
		status  certlifecycle.CertificateStatus
		wantErr error
	}{
		{
			name:    "valid update",
			serial:  "aa:bb:cc:dd",
			status:  certlifecycle.StatusExpiringSoon,
			wantErr: nil,
		},
		{
			name:    "empty serial number",
			serial:  "",
			status:  certlifecycle.StatusExpiringSoon,
			wantErr: certlifecycle.ErrInvalidSerial,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockDBTX{}
			store := newMockPgStore(db)

			err := store.UpdateStatus(context.Background(), tt.serial, tt.status)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPostgresStore_MarkRevoked(t *testing.T) {
	tests := []struct {
		name    string
		serial  string
		wantErr error
	}{
		{name: "valid revocation", serial: "aa:bb:cc:dd", wantErr: nil},
		{name: "empty serial", serial: "", wantErr: certlifecycle.ErrInvalidSerial},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockDBTX{}
			store := newMockPgStore(db)

			err := store.MarkRevoked(context.Background(), tt.serial)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPostgresStore_MarkRenewed(t *testing.T) {
	tests := []struct {
		name      string
		serial    string
		newSerial string
		wantErr   error
	}{
		{name: "valid renewal", serial: "aa:bb:cc:dd", newSerial: "ee:ff:00:11", wantErr: nil},
		{name: "empty serial", serial: "", newSerial: "ee:ff:00:11", wantErr: certlifecycle.ErrInvalidSerial},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockDBTX{}
			store := newMockPgStore(db)

			err := store.MarkRenewed(context.Background(), tt.serial, tt.newSerial)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPostgresStore_MarkRenewalFailed(t *testing.T) {
	tests := []struct {
		name    string
		serial  string
		errMsg  string
		wantErr error
	}{
		{name: "valid failure", serial: "aa:bb:cc:dd", errMsg: "vault unavailable", wantErr: nil},
		{name: "empty serial", serial: "", errMsg: "error", wantErr: certlifecycle.ErrInvalidSerial},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockDBTX{}
			store := newMockPgStore(db)

			err := store.MarkRenewalFailed(context.Background(), tt.serial, tt.errMsg, testLater)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPostgresStore_ListByTenant(t *testing.T) {
	meta := testCertMeta()

	db := &mockDBTX{
		queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
			return &mockRows{
				data: [][]interface{}{certMetaRow(meta)},
			}, nil
		},
	}

	store := newMockPgStore(db)
	results, err := store.ListByTenant(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "aa:bb:cc:dd", results[0].SerialNumber)
	assert.Equal(t, "tenant-1", results[0].TenantID)
}

func TestPostgresStore_ListByUser(t *testing.T) {
	meta := testCertMeta()

	db := &mockDBTX{
		queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
			return &mockRows{
				data: [][]interface{}{certMetaRow(meta)},
			}, nil
		},
	}

	store := newMockPgStore(db)
	results, err := store.ListByUser(context.Background(), "user-1")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "user-1", results[0].UserID)
}

func TestPostgresStore_ListByStatus(t *testing.T) {
	meta := testCertMeta()

	db := &mockDBTX{
		queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
			return &mockRows{
				data: [][]interface{}{certMetaRow(meta)},
			}, nil
		},
	}

	store := newMockPgStore(db)
	results, err := store.ListByStatus(context.Background(), certlifecycle.StatusActive)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, certlifecycle.StatusActive, results[0].Status)
}

func TestPostgresStore_ListExpiring(t *testing.T) {
	meta := testCertMeta()

	db := &mockDBTX{
		queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
			return &mockRows{
				data: [][]interface{}{certMetaRow(meta)},
			}, nil
		},
	}

	store := newMockPgStore(db)
	results, err := store.ListExpiring(context.Background(), testLater.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestPostgresStore_ListRenewalFailed(t *testing.T) {
	meta := testCertMeta()
	meta.Status = "renewal_failed"
	meta.NextRetryAt = pgtype.Timestamptz{Time: testNow, Valid: true}

	db := &mockDBTX{
		queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
			return &mockRows{
				data: [][]interface{}{certMetaRow(meta)},
			}, nil
		},
	}

	store := newMockPgStore(db)
	results, err := store.ListRenewalFailed(context.Background(), testLater)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, certlifecycle.StatusRenewalFailed, results[0].Status)
}

func TestPostgresStore_CountByStatus(t *testing.T) {
	db := &mockDBTX{
		queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
			return &mockRows{
				data: [][]interface{}{
					{"active", int64(5)},
					{"expired", int64(2)},
				},
			}, nil
		},
	}

	store := newMockPgStore(db)
	counts, err := store.CountByStatus(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(5), counts[certlifecycle.StatusActive])
	assert.Equal(t, int64(2), counts[certlifecycle.StatusExpired])
}

func TestPostgresStore_Delete(t *testing.T) {
	tests := []struct {
		name    string
		serial  string
		setup   func(*mockDBTX)
		wantErr error
	}{
		{
			name:   "existing certificate",
			serial: "aa:bb:cc:dd",
			setup: func(db *mockDBTX) {
				db.execFunc = func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("DELETE 1"), nil
				}
			},
			wantErr: nil,
		},
		{
			name:    "empty serial",
			serial:  "",
			setup:   func(_ *mockDBTX) {},
			wantErr: certlifecycle.ErrInvalidSerial,
		},
		{
			name:   "not found",
			serial: "not-exist",
			setup: func(db *mockDBTX) {
				db.execFunc = func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("DELETE 0"), nil
				}
			},
			wantErr: certlifecycle.ErrCertificateNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockDBTX{}
			tt.setup(db)
			store := newMockPgStore(db)

			err := store.Delete(context.Background(), tt.serial)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPostgresStore_ListEmpty(t *testing.T) {
	db := &mockDBTX{
		queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
			return &mockRows{data: nil}, nil
		},
	}

	store := newMockPgStore(db)

	results, err := store.ListByTenant(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, results)
}
