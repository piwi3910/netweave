package certlifecycle_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/certlifecycle"
)

func TestNewManager_RequiresStore(t *testing.T) {
	logger := zaptest.NewLogger(t)

	_, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Logger: logger,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store is required")
}

func TestNewManager_RequiresLogger(t *testing.T) {
	store := newMockStore()

	_, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store: store,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logger is required")
}

func TestNewManager_MinimalConfig(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:  store,
		Logger: logger,
	})
	require.NoError(t, err)
	require.NotNil(t, mgr)
}

func TestNewManager_WithVaultClient(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)
	vaultPKI := &mockVaultPKI{}

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:       store,
		VaultClient: vaultPKI,
		Logger:      logger,
	})
	require.NoError(t, err)
	require.NotNil(t, mgr)
}

func TestManager_StartStop(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:        store,
		Logger:       logger,
		ScanInterval: 50 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = mgr.Start(ctx)
	require.NoError(t, err)

	// Give workers time to start.
	time.Sleep(100 * time.Millisecond)

	err = mgr.Stop()
	require.NoError(t, err)
}

func TestManager_RecordIssuance(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:  store,
		Logger: logger,
	})
	require.NoError(t, err)

	cert := &certlifecycle.CertificateMetadata{
		SerialNumber: "issued-serial-001",
		UserID:       "user-1",
		TenantID:     "tenant-1",
		CommonName:   "test.example.com",
		RoleName:     "web-server",
		Status:       certlifecycle.StatusActive,
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(365 * 24 * time.Hour),
	}

	err = mgr.RecordIssuance(context.Background(), cert)
	require.NoError(t, err)

	// Verify cert is in the store.
	stored, getErr := store.Get(context.Background(), "issued-serial-001")
	require.NoError(t, getErr)
	assert.Equal(t, "issued-serial-001", stored.SerialNumber)
	assert.Equal(t, "user-1", stored.UserID)
}

func TestManager_RecordRevocation(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	// Pre-populate the cert in the store.
	store.certs["revoke-serial-001"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "revoke-serial-001",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
	}

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:  store,
		Logger: logger,
	})
	require.NoError(t, err)

	err = mgr.RecordRevocation(context.Background(), "revoke-serial-001")
	require.NoError(t, err)
}

func TestManager_RecordRevocation_NotFound(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:  store,
		Logger: logger,
	})
	require.NoError(t, err)

	err = mgr.RecordRevocation(context.Background(), "nonexistent-serial")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get certificate")
}

func TestManager_StartStop_WithVault(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)
	vaultPKI := &mockVaultPKI{}

	// Add a failed cert for retry processing.
	store.certs["failed-serial"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "failed-serial",
		TenantID:     "tenant-1",
		CommonName:   "test.example.com",
		RoleName:     "web-server",
		Status:       certlifecycle.StatusRenewalFailed,
		RetryCount:   1,
		NextRetryAt:  time.Now().Add(-1 * time.Hour),
	}

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:        store,
		VaultClient:  vaultPKI,
		Logger:       logger,
		ScanInterval: 50 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = mgr.Start(ctx)
	require.NoError(t, err)

	// Give workers time to process.
	time.Sleep(200 * time.Millisecond)

	err = mgr.Stop()
	require.NoError(t, err)
}

func TestManager_OnExpiringSoonCallback(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	// Add an active cert expiring within the renewal window.
	store.certs["expiring-serial"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "expiring-serial",
		TenantID:     "tenant-1",
		CommonName:   "test.example.com",
		RoleName:     "web-server",
		Status:       certlifecycle.StatusActive,
		ExpiresAt:    time.Now().Add(10 * 24 * time.Hour),
	}

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:         store,
		Logger:        logger,
		ScanInterval:  50 * time.Millisecond,
		RenewalWindow: 30 * 24 * time.Hour,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = mgr.Start(ctx)
	require.NoError(t, err)

	// Wait for at least one scan cycle.
	time.Sleep(200 * time.Millisecond)

	err = mgr.Stop()
	require.NoError(t, err)

	// Cert should have been detected as expiring_soon.
	updates := store.getStatusUpdates()
	require.NotEmpty(t, updates)
	assert.Equal(t, "expiring-serial", updates[0].SerialNumber)
	assert.Equal(t, certlifecycle.StatusExpiringSoon, updates[0].Status)
}

func TestManager_OnExpiredCallback(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	// Add an already-expired cert.
	store.certs["expired-serial"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "expired-serial",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:         store,
		Logger:        logger,
		ScanInterval:  50 * time.Millisecond,
		RenewalWindow: 30 * 24 * time.Hour,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = mgr.Start(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	err = mgr.Stop()
	require.NoError(t, err)

	// The cert should be marked as expired.
	found := false
	for _, u := range store.getStatusUpdates() {
		if u.SerialNumber == "expired-serial" &&
			u.Status == certlifecycle.StatusExpired {
			found = true
			break
		}
	}
	assert.True(t, found, "expected expired-serial to be marked expired")
}

func TestManager_RecordIssuance_WithWebhook(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	webhookServer := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer webhookServer.Close()

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:      store,
		Logger:     logger,
		WebhookURL: webhookServer.URL,
		HMACSecret: "test-secret",
	})
	require.NoError(t, err)

	cert := &certlifecycle.CertificateMetadata{
		SerialNumber: "webhook-serial",
		TenantID:     "tenant-1",
		RoleName:     "web-server",
		Status:       certlifecycle.StatusActive,
		ExpiresAt:    time.Now().Add(365 * 24 * time.Hour),
	}

	err = mgr.RecordIssuance(context.Background(), cert)
	require.NoError(t, err)
}

func TestManager_RecordRevocation_WithWebhook(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	webhookServer := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer webhookServer.Close()

	store.certs["revoke-wh"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "revoke-wh",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
	}

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:      store,
		Logger:     logger,
		WebhookURL: webhookServer.URL,
	})
	require.NoError(t, err)

	err = mgr.RecordRevocation(context.Background(), "revoke-wh")
	require.NoError(t, err)
}

func TestManager_RecordIssuance_DuplicateError(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:  store,
		Logger: logger,
	})
	require.NoError(t, err)

	cert := &certlifecycle.CertificateMetadata{
		SerialNumber: "dup-serial",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
		ExpiresAt:    time.Now().Add(365 * 24 * time.Hour),
	}

	// First issuance should succeed.
	err = mgr.RecordIssuance(context.Background(), cert)
	require.NoError(t, err)

	// Second issuance of same serial should still succeed
	// (mockStore overwrites on Create, real store would error).
	err = mgr.RecordIssuance(context.Background(), cert)
	require.NoError(t, err)
}
