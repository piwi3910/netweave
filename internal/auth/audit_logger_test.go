package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestNewAuditLogger(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	tests := []struct {
		name    string
		store   auth.Store
		logger  *zap.Logger
		wantErr bool
		errType error
	}{
		{
			name:    "valid logger and store",
			store:   store,
			logger:  zap.NewNop(),
			wantErr: false,
		},
		{
			name:    "nil logger returns error",
			store:   store,
			logger:  nil,
			wantErr: true,
			errType: auth.ErrNilLogger,
		},
		{
			name:    "nil store returns error",
			store:   nil,
			logger:  zap.NewNop(),
			wantErr: true,
			errType: auth.ErrNilAuditStore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			al, err := auth.NewAuditLogger(tt.store, tt.logger)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, al)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, al)
			}
		})
	}
}

func TestNewAuditLogger_WithStore(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	al, err := auth.NewAuditLogger(store, zaptest.NewLogger(t))
	require.NoError(t, err)
	assert.NotNil(t, al)
}

func TestAuditLogger_LogResourceOperation(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	logger := zaptest.NewLogger(t)

	tests := []struct {
		name         string
		user         *auth.AuthenticatedUser
		success      bool
		details      map[string]string
		useStore     bool
		resourceType string
		resourceID   string
	}{
		{
			name: "success with user and store",
			user: &auth.AuthenticatedUser{
				UserID:   "user-1",
				TenantID: "tenant-1",
				Subject:  "CN=alice,O=ACME",
			},
			success:      true,
			details:      map[string]string{"key": "value"},
			useStore:     true,
			resourceType: "subscription",
			resourceID:   "sub-1",
		},
		{
			name: "failure with nil details",
			user: &auth.AuthenticatedUser{
				UserID:   "user-2",
				TenantID: "tenant-1",
			},
			success:      false,
			details:      nil,
			useStore:     true,
			resourceType: "resource",
			resourceID:   "res-1",
		},
		{
			name:         "nil user with nil store",
			user:         nil,
			success:      true,
			details:      nil,
			useStore:     false,
			resourceType: "resource",
			resourceID:   "res-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store is always required by NewAuditLogger; useStore is retained
			// only for readability of the table. Passing nil would now error.
			_ = tt.useStore
			al, err := auth.NewAuditLogger(store, logger)
			require.NoError(t, err)

			ctx := context.Background()
			// Should not panic
			al.LogResourceOperation(ctx, auth.AuditEventResourceCreated, tt.resourceType, tt.resourceID, tt.user, tt.success, tt.details)
		})
	}
}

// newTestAuditLogger is a convenience helper that sets up a miniredis-backed
// audit store and returns a ready-to-use AuditLogger for tests which do not
// need to inspect the persisted events.
func newTestAuditLogger(t *testing.T) (*auth.AuditLogger, func()) {
	t.Helper()
	store := setupTestRedis(t)
	al, err := auth.NewAuditLogger(store, zaptest.NewLogger(t))
	require.NoError(t, err)
	return al, func() { _ = store.Close() }
}

func TestAuditLogger_LogSubscriptionOperation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	tests := []struct {
		name     string
		user     *auth.AuthenticatedUser
		details  map[string]string
		callback string
	}{
		{
			name: "with user and details",
			user: &auth.AuthenticatedUser{
				UserID:   "user-1",
				TenantID: "tenant-1",
			},
			details:  map[string]string{"filter": "pool-1"},
			callback: "https://smo.example.com/notify",
		},
		{
			name:     "nil user and nil details",
			user:     nil,
			details:  nil,
			callback: "https://example.com/cb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			al, err := auth.NewAuditLogger(store, logger)
			require.NoError(t, err)

			ctx := context.Background()
			al.LogSubscriptionOperation(ctx, auth.AuditEventSubscriptionCreated, "sub-1", tt.callback, tt.user, tt.details)
		})
	}
}

func TestAuditLogger_LogConfigurationChange(t *testing.T) {
	al, cleanup := newTestAuditLogger(t)
	defer cleanup()

	user := &auth.AuthenticatedUser{
		UserID:   "admin-1",
		TenantID: "platform",
	}

	ctx := context.Background()
	al.LogConfigurationChange(ctx, auth.AuditEventTLSConfigChanged, "tls", user, "TLS1.2", "TLS1.3")
}

func TestAuditLogger_LogAdminOperation(t *testing.T) {
	al, cleanup := newTestAuditLogger(t)
	defer cleanup()

	tests := []struct {
		name    string
		user    *auth.AuthenticatedUser
		details map[string]string
	}{
		{
			name: "with details",
			user: &auth.AuthenticatedUser{
				UserID:   "admin-1",
				TenantID: "platform",
			},
			details: map[string]string{"action": "rotate_key"},
		},
		{
			name:    "nil details",
			user:    nil,
			details: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			al.LogAdminOperation(ctx, auth.AuditEventTokenRotated, "token_rotation", tt.user, tt.details)
		})
	}
}

func TestAuditLogger_LogWebhookFailure(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	logger := zaptest.NewLogger(t)
	al, err := auth.NewAuditLogger(store, logger)
	require.NoError(t, err)

	ctx := context.Background()
	al.LogWebhookFailure(ctx, "sub-1", "https://smo.example.com/notify", "connection refused", 503)
}

func TestAuditLogger_LogSignatureVerificationFailure(t *testing.T) {
	al, cleanup := newTestAuditLogger(t)
	defer cleanup()

	ctx := context.Background()
	al.LogSignatureVerificationFailure(ctx, "sub-1", "192.168.1.100", "invalid HMAC signature")
}

func TestAuditLogger_LogTenantStatusChange(t *testing.T) {
	al, cleanup := newTestAuditLogger(t)
	defer cleanup()

	user := &auth.AuthenticatedUser{
		UserID:   "admin-1",
		TenantID: "platform",
	}

	tests := []struct {
		name      string
		oldStatus auth.TenantStatus
		newStatus auth.TenantStatus
	}{
		{
			name:      "active to suspended",
			oldStatus: auth.TenantStatusActive,
			newStatus: auth.TenantStatusSuspended,
		},
		{
			name:      "suspended to active",
			oldStatus: auth.TenantStatusSuspended,
			newStatus: auth.TenantStatusActive,
		},
		{
			name:      "active to pending deletion",
			oldStatus: auth.TenantStatusActive,
			newStatus: auth.TenantStatusPendingDeletion,
		},
		{
			name:      "unknown status defaults to updated event",
			oldStatus: auth.TenantStatusActive,
			newStatus: auth.TenantStatus("unknown"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			al.LogTenantStatusChange(ctx, "tenant-1", tt.oldStatus, tt.newStatus, user, "admin action")
		})
	}
}

func TestAuditLogger_LogUserStatusChange(t *testing.T) {
	al, cleanup := newTestAuditLogger(t)
	defer cleanup()

	actor := &auth.AuthenticatedUser{
		UserID:   "admin-1",
		TenantID: "tenant-1",
	}

	tests := []struct {
		name    string
		enabled bool
	}{
		{
			name:    "disable user",
			enabled: false,
		},
		{
			name:    "enable user",
			enabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			al.LogUserStatusChange(ctx, "user-1", tt.enabled, actor, "admin action")
		})
	}
}

func TestAuditLogger_LogQuotaUpdate(t *testing.T) {
	al, cleanup := newTestAuditLogger(t)
	defer cleanup()

	user := &auth.AuthenticatedUser{
		UserID:   "admin-1",
		TenantID: "platform",
	}

	ctx := context.Background()
	al.LogQuotaUpdate(ctx, "tenant-1", user, "subscriptions", 100, 200)
}

func TestAuditLogger_LogBulkOperation(t *testing.T) {
	al, cleanup := newTestAuditLogger(t)
	defer cleanup()

	tests := []struct {
		name    string
		details map[string]string
	}{
		{
			name:    "with details",
			details: map[string]string{"filter": "status=inactive"},
		},
		{
			name:    "nil details",
			details: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &auth.AuthenticatedUser{
				UserID:   "admin-1",
				TenantID: "platform",
			}

			ctx := context.Background()
			al.LogBulkOperation(ctx, "delete", "users", 15, user, tt.details)
		})
	}
}

func TestAuditLogger_ContextValues(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	logger := zaptest.NewLogger(t)
	al, err := auth.NewAuditLogger(store, logger)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = auth.WithClientIP(ctx, "10.0.0.1")
	ctx = auth.WithUserAgent(ctx, "test-agent/1.0")

	user := &auth.AuthenticatedUser{
		UserID:   "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=alice,O=ACME",
	}

	// Log with context values and verify it doesn't panic
	al.LogResourceOperation(ctx, auth.AuditEventResourceCreated, "resource", "res-1", user, true, nil)
}

func TestContextHelpers(t *testing.T) {
	t.Run("ClientIPFromContext with value", func(t *testing.T) {
		ctx := auth.WithClientIP(context.Background(), "192.168.1.1")
		ip := auth.ClientIPFromContext(ctx)
		assert.Equal(t, "192.168.1.1", ip)
	})

	t.Run("ClientIPFromContext without value", func(t *testing.T) {
		ip := auth.ClientIPFromContext(context.Background())
		assert.Empty(t, ip)
	})

	t.Run("UserAgentFromContext with value", func(t *testing.T) {
		ctx := auth.WithUserAgent(context.Background(), "Mozilla/5.0")
		ua := auth.UserAgentFromContext(ctx)
		assert.Equal(t, "Mozilla/5.0", ua)
	})

	t.Run("UserAgentFromContext without value", func(t *testing.T) {
		ua := auth.UserAgentFromContext(context.Background())
		assert.Empty(t, ua)
	})
}

func TestAuditLogger_StoreError(t *testing.T) {
	// Use a store that will fail - create and close it immediately
	store := setupTestRedis(t)
	_ = store.Close() // Close immediately so LogEvent fails

	logger := zaptest.NewLogger(t)
	al, err := auth.NewAuditLogger(store, logger)
	require.NoError(t, err)

	ctx := context.Background()
	// Should not panic even when store.LogEvent fails
	al.LogWebhookFailure(ctx, "sub-1", "https://example.com/cb", "error", 500)
}

func TestErrNilLogger(t *testing.T) {
	assert.Equal(t, "logger cannot be nil", auth.ErrNilLogger.Error())
	assert.True(t, errors.Is(auth.ErrNilLogger, auth.ErrNilLogger))
}
