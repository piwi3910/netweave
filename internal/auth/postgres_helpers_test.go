package auth

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/database/dbsqlc"
)

func TestMarshalPermissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		perms   []Permission
		want    string
		wantErr bool
	}{
		{
			name:  "multiple permissions",
			perms: []Permission{PermissionSubscriptionRead, PermissionSubscriptionCreate},
			want:  `["subscriptions:read","subscriptions:create"]`,
		},
		{
			name:  "single permission",
			perms: []Permission{PermissionUserRead},
			want:  `["users:read"]`,
		},
		{
			name:  "empty slice",
			perms: []Permission{},
			want:  `[]`,
		},
		{
			name:  "nil slice",
			perms: nil,
			want:  `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := marshalPermissions(tt.perms)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}

func TestUnmarshalPermissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		data    json.RawMessage
		want    []Permission
		wantErr bool
	}{
		{
			name: "multiple permissions",
			data: json.RawMessage(`["subscriptions:read","subscriptions:create"]`),
			want: []Permission{PermissionSubscriptionRead, PermissionSubscriptionCreate},
		},
		{
			name: "single permission",
			data: json.RawMessage(`["users:read"]`),
			want: []Permission{PermissionUserRead},
		},
		{
			name: "empty array",
			data: json.RawMessage(`[]`),
			want: []Permission{},
		},
		{
			name:    "invalid JSON",
			data:    json.RawMessage(`not json`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := unmarshalPermissions(tt.data)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMarshalStringMap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		m       map[string]string
		want    string
		wantErr bool
	}{
		{
			name: "non-empty map",
			m:    map[string]string{"key1": "val1", "key2": "val2"},
			want: `{"key1":"val1","key2":"val2"}`,
		},
		{
			name: "empty map",
			m:    map[string]string{},
			want: `{}`,
		},
		{
			name: "nil map defaults to empty object",
			m:    nil,
			want: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := marshalStringMap(tt.m)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}

func TestTimeToTimestamptz(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     time.Time
		wantValid bool
	}{
		{
			name:      "non-zero time",
			input:     time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			wantValid: true,
		},
		{
			name:      "zero time returns invalid",
			input:     time.Time{},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := timeToTimestamptz(tt.input)
			assert.Equal(t, tt.wantValid, got.Valid)
			if tt.wantValid {
				assert.Equal(t, tt.input, got.Time)
			}
		})
	}
}

func TestSafeIntToInt32(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input int
		want  int32
	}{
		{
			name:  "normal value",
			input: 42,
			want:  42,
		},
		{
			name:  "zero",
			input: 0,
			want:  0,
		},
		{
			name:  "negative value",
			input: -100,
			want:  -100,
		},
		{
			name:  "max int32",
			input: math.MaxInt32,
			want:  math.MaxInt32,
		},
		{
			name:  "exceeds max int32",
			input: math.MaxInt32 + 1,
			want:  math.MaxInt32,
		},
		{
			name:  "min int32",
			input: math.MinInt32,
			want:  math.MinInt32,
		},
		{
			name:  "below min int32",
			input: math.MinInt32 - 1,
			want:  math.MinInt32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := safeIntToInt32(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsPgUniqueViolation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "unique violation error",
			err:  &pgconn.PgError{Code: "23505"},
			want: true,
		},
		{
			name: "different pg error code",
			err:  &pgconn.PgError{Code: "23503"},
			want: false,
		},
		{
			name: "non-pg error",
			err:  assert.AnError,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPgUniqueViolation(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDbsqlcUserToModel(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	loginTime := now.Add(-1 * time.Hour)

	tests := []struct {
		name string
		row  *dbsqlc.User
		want *TenantUser
	}{
		{
			name: "full user with last login",
			row: &dbsqlc.User{
				ID:            "user-1",
				TenantID:      "tenant-1",
				Subject:       "CN=alice,O=ACME",
				CommonName:    "alice",
				Email:         "alice@example.com",
				OauthSubject:  "oauth-sub-123",
				OauthProvider: "keycloak",
				RoleID:        "role-1",
				IsActive:      true,
				LastLoginAt:   pgtype.Timestamptz{Time: loginTime, Valid: true},
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			want: &TenantUser{
				ID:            "user-1",
				TenantID:      "tenant-1",
				Subject:       "CN=alice,O=ACME",
				CommonName:    "alice",
				Email:         "alice@example.com",
				OAuthSubject:  "oauth-sub-123",
				OAuthProvider: "keycloak",
				RoleID:        "role-1",
				IsActive:      true,
				LastLoginAt:   loginTime,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
		},
		{
			name: "user without last login",
			row: &dbsqlc.User{
				ID:          "user-2",
				TenantID:    "tenant-1",
				CommonName:  "bob",
				RoleID:      "role-2",
				IsActive:    false,
				LastLoginAt: pgtype.Timestamptz{Valid: false},
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			want: &TenantUser{
				ID:         "user-2",
				TenantID:   "tenant-1",
				CommonName: "bob",
				RoleID:     "role-2",
				IsActive:   false,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := dbsqlcUserToModel(tt.row)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDbsqlcUsersToModels(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	rows := []dbsqlc.User{
		{
			ID:          "user-1",
			TenantID:    "tenant-1",
			CommonName:  "alice",
			RoleID:      "role-1",
			IsActive:    true,
			LastLoginAt: pgtype.Timestamptz{Valid: false},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "user-2",
			TenantID:    "tenant-1",
			CommonName:  "bob",
			RoleID:      "role-2",
			IsActive:    false,
			LastLoginAt: pgtype.Timestamptz{Valid: false},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}

	users := dbsqlcUsersToModels(rows)
	assert.Len(t, users, 2)
	assert.Equal(t, "user-1", users[0].ID)
	assert.Equal(t, "user-2", users[1].ID)
}

func TestDbsqlcRoleToModel(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		row     *dbsqlc.Role
		want    *Role
		wantErr bool
	}{
		{
			name: "valid role with permissions",
			row: &dbsqlc.Role{
				ID:          "role-1",
				Name:        "operator",
				Type:        "tenant",
				Description: "Operator role",
				Permissions: json.RawMessage(`["subscriptions:read","resources:read"]`),
				TenantID:    "tenant-1",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			want: &Role{
				ID:          "role-1",
				Name:        RoleName("operator"),
				Type:        RoleTypeTenant,
				Description: "Operator role",
				Permissions: []Permission{PermissionSubscriptionRead, PermissionResourceRead},
				TenantID:    "tenant-1",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
		{
			name: "invalid permissions JSON",
			row: &dbsqlc.Role{
				ID:          "role-2",
				Name:        "bad",
				Permissions: json.RawMessage(`not valid json`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := dbsqlcRoleToModel(tt.row)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDbsqlcRolesToModels(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	t.Run("valid roles", func(t *testing.T) {
		t.Parallel()
		rows := []dbsqlc.Role{
			{
				ID:          "role-1",
				Name:        "admin",
				Type:        "platform",
				Permissions: json.RawMessage(`["users:read"]`),
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			{
				ID:          "role-2",
				Name:        "viewer",
				Type:        "tenant",
				Permissions: json.RawMessage(`["resources:read"]`),
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		}

		roles, err := dbsqlcRolesToModels(rows)
		require.NoError(t, err)
		assert.Len(t, roles, 2)
		assert.Equal(t, "role-1", roles[0].ID)
		assert.Equal(t, "role-2", roles[1].ID)
	})

	t.Run("invalid permissions stops iteration", func(t *testing.T) {
		t.Parallel()
		rows := []dbsqlc.Role{
			{
				ID:          "role-1",
				Name:        "admin",
				Permissions: json.RawMessage(`invalid`),
			},
		}

		_, err := dbsqlcRolesToModels(rows)
		require.Error(t, err)
	})
}

func TestDbsqlcTenantToModel(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		row     *dbsqlc.Tenant
		wantErr bool
	}{
		{
			name: "valid tenant",
			row: &dbsqlc.Tenant{
				ID:           "tenant-1",
				Name:         "Test Tenant",
				Description:  "Test Description",
				Status:       "active",
				Quota:        json.RawMessage(`{"maxSubscriptions":100,"maxResourcePools":50,"maxDeployments":200,"maxUsers":20,"maxRequestsPerMinute":1000}`),
				Usage:        json.RawMessage(`{"subscriptions":5,"resourcePools":2,"deployments":10,"users":8}`),
				ContactEmail: "admin@test.com",
				Metadata:     json.RawMessage(`{"env":"prod"}`),
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		{
			name: "tenant with empty metadata",
			row: &dbsqlc.Tenant{
				ID:       "tenant-2",
				Name:     "Empty Meta",
				Status:   "active",
				Quota:    json.RawMessage(`{}`),
				Usage:    json.RawMessage(`{}`),
				Metadata: json.RawMessage(`{}`),
			},
		},
		{
			name: "invalid quota JSON",
			row: &dbsqlc.Tenant{
				ID:       "tenant-3",
				Quota:    json.RawMessage(`invalid`),
				Usage:    json.RawMessage(`{}`),
				Metadata: json.RawMessage(`{}`),
			},
			wantErr: true,
		},
		{
			name: "invalid usage JSON",
			row: &dbsqlc.Tenant{
				ID:       "tenant-4",
				Quota:    json.RawMessage(`{}`),
				Usage:    json.RawMessage(`invalid`),
				Metadata: json.RawMessage(`{}`),
			},
			wantErr: true,
		},
		{
			name: "invalid metadata JSON",
			row: &dbsqlc.Tenant{
				ID:       "tenant-5",
				Quota:    json.RawMessage(`{}`),
				Usage:    json.RawMessage(`{}`),
				Metadata: json.RawMessage(`{"invalid`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := dbsqlcTenantToModel(tt.row)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.row.ID, got.ID)
			assert.Equal(t, tt.row.Name, got.Name)
		})
	}
}

func TestDbsqlcTenantsToModels(t *testing.T) {
	t.Parallel()
	t.Run("valid tenants", func(t *testing.T) {
		t.Parallel()
		rows := []dbsqlc.Tenant{
			{
				ID:       "tenant-1",
				Name:     "T1",
				Status:   "active",
				Quota:    json.RawMessage(`{}`),
				Usage:    json.RawMessage(`{}`),
				Metadata: json.RawMessage(`{}`),
			},
			{
				ID:       "tenant-2",
				Name:     "T2",
				Status:   "suspended",
				Quota:    json.RawMessage(`{}`),
				Usage:    json.RawMessage(`{}`),
				Metadata: json.RawMessage(`{}`),
			},
		}

		tenants, err := dbsqlcTenantsToModels(rows)
		require.NoError(t, err)
		assert.Len(t, tenants, 2)
	})

	t.Run("invalid tenant stops iteration", func(t *testing.T) {
		t.Parallel()
		rows := []dbsqlc.Tenant{
			{
				ID:       "tenant-1",
				Quota:    json.RawMessage(`invalid`),
				Usage:    json.RawMessage(`{}`),
				Metadata: json.RawMessage(`{}`),
			},
		}

		_, err := dbsqlcTenantsToModels(rows)
		require.Error(t, err)
	})
}

func TestDbsqlcAuditEventToModel(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name    string
		row     *dbsqlc.AuditEvent
		wantErr bool
	}{
		{
			name: "valid event with details",
			row: &dbsqlc.AuditEvent{
				ID:           "event-1",
				Type:         "auth.success",
				TenantID:     "tenant-1",
				UserID:       "user-1",
				Subject:      "CN=alice",
				ResourceType: "subscription",
				ResourceID:   "sub-1",
				Action:       "create",
				Details:      json.RawMessage(`{"key":"value"}`),
				ClientIp:     "192.168.1.1",
				UserAgent:    "test-agent",
				Timestamp:    now,
			},
		},
		{
			name: "event with empty details",
			row: &dbsqlc.AuditEvent{
				ID:        "event-2",
				Type:      "auth.failure",
				Action:    "login",
				Details:   json.RawMessage(`{}`),
				Timestamp: now,
			},
		},
		{
			name: "event with null details",
			row: &dbsqlc.AuditEvent{
				ID:        "event-3",
				Type:      "auth.failure",
				Action:    "login",
				Details:   json.RawMessage(`null`),
				Timestamp: now,
			},
		},
		{
			name: "invalid details JSON",
			row: &dbsqlc.AuditEvent{
				ID:        "event-4",
				Type:      "auth.failure",
				Action:    "login",
				Details:   json.RawMessage(`{"invalid`),
				Timestamp: now,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := dbsqlcAuditEventToModel(tt.row)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.row.ID, got.ID)
			assert.Equal(t, AuditEventType(tt.row.Type), got.Type)
		})
	}
}

func TestDbsqlcAuditEventsToModels(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	t.Run("valid events", func(t *testing.T) {
		t.Parallel()
		rows := []dbsqlc.AuditEvent{
			{ID: "event-1", Type: "auth.success", Action: "login", Details: json.RawMessage(`{}`), Timestamp: now},
			{ID: "event-2", Type: "auth.failure", Action: "login", Details: json.RawMessage(`{}`), Timestamp: now},
		}

		events, err := dbsqlcAuditEventsToModels(rows)
		require.NoError(t, err)
		assert.Len(t, events, 2)
	})

	t.Run("invalid event stops iteration", func(t *testing.T) {
		t.Parallel()
		rows := []dbsqlc.AuditEvent{
			{ID: "event-1", Type: "auth.success", Action: "login", Details: json.RawMessage(`{"bad`), Timestamp: now},
		}

		_, err := dbsqlcAuditEventsToModels(rows)
		require.Error(t, err)
	})
}
