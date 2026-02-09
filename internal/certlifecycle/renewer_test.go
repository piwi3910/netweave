package certlifecycle_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/certlifecycle"
	"github.com/piwi3910/netweave/internal/vault"
)

// mockVaultPKI implements certlifecycle.VaultPKI for testing.
type mockVaultPKI struct {
	issueFn  func(ctx context.Context, roleName string, req *vault.CertificateRequest) (*vault.Certificate, error)
	revokeFn func(ctx context.Context, serialNumber string) error
}

func (m *mockVaultPKI) IssueCertificate(
	ctx context.Context, roleName string, req *vault.CertificateRequest,
) (*vault.Certificate, error) {
	if m.issueFn != nil {
		return m.issueFn(ctx, roleName, req)
	}
	return &vault.Certificate{
		SerialNumber: "new-serial-001",
		Expiration:   time.Now().Add(365 * 24 * time.Hour),
	}, nil
}

func (m *mockVaultPKI) RevokeCertificateBySerial(
	ctx context.Context, serialNumber string,
) error {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, serialNumber)
	}
	return nil
}

func TestRenewer_RenewCertificate_Success(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)
	vaultPKI := &mockVaultPKI{}

	// Add existing cert to store.
	oldMeta := &certlifecycle.CertificateMetadata{
		SerialNumber: "old-serial-001",
		UserID:       "user-1",
		TenantID:     "tenant-1",
		CommonName:   "test.example.com",
		RoleName:     "web-server",
		Status:       certlifecycle.StatusExpiringSoon,
		IssuedAt:     time.Now().Add(-30 * 24 * time.Hour),
		ExpiresAt:    time.Now().Add(5 * 24 * time.Hour),
	}
	store.certs["old-serial-001"] = oldMeta

	renewer := certlifecycle.NewRenewer(&certlifecycle.RenewerConfig{
		Store:       store,
		VaultClient: vaultPKI,
		Logger:      logger,
	})

	err := renewer.RenewCertificate(context.Background(), oldMeta)
	require.NoError(t, err)

	// New cert should be in the store.
	newCert, getErr := store.Get(context.Background(), "new-serial-001")
	require.NoError(t, getErr)
	assert.Equal(t, "new-serial-001", newCert.SerialNumber)
	assert.Equal(t, "user-1", newCert.UserID)
	assert.Equal(t, "tenant-1", newCert.TenantID)
	assert.Equal(t, "test.example.com", newCert.CommonName)
	assert.Equal(t, "web-server", newCert.RoleName)
	assert.Equal(t, certlifecycle.StatusActive, newCert.Status)
	assert.Equal(t, "old-serial-001", newCert.RenewedFrom)
	assert.Equal(t, 1, newCert.RenewalCount)
}

func TestRenewer_RenewCertificate_VaultFailure(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)
	vaultErr := errors.New("vault unavailable")
	vaultPKI := &mockVaultPKI{
		issueFn: func(
			_ context.Context, _ string, _ *vault.CertificateRequest,
		) (*vault.Certificate, error) {
			return nil, vaultErr
		},
	}

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "failing-serial",
		TenantID:     "tenant-1",
		CommonName:   "test.example.com",
		RoleName:     "web-server",
		Status:       certlifecycle.StatusExpiringSoon,
		RetryCount:   0,
	}
	store.certs["failing-serial"] = meta

	renewer := certlifecycle.NewRenewer(&certlifecycle.RenewerConfig{
		MaxRetries:  3,
		Store:       store,
		VaultClient: vaultPKI,
		Logger:      logger,
	})

	err := renewer.RenewCertificate(context.Background(), meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "renewal failed for failing-serial")
}

func TestRenewer_RenewCertificate_MaxRetriesExceeded(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)
	vaultPKI := &mockVaultPKI{
		issueFn: func(
			_ context.Context, _ string, _ *vault.CertificateRequest,
		) (*vault.Certificate, error) {
			return nil, errors.New("vault error")
		},
	}

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "max-retry-serial",
		TenantID:     "tenant-1",
		CommonName:   "test.example.com",
		RoleName:     "web-server",
		Status:       certlifecycle.StatusExpiringSoon,
		RetryCount:   2, // Already at retry 2, next attempt = 3 (maxRetries).
	}
	store.certs["max-retry-serial"] = meta

	renewer := certlifecycle.NewRenewer(&certlifecycle.RenewerConfig{
		MaxRetries:  3,
		Store:       store,
		VaultClient: vaultPKI,
		Logger:      logger,
	})

	err := renewer.RenewCertificate(context.Background(), meta)
	require.Error(t, err)
}

func TestRenewer_DefaultConfig(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)
	vaultPKI := &mockVaultPKI{}

	renewer := certlifecycle.NewRenewer(&certlifecycle.RenewerConfig{
		Store:       store,
		VaultClient: vaultPKI,
		Logger:      logger,
	})

	require.NotNil(t, renewer)
}

func TestRenewer_RenewCertificate_WithNotifier(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)
	vaultPKI := &mockVaultPKI{}

	// Setup webhook server.
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	notifier, notifierErr := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
		WebhookURL: webhookServer.URL,
		Logger:     logger,
	})
	require.NoError(t, notifierErr)

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "notify-serial",
		TenantID:     "tenant-1",
		CommonName:   "test.example.com",
		RoleName:     "web-server",
		Status:       certlifecycle.StatusExpiringSoon,
		IssuedAt:     time.Now().Add(-30 * 24 * time.Hour),
		ExpiresAt:    time.Now().Add(5 * 24 * time.Hour),
	}
	store.certs["notify-serial"] = meta

	renewer := certlifecycle.NewRenewer(&certlifecycle.RenewerConfig{
		Store:       store,
		VaultClient: vaultPKI,
		Notifier:    notifier,
		Logger:      logger,
	})

	err := renewer.RenewCertificate(context.Background(), meta)
	require.NoError(t, err)
}

func TestRenewer_RenewCertificate_FailureWithNotifier(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)
	vaultPKI := &mockVaultPKI{
		issueFn: func(
			_ context.Context, _ string, _ *vault.CertificateRequest,
		) (*vault.Certificate, error) {
			return nil, errors.New("vault error")
		},
	}

	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	notifier, notifierErr := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
		WebhookURL: webhookServer.URL,
		Logger:     logger,
	})
	require.NoError(t, notifierErr)

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "fail-notify-serial",
		TenantID:     "tenant-1",
		CommonName:   "test.example.com",
		RoleName:     "web-server",
		Status:       certlifecycle.StatusExpiringSoon,
		RetryCount:   2, // Will exceed max retries (3).
	}
	store.certs["fail-notify-serial"] = meta

	renewer := certlifecycle.NewRenewer(&certlifecycle.RenewerConfig{
		MaxRetries:  3,
		Store:       store,
		VaultClient: vaultPKI,
		Notifier:    notifier,
		Logger:      logger,
	})

	err := renewer.RenewCertificate(context.Background(), meta)
	require.Error(t, err)
}

func TestRenewer_RenewCertificate_PreservesMetadata(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)
	vaultPKI := &mockVaultPKI{
		issueFn: func(
			_ context.Context, _ string, _ *vault.CertificateRequest,
		) (*vault.Certificate, error) {
			return &vault.Certificate{
				SerialNumber: "renewed-serial",
				Expiration:   time.Now().Add(365 * 24 * time.Hour),
			}, nil
		},
	}

	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "original-serial",
		UserID:       "user-42",
		TenantID:     "tenant-99",
		CommonName:   "deep.example.com",
		RoleName:     "api-server",
		Status:       certlifecycle.StatusExpiringSoon,
		RenewalCount: 3,
		IssuedAt:     time.Now().Add(-60 * 24 * time.Hour),
		ExpiresAt:    time.Now().Add(2 * 24 * time.Hour),
	}
	store.certs["original-serial"] = meta

	renewer := certlifecycle.NewRenewer(&certlifecycle.RenewerConfig{
		Store:       store,
		VaultClient: vaultPKI,
		Logger:      logger,
	})

	err := renewer.RenewCertificate(context.Background(), meta)
	require.NoError(t, err)

	newCert, getErr := store.Get(context.Background(), "renewed-serial")
	require.NoError(t, getErr)
	assert.Equal(t, "user-42", newCert.UserID)
	assert.Equal(t, "tenant-99", newCert.TenantID)
	assert.Equal(t, "deep.example.com", newCert.CommonName)
	assert.Equal(t, "api-server", newCert.RoleName)
	assert.Equal(t, 4, newCert.RenewalCount)
	assert.Equal(t, "original-serial", newCert.RenewedFrom)
}
