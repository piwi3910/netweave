// postgres_unit_test.go contains unit tests for PostgresStore using a mock DBTX.
// These tests do NOT require a real PostgreSQL database — they mock the
// dbsqlc.DBTX interface (Exec/Query/QueryRow) to test all store methods
// in isolation. Run with: go test -short ./internal/auth/...
package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========================================================================
// Tenant Tests
// ========================================================================

func TestPostgresStore_CreateTenantUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tenant  *auth.Tenant
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			tenant: &auth.Tenant{
				ID:     "tenant-1",
				Name:   "ACME Corp",
				Status: auth.TenantStatusActive,
				Quota:  auth.DefaultQuota(),
			},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("INSERT 0 1"), nil
				},
			},
		},
		{
			name:   "empty ID returns ErrInvalidTenantID",
			tenant: &auth.Tenant{ID: "", Name: "No ID"},
			db:     &mockDBTX{},
			errIs:  auth.ErrInvalidTenantID,
		},
		{
			name: "duplicate returns ErrTenantExists",
			tenant: &auth.Tenant{
				ID:    "tenant-dup",
				Name:  "Dup",
				Quota: auth.DefaultQuota(),
			},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, auth.ExportNewPgError("23505")
				},
			},
			errIs: auth.ErrTenantExists,
		},
		{
			name: "database error",
			tenant: &auth.Tenant{
				ID:    "tenant-err",
				Name:  "Err",
				Quota: auth.DefaultQuota(),
			},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("connection refused")
				},
			},
			wantErr: "failed to create tenant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.CreateTenant(context.Background(), tt.tenant)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_GetTenantUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	quotaJSON, _ := json.Marshal(auth.DefaultQuota())
	usageJSON, _ := json.Marshal(auth.TenantUsage{Subscriptions: 5})
	metadataJSON := json.RawMessage(`{"env":"prod"}`)

	tests := []struct {
		name    string
		id      string
		db      *mockDBTX
		wantID  string
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			id:   "tenant-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return &mockRow{scanFunc: func(dest ...interface{}) error {
						return scanRowInto(dest, tenantRow(&dbsqlc.Tenant{
							ID:           "tenant-1",
							Name:         "ACME",
							Description:  "Test",
							Status:       "active",
							Quota:        json.RawMessage(quotaJSON),
							Usage:        json.RawMessage(usageJSON),
							ContactEmail: "a@b.com",
							Metadata:     metadataJSON,
							CreatedAt:    now,
							UpdatedAt:    now,
						}))
					}}
				},
			},
			wantID: "tenant-1",
		},
		{
			name: "not found",
			id:   "nonexistent",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(pgx.ErrNoRows)
				},
			},
			errIs: auth.ErrTenantNotFound,
		},
		{
			name:  "empty ID",
			id:    "",
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidTenantID,
		},
		{
			name: "database error",
			id:   "tenant-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("connection lost"))
				},
			},
			wantErr: "failed to get tenant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.GetTenant(context.Background(), tt.id)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestPostgresStore_UpdateTenantUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tenant  *auth.Tenant
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			tenant: &auth.Tenant{
				ID:    "tenant-1",
				Name:  "Updated",
				Quota: auth.DefaultQuota(),
			},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(true)
				},
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
			},
		},
		{
			name:   "empty ID",
			tenant: &auth.Tenant{ID: ""},
			db:     &mockDBTX{},
			errIs:  auth.ErrInvalidTenantID,
		},
		{
			name:   "not found",
			tenant: &auth.Tenant{ID: "nonexistent", Quota: auth.DefaultQuota()},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(false)
				},
			},
			errIs: auth.ErrTenantNotFound,
		},
		{
			name:   "existence check error",
			tenant: &auth.Tenant{ID: "tenant-1", Quota: auth.DefaultQuota()},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("db error"))
				},
			},
			wantErr: "failed to check tenant existence",
		},
		{
			name:   "update exec error",
			tenant: &auth.Tenant{ID: "tenant-1", Quota: auth.DefaultQuota()},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(true)
				},
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("write error")
				},
			},
			wantErr: "failed to update tenant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.UpdateTenant(context.Background(), tt.tenant)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_DeleteTenantUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			id:   "tenant-1",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("DELETE 1"), nil
				},
			},
		},
		{
			name:  "empty ID",
			id:    "",
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidTenantID,
		},
		{
			name: "not found",
			id:   "nonexistent",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("DELETE 0"), nil
				},
			},
			errIs: auth.ErrTenantNotFound,
		},
		{
			name: "database error",
			id:   "tenant-1",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("connection error")
				},
			},
			wantErr: "failed to delete tenant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.DeleteTenant(context.Background(), tt.id)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_ListTenantsUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	quotaJSON, _ := json.Marshal(auth.DefaultQuota())
	usageJSON, _ := json.Marshal(auth.TenantUsage{})

	tests := []struct {
		name    string
		db      *mockDBTX
		want    int
		wantErr string
	}{
		{
			name: "success with results",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{
						data: [][]interface{}{
							tenantRow(&dbsqlc.Tenant{
								ID: "t-1", Name: "First", Status: "active",
								Quota: json.RawMessage(quotaJSON), Usage: json.RawMessage(usageJSON),
								Metadata: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now,
							}),
							tenantRow(&dbsqlc.Tenant{
								ID: "t-2", Name: "Second", Status: "suspended",
								Quota: json.RawMessage(quotaJSON), Usage: json.RawMessage(usageJSON),
								Metadata: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now,
							}),
						},
					}, nil
				},
			},
			want: 2,
		},
		{
			name: "empty results",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{data: [][]interface{}{}}, nil
				},
			},
			want: 0,
		},
		{
			name: "database error",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return nil, fmt.Errorf("query error")
				},
			},
			wantErr: "failed to list tenants",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.ListTenants(context.Background())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}

// ========================================================================
// IncrementUsage / DecrementUsage Tests
// ========================================================================

func TestPostgresStore_IncrementUsageUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			id:   "tenant-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(true)
				},
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
			},
		},
		{
			name:  "empty ID",
			id:    "",
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidTenantID,
		},
		{
			name: "tenant not found",
			id:   "nonexistent",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(false)
				},
			},
			errIs: auth.ErrTenantNotFound,
		},
		{
			name: "existence check error",
			id:   "tenant-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("db error"))
				},
			},
			wantErr: "failed to check tenant existence",
		},
		{
			name: "increment exec error",
			id:   "tenant-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(true)
				},
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("write error")
				},
			},
			wantErr: "failed to increment tenant usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.IncrementUsage(context.Background(), tt.id, "subscriptions")
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_DecrementUsageUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			id:   "tenant-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(true)
				},
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
			},
		},
		{
			name:  "empty ID",
			id:    "",
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidTenantID,
		},
		{
			name: "tenant not found",
			id:   "nonexistent",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(false)
				},
			},
			errIs: auth.ErrTenantNotFound,
		},
		{
			name: "existence check error",
			id:   "tenant-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("db error"))
				},
			},
			wantErr: "failed to check tenant existence",
		},
		{
			name: "decrement exec error",
			id:   "tenant-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(true)
				},
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("write error")
				},
			},
			wantErr: "failed to decrement tenant usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.DecrementUsage(context.Background(), tt.id, "subscriptions")
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// ========================================================================
// User Tests
// ========================================================================

func TestPostgresStore_CreateUserUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		user    *auth.TenantUser
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			user: &auth.TenantUser{
				ID:         "user-1",
				TenantID:   "tenant-1",
				CommonName: "alice",
				RoleID:     "role-1",
				IsActive:   true,
			},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("INSERT 0 1"), nil
				},
			},
		},
		{
			name:  "empty ID",
			user:  &auth.TenantUser{ID: ""},
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidUserID,
		},
		{
			name: "duplicate returns ErrUserExists",
			user: &auth.TenantUser{ID: "user-dup", CommonName: "dup"},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, auth.ExportNewPgError("23505")
				},
			},
			errIs: auth.ErrUserExists,
		},
		{
			name: "database error",
			user: &auth.TenantUser{ID: "user-err", CommonName: "err"},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("connection refused")
				},
			},
			wantErr: "failed to create user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.CreateUser(context.Background(), tt.user)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_GetUserUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		id      string
		db      *mockDBTX
		wantID  string
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			id:   "user-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return &mockRow{scanFunc: func(dest ...interface{}) error {
						return scanRowInto(dest, userRow(&dbsqlc.User{
							ID: "user-1", TenantID: "t1", Subject: "CN=alice",
							CommonName: "alice", Email: "a@b.com",
							OauthSubject: "oa-1", OauthProvider: "kc",
							RoleID: "r1", IsActive: true,
							LastLoginAt: pgtype.Timestamptz{Time: now, Valid: true},
							CreatedAt:   now, UpdatedAt: now,
						}))
					}}
				},
			},
			wantID: "user-1",
		},
		{
			name: "not found",
			id:   "nonexistent",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(pgx.ErrNoRows)
				},
			},
			errIs: auth.ErrUserNotFound,
		},
		{
			name:  "empty ID",
			id:    "",
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidUserID,
		},
		{
			name: "database error",
			id:   "user-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("connection lost"))
				},
			},
			wantErr: "failed to get user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.GetUser(context.Background(), tt.id)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestPostgresStore_GetUserBySubjectUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		subject string
		db      *mockDBTX
		wantID  string
		wantErr string
		errIs   error
	}{
		{
			name:    "success",
			subject: "CN=alice,O=ACME",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return &mockRow{scanFunc: func(dest ...interface{}) error {
						return scanRowInto(dest, userRow(&dbsqlc.User{
							ID: "user-1", TenantID: "t1", Subject: "CN=alice,O=ACME",
							CommonName: "alice", RoleID: "r1", IsActive: true,
							LastLoginAt: pgtype.Timestamptz{Valid: false},
							CreatedAt:   now, UpdatedAt: now,
						}))
					}}
				},
			},
			wantID: "user-1",
		},
		{
			name:    "not found",
			subject: "CN=nobody",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(pgx.ErrNoRows)
				},
			},
			errIs: auth.ErrUserNotFound,
		},
		{
			name:    "database error",
			subject: "CN=alice",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("db error"))
				},
			},
			wantErr: "failed to get user by subject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.GetUserBySubject(context.Background(), tt.subject)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestPostgresStore_GetUserByOAuthSubjectUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		oauth   string
		db      *mockDBTX
		wantID  string
		wantErr string
		errIs   error
	}{
		{
			name:  "success",
			oauth: "kc-123",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return &mockRow{scanFunc: func(dest ...interface{}) error {
						return scanRowInto(dest, userRow(&dbsqlc.User{
							ID: "user-1", TenantID: "t1", OauthSubject: "kc-123",
							OauthProvider: "keycloak", CommonName: "alice",
							RoleID: "r1", IsActive: true,
							LastLoginAt: pgtype.Timestamptz{Valid: false},
							CreatedAt:   now, UpdatedAt: now,
						}))
					}}
				},
			},
			wantID: "user-1",
		},
		{
			name:  "not found",
			oauth: "nonexistent",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(pgx.ErrNoRows)
				},
			},
			errIs: auth.ErrUserNotFound,
		},
		{
			name:  "database error",
			oauth: "kc-123",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("db error"))
				},
			},
			wantErr: "failed to get user by OAuth subject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.GetUserByOAuthSubject(context.Background(), tt.oauth)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestPostgresStore_GetUserByEmailUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		email   string
		db      *mockDBTX
		wantID  string
		wantErr string
		errIs   error
	}{
		{
			name:  "success",
			email: "alice@acme.com",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return &mockRow{scanFunc: func(dest ...interface{}) error {
						return scanRowInto(dest, userRow(&dbsqlc.User{
							ID: "user-1", TenantID: "t1", Email: "alice@acme.com",
							CommonName: "alice", RoleID: "r1", IsActive: true,
							LastLoginAt: pgtype.Timestamptz{Valid: false},
							CreatedAt:   now, UpdatedAt: now,
						}))
					}}
				},
			},
			wantID: "user-1",
		},
		{
			name:  "not found",
			email: "missing@acme.com",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(pgx.ErrNoRows)
				},
			},
			errIs: auth.ErrUserNotFound,
		},
		{
			name:  "database error",
			email: "alice@acme.com",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("db error"))
				},
			},
			wantErr: "failed to get user by email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.GetUserByEmail(context.Background(), tt.email)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestPostgresStore_UpdateUserUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		user    *auth.TenantUser
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			user: &auth.TenantUser{ID: "user-1", CommonName: "updated", RoleID: "r1"},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(true)
				},
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
			},
		},
		{
			name:  "empty ID",
			user:  &auth.TenantUser{ID: ""},
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidUserID,
		},
		{
			name: "not found",
			user: &auth.TenantUser{ID: "nonexistent"},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(false)
				},
			},
			errIs: auth.ErrUserNotFound,
		},
		{
			name: "existence check error",
			user: &auth.TenantUser{ID: "user-1"},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("db error"))
				},
			},
			wantErr: "failed to check user existence",
		},
		{
			name: "update exec error",
			user: &auth.TenantUser{ID: "user-1"},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(true)
				},
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("write error")
				},
			},
			wantErr: "failed to update user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.UpdateUser(context.Background(), tt.user)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_DeleteUserUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			id:   "user-1",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("DELETE 1"), nil
				},
			},
		},
		{
			name:  "empty ID",
			id:    "",
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidUserID,
		},
		{
			name: "not found",
			id:   "nonexistent",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("DELETE 0"), nil
				},
			},
			errIs: auth.ErrUserNotFound,
		},
		{
			name: "database error",
			id:   "user-1",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("connection error")
				},
			},
			wantErr: "failed to delete user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.DeleteUser(context.Background(), tt.id)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_ListUsersByTenantUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		db      *mockDBTX
		want    int
		wantErr string
	}{
		{
			name: "success with results",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{
						data: [][]interface{}{
							userRow(&dbsqlc.User{
								ID: "u-1", TenantID: "t1", CommonName: "alice",
								RoleID: "r1", IsActive: true,
								LastLoginAt: pgtype.Timestamptz{Valid: false},
								CreatedAt:   now, UpdatedAt: now,
							}),
							userRow(&dbsqlc.User{
								ID: "u-2", TenantID: "t1", CommonName: "bob",
								RoleID: "r2", IsActive: false,
								LastLoginAt: pgtype.Timestamptz{Valid: false},
								CreatedAt:   now, UpdatedAt: now,
							}),
						},
					}, nil
				},
			},
			want: 2,
		},
		{
			name: "empty results",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{data: [][]interface{}{}}, nil
				},
			},
			want: 0,
		},
		{
			name: "database error",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return nil, fmt.Errorf("query error")
				},
			},
			wantErr: "failed to list users by tenant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.ListUsersByTenant(context.Background(), "t1")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}

func TestPostgresStore_UpdateLastLoginUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			id:   "user-1",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
			},
		},
		{
			name:  "empty ID",
			id:    "",
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidUserID,
		},
		{
			name: "database error",
			id:   "user-1",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("db error")
				},
			},
			wantErr: "failed to update last login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.UpdateLastLogin(context.Background(), tt.id)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// ========================================================================
// Role Tests
// ========================================================================

func TestPostgresStore_CreateRoleUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		role    *auth.Role
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			role: &auth.Role{
				ID:          "role-1",
				Name:        auth.RoleOperator,
				Type:        auth.RoleTypeTenant,
				Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
			},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("INSERT 0 1"), nil
				},
			},
		},
		{
			name:  "empty ID",
			role:  &auth.Role{ID: ""},
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidRoleID,
		},
		{
			name: "duplicate returns ErrRoleExists",
			role: &auth.Role{
				ID:          "role-dup",
				Name:        "dup",
				Permissions: []auth.Permission{},
			},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, auth.ExportNewPgError("23505")
				},
			},
			errIs: auth.ErrRoleExists,
		},
		{
			name: "database error",
			role: &auth.Role{
				ID:          "role-err",
				Name:        "err",
				Permissions: []auth.Permission{},
			},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("connection refused")
				},
			},
			wantErr: "failed to create role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.CreateRole(context.Background(), tt.role)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_GetRoleUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	permsJSON := json.RawMessage(`["subscriptions:read","resources:read"]`)

	tests := []struct {
		name    string
		id      string
		db      *mockDBTX
		wantID  string
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			id:   "role-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return &mockRow{scanFunc: func(dest ...interface{}) error {
						return scanRowInto(dest, roleRow(&dbsqlc.Role{
							ID: "role-1", Name: "operator", Type: "tenant",
							Description: "Op role", Permissions: permsJSON,
							TenantID: "t1", CreatedAt: now, UpdatedAt: now,
						}))
					}}
				},
			},
			wantID: "role-1",
		},
		{
			name: "not found",
			id:   "nonexistent",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(pgx.ErrNoRows)
				},
			},
			errIs: auth.ErrRoleNotFound,
		},
		{
			name:  "empty ID",
			id:    "",
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidRoleID,
		},
		{
			name: "database error",
			id:   "role-1",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("connection lost"))
				},
			},
			wantErr: "failed to get role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.GetRole(context.Background(), tt.id)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestPostgresStore_GetRoleByNameUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	permsJSON := json.RawMessage(`["subscriptions:read"]`)

	tests := []struct {
		name     string
		roleName auth.RoleName
		db       *mockDBTX
		wantID   string
		wantErr  string
		errIs    error
	}{
		{
			name:     "success",
			roleName: auth.RoleOperator,
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return &mockRow{scanFunc: func(dest ...interface{}) error {
						return scanRowInto(dest, roleRow(&dbsqlc.Role{
							ID: "role-1", Name: "operator", Type: "tenant",
							Permissions: permsJSON,
							CreatedAt:   now, UpdatedAt: now,
						}))
					}}
				},
			},
			wantID: "role-1",
		},
		{
			name:     "not found",
			roleName: "nonexistent",
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(pgx.ErrNoRows)
				},
			},
			errIs: auth.ErrRoleNotFound,
		},
		{
			name:     "database error",
			roleName: auth.RoleOperator,
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("db error"))
				},
			},
			wantErr: "failed to get role by name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.GetRoleByName(context.Background(), tt.roleName)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestPostgresStore_UpdateRoleUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		role    *auth.Role
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			role: &auth.Role{
				ID:          "role-1",
				Name:        auth.RoleOperator,
				Type:        auth.RoleTypeTenant,
				Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
			},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(true)
				},
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
			},
		},
		{
			name:  "empty ID",
			role:  &auth.Role{ID: ""},
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidRoleID,
		},
		{
			name: "not found",
			role: &auth.Role{ID: "nonexistent", Permissions: []auth.Permission{}},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(false)
				},
			},
			errIs: auth.ErrRoleNotFound,
		},
		{
			name: "existence check error",
			role: &auth.Role{ID: "role-1", Permissions: []auth.Permission{}},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return errorRow(fmt.Errorf("db error"))
				},
			},
			wantErr: "failed to check role existence",
		},
		{
			name: "update exec error",
			role: &auth.Role{ID: "role-1", Permissions: []auth.Permission{}},
			db: &mockDBTX{
				queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
					return boolExistsRow(true)
				},
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("write error")
				},
			},
			wantErr: "failed to update role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.UpdateRole(context.Background(), tt.role)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_DeleteRoleUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		db      *mockDBTX
		wantErr string
		errIs   error
	}{
		{
			name: "success",
			id:   "role-1",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("DELETE 1"), nil
				},
			},
		},
		{
			name:  "empty ID",
			id:    "",
			db:    &mockDBTX{},
			errIs: auth.ErrInvalidRoleID,
		},
		{
			name: "not found",
			id:   "nonexistent",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("DELETE 0"), nil
				},
			},
			errIs: auth.ErrRoleNotFound,
		},
		{
			name: "database error",
			id:   "role-1",
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("connection error")
				},
			},
			wantErr: "failed to delete role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.DeleteRole(context.Background(), tt.id)
			if tt.errIs != nil {
				require.ErrorIs(t, err, tt.errIs)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_ListRolesUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	permsJSON := json.RawMessage(`["subscriptions:read"]`)

	tests := []struct {
		name    string
		db      *mockDBTX
		want    int
		wantErr string
	}{
		{
			name: "success with results",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{
						data: [][]interface{}{
							roleRow(&dbsqlc.Role{
								ID: "r-1", Name: "admin", Type: "platform",
								Permissions: permsJSON, CreatedAt: now, UpdatedAt: now,
							}),
							roleRow(&dbsqlc.Role{
								ID: "r-2", Name: "viewer", Type: "tenant",
								Permissions: permsJSON, CreatedAt: now, UpdatedAt: now,
							}),
						},
					}, nil
				},
			},
			want: 2,
		},
		{
			name: "empty results",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{data: [][]interface{}{}}, nil
				},
			},
			want: 0,
		},
		{
			name: "database error",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return nil, fmt.Errorf("query error")
				},
			},
			wantErr: "failed to list roles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.ListRoles(context.Background())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}

func TestPostgresStore_ListRolesByTenantUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	permsJSON := json.RawMessage(`["subscriptions:read"]`)

	tests := []struct {
		name    string
		db      *mockDBTX
		want    int
		wantErr string
	}{
		{
			name: "success with results",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{
						data: [][]interface{}{
							roleRow(&dbsqlc.Role{
								ID: "r-global", Name: "admin", Type: "platform",
								Permissions: permsJSON, CreatedAt: now, UpdatedAt: now,
							}),
							roleRow(&dbsqlc.Role{
								ID: "r-tenant", Name: "custom", Type: "tenant",
								TenantID:    "t1",
								Permissions: permsJSON, CreatedAt: now, UpdatedAt: now,
							}),
						},
					}, nil
				},
			},
			want: 2,
		},
		{
			name: "database error",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return nil, fmt.Errorf("query error")
				},
			},
			wantErr: "failed to list roles by tenant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.ListRolesByTenant(context.Background(), "t1")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}

func TestPostgresStore_InitializeDefaultRolesUnit(t *testing.T) {
	t.Parallel()

	t.Run("all roles created successfully", func(t *testing.T) {
		t.Parallel()
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				// RoleExists always returns false so roles get created.
				return boolExistsRow(false)
			},
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}
		store := newMockPgStore(db)
		err := store.InitializeDefaultRoles(context.Background())
		require.NoError(t, err)
	})

	t.Run("roles already exist skipped", func(t *testing.T) {
		t.Parallel()
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				// RoleExists always returns true.
				return boolExistsRow(true)
			},
		}
		store := newMockPgStore(db)
		err := store.InitializeDefaultRoles(context.Background())
		require.NoError(t, err)
	})

	t.Run("existence check error", func(t *testing.T) {
		t.Parallel()
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return errorRow(fmt.Errorf("db error"))
			},
		}
		store := newMockPgStore(db)
		err := store.InitializeDefaultRoles(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check default role existence")
	})

	t.Run("create role error (not ErrRoleExists)", func(t *testing.T) {
		t.Parallel()
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return boolExistsRow(false)
			},
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, fmt.Errorf("insert error")
			},
		}
		store := newMockPgStore(db)
		err := store.InitializeDefaultRoles(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create default role")
	})

	t.Run("concurrent ErrRoleExists is ignored", func(t *testing.T) {
		t.Parallel()
		// Simulate: RoleExists returns false, but CreateRole gets unique violation
		// (race condition). Should be silently ignored.
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return boolExistsRow(false)
			},
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, auth.ExportNewPgError("23505")
			},
		}
		store := newMockPgStore(db)
		err := store.InitializeDefaultRoles(context.Background())
		require.NoError(t, err)
	})
}

// ========================================================================
// Audit Tests
// ========================================================================

func TestPostgresStore_LogEventUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		event   *auth.AuditEvent
		db      *mockDBTX
		wantErr string
	}{
		{
			name: "success",
			event: &auth.AuditEvent{
				ID:        "evt-1",
				Type:      auth.AuditEventTenantCreated,
				TenantID:  "t1",
				UserID:    "u1",
				Action:    "create",
				Details:   map[string]string{"name": "ACME"},
				Timestamp: time.Now().UTC(),
			},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("INSERT 0 1"), nil
				},
			},
		},
		{
			name: "success with zero timestamp (auto-set)",
			event: &auth.AuditEvent{
				ID:     "evt-2",
				Type:   auth.AuditEventAuthSuccess,
				Action: "login",
			},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("INSERT 0 1"), nil
				},
			},
		},
		{
			name: "database error",
			event: &auth.AuditEvent{
				ID:     "evt-3",
				Type:   auth.AuditEventAuthFailure,
				Action: "login",
			},
			db: &mockDBTX{
				execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, fmt.Errorf("insert error")
				},
			},
			wantErr: "failed to log audit event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			err := store.LogEvent(context.Background(), tt.event)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostgresStore_ListEventsUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		db      *mockDBTX
		want    int
		wantErr string
	}{
		{
			name: "success with results",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{
						data: [][]interface{}{
							auditEventRow(&dbsqlc.AuditEvent{
								ID: "e-1", Type: "auth.success", TenantID: "t1",
								UserID: "u1", Action: "login",
								Details:   json.RawMessage(`{}`),
								Timestamp: now,
							}),
							auditEventRow(&dbsqlc.AuditEvent{
								ID: "e-2", Type: "tenant.created", TenantID: "t1",
								UserID: "u1", Action: "create",
								Details:   json.RawMessage(`{"name":"ACME"}`),
								Timestamp: now,
							}),
						},
					}, nil
				},
			},
			want: 2,
		},
		{
			name: "empty results",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{data: [][]interface{}{}}, nil
				},
			},
			want: 0,
		},
		{
			name: "database error",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return nil, fmt.Errorf("query error")
				},
			},
			wantErr: "failed to list audit events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.ListEvents(context.Background(), "t1", 100, 0)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}

func TestPostgresStore_ListEventsByTypeUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		db      *mockDBTX
		want    int
		wantErr string
	}{
		{
			name: "success",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{
						data: [][]interface{}{
							auditEventRow(&dbsqlc.AuditEvent{
								ID: "e-1", Type: "auth.success", Action: "login",
								Details: json.RawMessage(`{}`), Timestamp: now,
							}),
						},
					}, nil
				},
			},
			want: 1,
		},
		{
			name: "database error",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return nil, fmt.Errorf("query error")
				},
			},
			wantErr: "failed to list audit events by type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.ListEventsByType(context.Background(), auth.AuditEventAuthSuccess, 100)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}

func TestPostgresStore_ListEventsByUserUnit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		db      *mockDBTX
		want    int
		wantErr string
	}{
		{
			name: "success",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return &mockRows{
						data: [][]interface{}{
							auditEventRow(&dbsqlc.AuditEvent{
								ID: "e-1", Type: "auth.success", UserID: "u1",
								Action: "login", Details: json.RawMessage(`{}`),
								Timestamp: now,
							}),
							auditEventRow(&dbsqlc.AuditEvent{
								ID: "e-2", Type: "tenant.created", UserID: "u1",
								Action: "create", Details: json.RawMessage(`{}`),
								Timestamp: now,
							}),
						},
					}, nil
				},
			},
			want: 2,
		},
		{
			name: "database error",
			db: &mockDBTX{
				queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
					return nil, fmt.Errorf("query error")
				},
			},
			wantErr: "failed to list audit events by user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMockPgStore(tt.db)
			got, err := store.ListEventsByUser(context.Background(), "u1", 100)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}

// ========================================================================
// NewPostgresStore / Close / Ping (constructor tests using mock)
// ========================================================================

func TestPostgresStore_NewPostgresStoreUnit(t *testing.T) {
	t.Parallel()
	// Verify that ExportNewTestPostgresStore creates a usable store.
	db := &mockDBTX{}
	store := newMockPgStore(db)
	require.NotNil(t, store)
}
