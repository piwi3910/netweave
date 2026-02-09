package certlifecycle_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/certlifecycle"
)

// mockKeycloakClient implements certlifecycle.KeycloakUserClient for testing.
type mockKeycloakClient struct {
	mu    sync.Mutex
	attrs map[string]map[string]string // userID -> attrName -> attrValue
	err   error
}

func newMockKeycloakClient() *mockKeycloakClient {
	return &mockKeycloakClient{
		attrs: make(map[string]map[string]string),
	}
}

func (m *mockKeycloakClient) SetUserAttribute(
	_ context.Context, userID, attributeName, attributeValue string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	if m.attrs[userID] == nil {
		m.attrs[userID] = make(map[string]string)
	}
	m.attrs[userID][attributeName] = attributeValue
	return nil
}

func (m *mockKeycloakClient) getAttr(userID, attrName string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.attrs[userID] == nil {
		return ""
	}
	return m.attrs[userID][attrName]
}

func (m *mockKeycloakClient) attrCount(userID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.attrs[userID])
}

func TestKeycloakSyncer_SyncIssuance(t *testing.T) {
	kcClient := newMockKeycloakClient()
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "serial-001",
		UserID:       "user-1",
		CommonName:   "test.example.com",
		Status:       certlifecycle.StatusActive,
		IssuedAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt:    time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	syncer.SyncIssuance(context.Background(), meta)

	assert.Equal(t, 5, kcClient.attrCount("user-1"))
	assert.Equal(t, "serial-001", kcClient.getAttr("user-1", "cert_serial"))
	assert.Equal(t, "test.example.com", kcClient.getAttr("user-1", "cert_cn"))
	assert.Equal(t, "active", kcClient.getAttr("user-1", "cert_status"))
	assert.Contains(t, kcClient.getAttr("user-1", "cert_issued_at"), "2026-01-01")
	assert.Contains(t, kcClient.getAttr("user-1", "cert_expires_at"), "2027-01-01")
}

func TestKeycloakSyncer_SyncIssuance_EmptyUserID(t *testing.T) {
	kcClient := newMockKeycloakClient()
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "serial-002",
		UserID:       "",
		CommonName:   "no-user.example.com",
		Status:       certlifecycle.StatusActive,
	}

	syncer.SyncIssuance(context.Background(), meta)

	// No attributes should be set when userID is empty.
	assert.Equal(t, 0, kcClient.attrCount(""))
}

func TestKeycloakSyncer_SyncIssuance_Error(t *testing.T) {
	kcClient := newMockKeycloakClient()
	kcClient.err = errors.New("keycloak unavailable")
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "serial-003",
		UserID:       "user-1",
		CommonName:   "err.example.com",
		Status:       certlifecycle.StatusActive,
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(365 * 24 * time.Hour),
	}

	// Should not panic; errors are logged as warnings.
	syncer.SyncIssuance(context.Background(), meta)
}

func TestKeycloakSyncer_SyncRevocation(t *testing.T) {
	kcClient := newMockKeycloakClient()
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	syncer.SyncRevocation(context.Background(), "user-1")

	assert.Equal(t, "revoked", kcClient.getAttr("user-1", "cert_status"))
}

func TestKeycloakSyncer_SyncRevocation_EmptyUserID(t *testing.T) {
	kcClient := newMockKeycloakClient()
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	syncer.SyncRevocation(context.Background(), "")

	assert.Equal(t, 0, kcClient.attrCount(""))
}

func TestKeycloakSyncer_SyncRevocation_Error(t *testing.T) {
	kcClient := newMockKeycloakClient()
	kcClient.err = errors.New("keycloak unavailable")
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	// Should not panic.
	syncer.SyncRevocation(context.Background(), "user-1")
}

func TestKeycloakSyncer_SyncRenewal(t *testing.T) {
	kcClient := newMockKeycloakClient()
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "renewed-serial",
		UserID:       "user-2",
		CommonName:   "renewed.example.com",
		Status:       certlifecycle.StatusActive,
		ExpiresAt:    time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	syncer.SyncRenewal(context.Background(), meta)

	assert.Equal(t, "renewed-serial", kcClient.getAttr("user-2", "cert_serial"))
	assert.Equal(t, "renewed.example.com", kcClient.getAttr("user-2", "cert_cn"))
	assert.Equal(t, "active", kcClient.getAttr("user-2", "cert_status"))
	assert.Contains(t, kcClient.getAttr("user-2", "cert_expires_at"), "2028-01-01")
	assert.NotEmpty(t, kcClient.getAttr("user-2", "cert_renewed_at"))
}

func TestKeycloakSyncer_SyncRenewal_EmptyUserID(t *testing.T) {
	kcClient := newMockKeycloakClient()
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "renewed-serial",
		UserID:       "",
	}

	syncer.SyncRenewal(context.Background(), meta)
	assert.Equal(t, 0, kcClient.attrCount(""))
}

func TestKeycloakSyncer_SyncExpiringSoon(t *testing.T) {
	kcClient := newMockKeycloakClient()
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "expiring-serial",
		UserID:       "user-3",
	}

	syncer.SyncExpiringSoon(context.Background(), meta)

	assert.Equal(t, "expiring_soon", kcClient.getAttr("user-3", "cert_status"))
}

func TestKeycloakSyncer_SyncExpiringSoon_EmptyUserID(t *testing.T) {
	kcClient := newMockKeycloakClient()
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "expiring-serial",
		UserID:       "",
	}

	syncer.SyncExpiringSoon(context.Background(), meta)
	assert.Equal(t, 0, kcClient.attrCount(""))
}

func TestKeycloakSyncer_SyncExpiringSoon_Error(t *testing.T) {
	kcClient := newMockKeycloakClient()
	kcClient.err = errors.New("keycloak down")
	logger := zaptest.NewLogger(t)
	syncer := certlifecycle.NewKeycloakSyncer(kcClient, logger)

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "expiring-serial",
		UserID:       "user-3",
	}

	// Should not panic.
	syncer.SyncExpiringSoon(context.Background(), meta)
}

func TestManager_RecordIssuance_WithKeycloak(t *testing.T) {
	store := newMockStore()
	kcClient := newMockKeycloakClient()
	logger := zaptest.NewLogger(t)

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:          store,
		KeycloakClient: kcClient,
		Logger:         logger,
	})
	require.NoError(t, err)

	cert := &certlifecycle.CertificateMetadata{
		SerialNumber: "kc-serial-001",
		UserID:       "user-kc-1",
		TenantID:     "tenant-1",
		CommonName:   "kc.example.com",
		Status:       certlifecycle.StatusActive,
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(365 * 24 * time.Hour),
	}

	err = mgr.RecordIssuance(context.Background(), cert)
	require.NoError(t, err)

	// Verify Keycloak attributes were set.
	assert.Equal(t, "kc-serial-001", kcClient.getAttr("user-kc-1", "cert_serial"))
	assert.Equal(t, "kc.example.com", kcClient.getAttr("user-kc-1", "cert_cn"))
	assert.Equal(t, "active", kcClient.getAttr("user-kc-1", "cert_status"))
}

func TestManager_RecordRevocation_WithKeycloak(t *testing.T) {
	store := newMockStore()
	kcClient := newMockKeycloakClient()
	logger := zaptest.NewLogger(t)

	// Pre-populate cert.
	store.certs["kc-revoke-serial"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "kc-revoke-serial",
		UserID:       "user-kc-2",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
	}

	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:          store,
		KeycloakClient: kcClient,
		Logger:         logger,
	})
	require.NoError(t, err)

	err = mgr.RecordRevocation(context.Background(), "kc-revoke-serial")
	require.NoError(t, err)

	// Verify Keycloak status was updated.
	assert.Equal(t, "revoked", kcClient.getAttr("user-kc-2", "cert_status"))
}
