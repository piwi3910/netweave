package certmanager

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.NotNil(t, config)
	assert.Equal(t, "http://localhost:8200", config.VaultAddress)
	assert.Equal(t, "pki_int", config.VaultPKIPath)
	assert.Equal(t, "netweave-client", config.VaultRole)
	assert.Equal(t, time.Hour, config.MonitorInterval)
	assert.True(t, config.EnableAutoRenewal)
	assert.NotNil(t, config.RenewalPolicy)
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "missing VaultAddress",
			config: &Config{
				VaultPKIPath:    "pki_int",
				VaultRole:       "netweave-client",
				KeycloakBaseURL: "http://localhost:8090",
				KeycloakRealm:   "netweave",
				MonitorInterval: time.Hour,
				RenewalPolicy:   DefaultRenewalPolicy(),
			},
			wantErr: true,
			errMsg:  "VaultAddress is required",
		},
		{
			name: "missing VaultPKIPath",
			config: &Config{
				VaultAddress:    "http://localhost:8200",
				VaultRole:       "netweave-client",
				KeycloakBaseURL: "http://localhost:8090",
				KeycloakRealm:   "netweave",
				MonitorInterval: time.Hour,
				RenewalPolicy:   DefaultRenewalPolicy(),
			},
			wantErr: true,
			errMsg:  "VaultPKIPath is required",
		},
		{
			name: "missing VaultRole",
			config: &Config{
				VaultAddress:    "http://localhost:8200",
				VaultPKIPath:    "pki_int",
				KeycloakBaseURL: "http://localhost:8090",
				KeycloakRealm:   "netweave",
				MonitorInterval: time.Hour,
				RenewalPolicy:   DefaultRenewalPolicy(),
			},
			wantErr: true,
			errMsg:  "VaultRole is required",
		},
		{
			name: "missing KeycloakBaseURL",
			config: &Config{
				VaultAddress:    "http://localhost:8200",
				VaultPKIPath:    "pki_int",
				VaultRole:       "netweave-client",
				KeycloakRealm:   "netweave",
				MonitorInterval: time.Hour,
				RenewalPolicy:   DefaultRenewalPolicy(),
			},
			wantErr: true,
			errMsg:  "KeycloakBaseURL is required",
		},
		{
			name: "missing KeycloakRealm",
			config: &Config{
				VaultAddress:    "http://localhost:8200",
				VaultPKIPath:    "pki_int",
				VaultRole:       "netweave-client",
				KeycloakBaseURL: "http://localhost:8090",
				MonitorInterval: time.Hour,
				RenewalPolicy:   DefaultRenewalPolicy(),
			},
			wantErr: true,
			errMsg:  "KeycloakRealm is required",
		},
		{
			name: "invalid MonitorInterval",
			config: &Config{
				VaultAddress:    "http://localhost:8200",
				VaultPKIPath:    "pki_int",
				VaultRole:       "netweave-client",
				KeycloakBaseURL: "http://localhost:8090",
				KeycloakRealm:   "netweave",
				MonitorInterval: 0,
				RenewalPolicy:   DefaultRenewalPolicy(),
			},
			wantErr: true,
			errMsg:  "MonitorInterval must be positive",
		},
		{
			name: "nil RenewalPolicy",
			config: &Config{
				VaultAddress:    "http://localhost:8200",
				VaultPKIPath:    "pki_int",
				VaultRole:       "netweave-client",
				KeycloakBaseURL: "http://localhost:8090",
				KeycloakRealm:   "netweave",
				MonitorInterval: time.Hour,
				RenewalPolicy:   nil,
			},
			wantErr: true,
			errMsg:  "RenewalPolicy cannot be nil",
		},
		{
			name: "invalid RenewalWindow",
			config: &Config{
				VaultAddress:    "http://localhost:8200",
				VaultPKIPath:    "pki_int",
				VaultRole:       "netweave-client",
				KeycloakBaseURL: "http://localhost:8090",
				KeycloakRealm:   "netweave",
				MonitorInterval: time.Hour,
				RenewalPolicy: &RenewalPolicy{
					RenewalWindow: 0,
					MaxRetries:    3,
					RetryInterval: time.Hour,
				},
			},
			wantErr: true,
			errMsg:  "RenewalPolicy.RenewalWindow must be positive",
		},
		{
			name: "negative MaxRetries",
			config: &Config{
				VaultAddress:    "http://localhost:8200",
				VaultPKIPath:    "pki_int",
				VaultRole:       "netweave-client",
				KeycloakBaseURL: "http://localhost:8090",
				KeycloakRealm:   "netweave",
				MonitorInterval: time.Hour,
				RenewalPolicy: &RenewalPolicy{
					RenewalWindow: 30 * 24 * time.Hour,
					MaxRetries:    -1,
					RetryInterval: time.Hour,
				},
			},
			wantErr: true,
			errMsg:  "RenewalPolicy.MaxRetries cannot be negative",
		},
		{
			name: "invalid RetryInterval",
			config: &Config{
				VaultAddress:    "http://localhost:8200",
				VaultPKIPath:    "pki_int",
				VaultRole:       "netweave-client",
				KeycloakBaseURL: "http://localhost:8090",
				KeycloakRealm:   "netweave",
				MonitorInterval: time.Hour,
				RenewalPolicy: &RenewalPolicy{
					RenewalWindow: 30 * 24 * time.Hour,
					MaxRetries:    3,
					RetryInterval: 0,
				},
			},
			wantErr: true,
			errMsg:  "RenewalPolicy.RetryInterval must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDefaultRenewalPolicy(t *testing.T) {
	policy := DefaultRenewalPolicy()
	assert.NotNil(t, policy)
	assert.Equal(t, 30*24*time.Hour, policy.RenewalWindow)
	assert.Equal(t, 3, policy.MaxRetries)
	assert.Equal(t, time.Hour, policy.RetryInterval)
	// Note: Notification flags removed - see issue #298 for future implementation
}

func TestCertificateStatus(t *testing.T) {
	// Verify status constants
	assert.Equal(t, CertificateStatus("active"), CertStatusActive)
	assert.Equal(t, CertificateStatus("expiring_soon"), CertStatusExpiringSoon)
	assert.Equal(t, CertificateStatus("expired"), CertStatusExpired)
	assert.Equal(t, CertificateStatus("revoked"), CertStatusRevoked)
	assert.Equal(t, CertificateStatus("renewal_pending"), CertStatusRenewalPending)
	assert.Equal(t, CertificateStatus("renewal_failed"), CertStatusRenewalFailed)
}

func TestNewService_InvalidConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test with invalid config
	config := &Config{
		VaultAddress: "", // Invalid: empty address
	}

	service, err := NewService(config, logger)
	require.Error(t, err)
	assert.Nil(t, service)
	assert.Contains(t, err.Error(), "invalid config")
}

func TestCertificateOperations_Validation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.VaultToken = "test-token"
	config.KeycloakClientID = "test-client"
	config.KeycloakClientSecret = "test-secret"

	// Note: This will fail to create clients (no real Vault/Keycloak)
	// but we can test validation logic with a mock service
	mockService := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	ctx := context.Background()

	t.Run("IssueCertificate validation", func(t *testing.T) {
		tests := []struct {
			name    string
			req     *CertificateRequest
			wantErr bool
			errMsg  string
		}{
			{
				name:    "nil request",
				req:     nil,
				wantErr: true,
				errMsg:  "request cannot be nil",
			},
			{
				name: "missing UserID",
				req: &CertificateRequest{
					TenantID:   "tenant-1",
					CommonName: "test.example.com",
				},
				wantErr: true,
				errMsg:  "user_id is required",
			},
			{
				name: "missing TenantID",
				req: &CertificateRequest{
					UserID:     "user-1",
					CommonName: "test.example.com",
				},
				wantErr: true,
				errMsg:  "tenant_id is required",
			},
			{
				name: "missing CommonName",
				req: &CertificateRequest{
					UserID:   "user-1",
					TenantID: "tenant-1",
				},
				wantErr: true,
				errMsg:  "common_name is required",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := mockService.IssueCertificate(ctx, tt.req)
				if tt.wantErr {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			})
		}
	})

	t.Run("GetCertificate validation", func(t *testing.T) {
		_, err := mockService.GetCertificate(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "serial_number is required")
	})

	t.Run("RevokeCertificate validation", func(t *testing.T) {
		err := mockService.RevokeCertificate(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "serial_number is required")
	})
}

func TestGetMonitoringReport(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	mockService := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	// Add test certificates
	now := time.Now()
	mockService.certificates["cert-1"] = &Certificate{
		SerialNumber: "cert-1",
		Status:       CertStatusActive,
		ExpiresAt:    now.Add(60 * 24 * time.Hour), // 60 days
	}
	mockService.certificates["cert-2"] = &Certificate{
		SerialNumber: "cert-2",
		Status:       CertStatusExpiringSoon,
		ExpiresAt:    now.Add(15 * 24 * time.Hour), // 15 days
	}
	mockService.certificates["cert-3"] = &Certificate{
		SerialNumber: "cert-3",
		Status:       CertStatusExpired,
		ExpiresAt:    now.Add(-5 * 24 * time.Hour), // Expired 5 days ago
	}
	mockService.certificates["cert-4"] = &Certificate{
		SerialNumber: "cert-4",
		Status:       CertStatusRevoked,
		ExpiresAt:    now.Add(30 * 24 * time.Hour),
	}

	ctx := context.Background()
	report, err := mockService.GetMonitoringReport(ctx)
	require.NoError(t, err)
	assert.NotNil(t, report)

	assert.Equal(t, 4, report.TotalCertificates)
	assert.Equal(t, 1, report.ActiveCertificates)
	assert.Equal(t, 1, report.ExpiringSoon)
	assert.Equal(t, 1, report.Expired)
	assert.Equal(t, 1, report.Revoked)
	assert.False(t, report.GeneratedAt.IsZero())
}
