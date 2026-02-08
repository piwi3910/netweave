package certmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/keycloak"
	"github.com/piwi3910/netweave/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// setupVaultServer creates a httptest server that simulates Vault PKI API.
func setupVaultServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for pattern, handler := range handlers {
			if r.URL.Path == pattern {
				handler(w, r)
				return
			}
		}
		// Default: return 404
		w.WriteHeader(http.StatusNotFound)
	}))
}

// setupKeycloakServer creates a httptest server that simulates Keycloak Admin API.
func setupKeycloakServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for pattern, handler := range handlers {
			if r.URL.Path == pattern {
				handler(w, r)
				return
			}
		}
		// Default: return 404
		w.WriteHeader(http.StatusNotFound)
	}))
}

// newTestServiceWithServers creates a Service with real vault and keycloak clients backed by test servers.
func newTestServiceWithServers(t *testing.T, vaultServer, kcServer *httptest.Server) *Service {
	t.Helper()
	logger := zaptest.NewLogger(t)

	vaultCfg := &vault.Config{
		Address:    vaultServer.URL,
		Token:      "test-token",
		PKIPath:    "pki_int",
		HTTPClient: vaultServer.Client(),
	}
	vaultClient, err := vault.NewClient(vaultCfg)
	require.NoError(t, err)

	kcCfg := &keycloak.Config{
		BaseURL:       kcServer.URL,
		Realm:         "netweave",
		ClientID:      keycloak.AdminCLIClientID,
		AdminUsername: "admin",
		AdminPassword: "admin",
		HTTPClient:    kcServer.Client(),
	}
	kcClient, err := keycloak.NewClient(kcCfg)
	require.NoError(t, err)

	config := DefaultConfig()
	config.VaultAddress = vaultServer.URL
	config.VaultToken = "test-token"
	config.KeycloakBaseURL = kcServer.URL
	config.KeycloakRealm = "netweave"
	config.KeycloakClientID = keycloak.AdminCLIClientID
	config.KeycloakAdminUsername = "admin"
	config.KeycloakAdminPassword = "admin"

	return &Service{
		config:         config,
		vaultClient:    vaultClient,
		keycloakClient: kcClient,
		logger:         logger,
		certificates:   make(map[string]*Certificate),
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
	}
}

func TestIssueCertificate_Success(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/issue/netweave-client": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"certificate":      "-----BEGIN CERTIFICATE-----\nMIIBtest\n-----END CERTIFICATE-----",
					"private_key":      "-----BEGIN RSA PRIVATE KEY-----\nMIIBtestkey\n-----END RSA PRIVATE KEY-----",
					"private_key_type": "rsa",
					"serial_number":    "aa:bb:cc:dd:ee:ff",
					"issuing_ca":       "-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----",
					"ca_chain":         []string{"ca1", "ca2"},
					"expiration":       float64(time.Now().Add(365 * 24 * time.Hour).Unix()),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/master/protocol/openid-connect/token": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"access_token": "test-admin-token",
				"expires_in":   300,
				"token_type":   "Bearer",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/admin/realms/netweave/users/user-123": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				resp := map[string]interface{}{
					"id":       "user-123",
					"username": "testuser",
					"enabled":  true,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		},
	}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	req := &CertificateRequest{
		UserID:     "user-123",
		TenantID:   "tenant-1",
		CommonName: "test.example.com",
		AltNames:   []string{"alt1.example.com"},
		IPSANs:     []string{"10.0.0.1"},
	}

	cert, err := svc.IssueCertificate(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, cert)

	assert.Equal(t, "aa:bb:cc:dd:ee:ff", cert.SerialNumber)
	assert.Equal(t, "test.example.com", cert.CommonName)
	assert.Equal(t, "user-123", cert.UserID)
	assert.Equal(t, "tenant-1", cert.TenantID)
	assert.Equal(t, CertStatusActive, cert.Status)
	assert.NotEmpty(t, cert.CertificatePEM)
	assert.NotEmpty(t, cert.PrivateKeyPEM)
	assert.NotEmpty(t, cert.IssuingCA)
	assert.False(t, cert.IssuedAt.IsZero())
	assert.Equal(t, DefaultCertificateTTL, cert.TTL)

	// Verify cert is stored
	svc.mu.RLock()
	storedCert, exists := svc.certificates[cert.SerialNumber]
	svc.mu.RUnlock()
	assert.True(t, exists)
	assert.Equal(t, cert.SerialNumber, storedCert.SerialNumber)
}

func TestIssueCertificate_WithCustomTTL(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/issue/netweave-client": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"certificate":   "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
					"private_key":   "-----BEGIN RSA PRIVATE KEY-----\nkey\n-----END RSA PRIVATE KEY-----",
					"serial_number": "11:22:33:44",
					"issuing_ca":    "ca-cert",
					"expiration":    float64(time.Now().Add(30 * 24 * time.Hour).Unix()),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/master/protocol/openid-connect/token": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"access_token": "test-admin-token",
				"expires_in":   300,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/admin/realms/netweave/users/user-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				resp := map[string]interface{}{
					"id":       "user-1",
					"username": "testuser",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		},
	}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	req := &CertificateRequest{
		UserID:     "user-1",
		TenantID:   "tenant-1",
		CommonName: "test.example.com",
		TTL:        "720h",
	}

	cert, err := svc.IssueCertificate(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "720h", cert.TTL)
}

func TestIssueCertificate_VaultError(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/issue/netweave-client": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			resp := map[string]interface{}{
				"errors": []string{"permission denied"},
			}
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	req := &CertificateRequest{
		UserID:     "user-1",
		TenantID:   "tenant-1",
		CommonName: "test.example.com",
	}

	cert, err := svc.IssueCertificate(ctx, req)
	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "failed to issue certificate from vault")
}

func TestIssueCertificate_ContextCanceled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	svc := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := &CertificateRequest{
		UserID:     "user-1",
		TenantID:   "tenant-1",
		CommonName: "test.example.com",
	}

	cert, err := svc.IssueCertificate(ctx, req)
	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestIssueCertificate_KeycloakUpdateFailure(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/issue/netweave-client": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"certificate":   "cert-pem",
					"private_key":   "key-pem",
					"serial_number": "kc-fail-serial",
					"issuing_ca":    "ca-cert",
					"expiration":    float64(time.Now().Add(365 * 24 * time.Hour).Unix()),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/master/protocol/openid-connect/token": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"access_token": "test-admin-token",
				"expires_in":   300,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/admin/realms/netweave/users/user-kc-fail": func(w http.ResponseWriter, r *http.Request) {
			// Return error for GetUser
			w.WriteHeader(http.StatusInternalServerError)
		},
	}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	req := &CertificateRequest{
		UserID:     "user-kc-fail",
		TenantID:   "tenant-1",
		CommonName: "test.example.com",
	}

	// Should succeed even if Keycloak update fails
	cert, err := svc.IssueCertificate(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Equal(t, "kc-fail-serial", cert.SerialNumber)
}

func TestRevokeCertificate_Success(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/revoke": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"revocation_time":         float64(time.Now().Unix()),
					"revocation_time_rfc3339": time.Now().Format(time.RFC3339),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)

	// Add a cert to the store first
	svc.mu.Lock()
	svc.certificates["revoke-test"] = &Certificate{
		SerialNumber: "revoke-test",
		CommonName:   "revoke.example.com",
		Status:       CertStatusActive,
	}
	svc.mu.Unlock()

	ctx := context.Background()
	err := svc.RevokeCertificate(ctx, "revoke-test")
	require.NoError(t, err)

	// Verify status changed
	svc.mu.RLock()
	cert := svc.certificates["revoke-test"]
	svc.mu.RUnlock()
	assert.Equal(t, CertStatusRevoked, cert.Status)
	assert.NotNil(t, cert.RevokedAt)
}

func TestRevokeCertificate_VaultError(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/revoke": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			resp := map[string]interface{}{
				"errors": []string{"serial not found"},
			}
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	err := svc.RevokeCertificate(ctx, "non-existent-serial")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to revoke certificate in vault")
}

func TestRevokeCertificate_NotInStore(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/revoke": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"revocation_time": float64(time.Now().Unix()),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	// Revoke a certificate that's not in the store - should still succeed in Vault
	err := svc.RevokeCertificate(ctx, "unknown-serial")
	require.NoError(t, err)
}

func TestUpdateKeycloakUser_Success(t *testing.T) {
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/master/protocol/openid-connect/token": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"access_token": "test-admin-token",
				"expires_in":   300,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/admin/realms/netweave/users/user-update": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				resp := map[string]interface{}{
					"id":         "user-update",
					"username":   "testuser",
					"attributes": map[string][]string{"existing": {"value"}},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		},
	}
	vaultHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	cert := &Certificate{
		SerialNumber: "kc-test-serial",
		CommonName:   "test.example.com",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(365 * 24 * time.Hour),
	}

	err := svc.updateKeycloakUser(ctx, "user-update", cert)
	require.NoError(t, err)
}

func TestUpdateKeycloakUser_GetUserError(t *testing.T) {
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/master/protocol/openid-connect/token": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"access_token": "test-admin-token",
				"expires_in":   300,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/admin/realms/netweave/users/bad-user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
	}
	vaultHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	cert := &Certificate{
		SerialNumber: "kc-error-serial",
		CommonName:   "test.example.com",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(365 * 24 * time.Hour),
	}

	err := svc.updateKeycloakUser(ctx, "bad-user", cert)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get user")
}

func TestUpdateKeycloakUser_UpdateUserError(t *testing.T) {
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/master/protocol/openid-connect/token": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"access_token": "test-admin-token",
				"expires_in":   300,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/admin/realms/netweave/users/user-update-fail": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				resp := map[string]interface{}{
					"id":       "user-update-fail",
					"username": "testuser",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		},
	}
	vaultHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	cert := &Certificate{
		SerialNumber: "kc-update-fail-serial",
		CommonName:   "test.example.com",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(365 * 24 * time.Hour),
	}

	err := svc.updateKeycloakUser(ctx, "user-update-fail", cert)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update user")
}

func TestUpdateKeycloakUser_NilAttributes(t *testing.T) {
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/master/protocol/openid-connect/token": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"access_token": "test-admin-token",
				"expires_in":   300,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/admin/realms/netweave/users/user-nil-attrs": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				// Return user with no attributes field
				resp := map[string]interface{}{
					"id":       "user-nil-attrs",
					"username": "testuser",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		},
	}
	vaultHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	cert := &Certificate{
		SerialNumber: "nil-attrs-serial",
		CommonName:   "test.example.com",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(365 * 24 * time.Hour),
	}

	err := svc.updateKeycloakUser(ctx, "user-nil-attrs", cert)
	require.NoError(t, err)
}

func TestHealth_Success(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/sys/health": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/netweave/.well-known/openid-configuration": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"issuer": "test",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	err := svc.Health(ctx)
	require.NoError(t, err)
}

func TestHealth_VaultUnhealthy(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/sys/health": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	err := svc.Health(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault health check failed")
}

func TestHealth_KeycloakUnhealthy(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/sys/health": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/netweave/.well-known/openid-configuration": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		},
	}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	ctx := context.Background()

	err := svc.Health(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keycloak health check failed")
}

func TestNewService_Success(t *testing.T) {
	vaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer vaultSrv.Close()

	kcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer kcSrv.Close()

	config := DefaultConfig()
	config.VaultAddress = vaultSrv.URL
	config.VaultToken = "test-token"
	config.KeycloakBaseURL = kcSrv.URL
	config.KeycloakRealm = "netweave"
	config.KeycloakClientID = keycloak.AdminCLIClientID
	config.KeycloakAdminUsername = "admin"
	config.KeycloakAdminPassword = "admin"

	svc, err := NewService(config, zaptest.NewLogger(t))
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.vaultClient)
	assert.NotNil(t, svc.keycloakClient)
	assert.NotNil(t, svc.certificates)
}

func TestNewService_NilLogger(t *testing.T) {
	vaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer vaultSrv.Close()

	kcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer kcSrv.Close()

	config := DefaultConfig()
	config.VaultAddress = vaultSrv.URL
	config.VaultToken = "test-token"
	config.KeycloakBaseURL = kcSrv.URL
	config.KeycloakRealm = "netweave"
	config.KeycloakClientID = keycloak.AdminCLIClientID
	config.KeycloakAdminUsername = "admin"
	config.KeycloakAdminPassword = "admin"

	// nil logger should get a Nop logger
	svc, err := NewService(config, nil)
	require.NoError(t, err)
	require.NotNil(t, svc)
}

func TestNewService_VaultClientError(t *testing.T) {
	config := DefaultConfig()
	config.VaultToken = "" // Will cause vault client to fail (token required)
	config.KeycloakClientID = keycloak.AdminCLIClientID
	config.KeycloakAdminUsername = "admin"
	config.KeycloakAdminPassword = "admin"

	svc, err := NewService(config, zaptest.NewLogger(t))
	require.Error(t, err)
	assert.Nil(t, svc)
	assert.Contains(t, err.Error(), "failed to create Vault client")
}

func TestNewService_KeycloakClientError(t *testing.T) {
	config := DefaultConfig()
	config.VaultAddress = "http://localhost:8200"
	config.VaultToken = "test-token"
	config.KeycloakBaseURL = "" // Will fail validation
	config.KeycloakRealm = "netweave"

	svc, err := NewService(config, zaptest.NewLogger(t))
	require.Error(t, err)
	assert.Nil(t, svc)
	// Will hit "invalid config" because KeycloakBaseURL is required in config validation
	assert.Contains(t, err.Error(), "invalid config")
}

func TestRenewCertificate_Success(t *testing.T) {
	issueCalls := 0
	revokeCalls := 0

	var mu sync.Mutex

	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/issue/netweave-client": func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			issueCalls++
			mu.Unlock()
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"certificate":   "new-cert-pem",
					"private_key":   "new-key-pem",
					"serial_number": "new-serial-111",
					"issuing_ca":    "ca-cert",
					"expiration":    float64(time.Now().Add(365 * 24 * time.Hour).Unix()),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/v1/pki_int/revoke": func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			revokeCalls++
			mu.Unlock()
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"revocation_time": float64(time.Now().Unix()),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/master/protocol/openid-connect/token": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"access_token": "test-admin-token",
				"expires_in":   300,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/admin/realms/netweave/users/renew-user": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				resp := map[string]interface{}{
					"id":       "renew-user",
					"username": "testuser",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		},
	}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	svc.config.RenewalPolicy = &RenewalPolicy{
		RenewalWindow: 30 * 24 * time.Hour,
		MaxRetries:    3,
		RetryInterval: time.Hour,
	}

	oldCert := &Certificate{
		SerialNumber:    "old-serial-999",
		CommonName:      "renew.example.com",
		UserID:          "renew-user",
		TenantID:        "tenant-1",
		TTL:             "8760h",
		Status:          CertStatusExpiringSoon,
		RenewalAttempts: 0,
	}
	svc.mu.Lock()
	svc.certificates[oldCert.SerialNumber] = oldCert
	svc.mu.Unlock()

	ctx := context.Background()
	svc.renewCertificate(ctx, oldCert)

	mu.Lock()
	assert.Equal(t, 1, issueCalls)
	assert.Equal(t, 1, revokeCalls)
	mu.Unlock()
}

func TestRenewCertificate_IssuanceFailure(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/issue/netweave-client": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			resp := map[string]interface{}{
				"errors": []string{"issuance failed"},
			}
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	svc.config.RenewalPolicy = &RenewalPolicy{
		RenewalWindow: 30 * 24 * time.Hour,
		MaxRetries:    3,
		RetryInterval: time.Hour,
	}

	cert := &Certificate{
		SerialNumber:    "fail-renew-serial",
		CommonName:      "fail-renew.example.com",
		UserID:          "user-1",
		TenantID:        "tenant-1",
		TTL:             "8760h",
		Status:          CertStatusExpiringSoon,
		RenewalAttempts: 0,
	}
	svc.mu.Lock()
	svc.certificates[cert.SerialNumber] = cert
	svc.mu.Unlock()

	ctx := context.Background()
	svc.renewCertificate(ctx, cert)

	svc.mu.RLock()
	// After first failure, status should go back to ExpiringSoon
	assert.Equal(t, CertStatusExpiringSoon, cert.Status)
	assert.Equal(t, 1, cert.RenewalAttempts)
	svc.mu.RUnlock()
}

func TestRenewCertificate_MaxRetriesExceeded(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/issue/netweave-client": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			resp := map[string]interface{}{
				"errors": []string{"issuance failed"},
			}
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	svc.config.RenewalPolicy = &RenewalPolicy{
		RenewalWindow: 30 * 24 * time.Hour,
		MaxRetries:    3,
		RetryInterval: time.Hour,
	}

	cert := &Certificate{
		SerialNumber:    "max-retry-serial",
		CommonName:      "max-retry.example.com",
		UserID:          "user-1",
		TenantID:        "tenant-1",
		TTL:             "8760h",
		Status:          CertStatusExpiringSoon,
		RenewalAttempts: 2, // Already at max-1
	}
	svc.mu.Lock()
	svc.certificates[cert.SerialNumber] = cert
	svc.mu.Unlock()

	ctx := context.Background()
	svc.renewCertificate(ctx, cert)

	svc.mu.RLock()
	assert.Equal(t, CertStatusRenewalFailed, cert.Status)
	assert.Equal(t, 3, cert.RenewalAttempts)
	svc.mu.RUnlock()
}

func TestRenewCertificate_ContextCanceled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.RenewalPolicy = DefaultRenewalPolicy()

	svc := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	cert := &Certificate{
		SerialNumber: "ctx-cancel-serial",
		CommonName:   "ctx-cancel.example.com",
		UserID:       "user-1",
		TenantID:     "tenant-1",
		TTL:          "8760h",
		Status:       CertStatusExpiringSoon,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	svc.renewCertificate(ctx, cert)
	// Should return early without modifying certificate
	assert.Equal(t, CertStatusExpiringSoon, cert.Status)
}

func TestScanAndRenew_WithCertificates(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.EnableAutoRenewal = false // Disable auto-renewal so no goroutines spawn
	config.RenewalPolicy = &RenewalPolicy{
		RenewalWindow: 30 * 24 * time.Hour,
		MaxRetries:    3,
		RetryInterval: time.Hour,
	}

	rCtx, rCancel := context.WithCancel(context.Background())
	defer rCancel()

	svc := &Service{
		config:        config,
		logger:        logger,
		certificates:  make(map[string]*Certificate),
		renewalCtx:    rCtx,
		renewalCancel: rCancel,
	}

	now := time.Now()

	// Add various certificates
	svc.mu.Lock()
	svc.certificates["active-cert"] = &Certificate{
		SerialNumber: "active-cert",
		CommonName:   "active.example.com",
		Status:       CertStatusActive,
		ExpiresAt:    now.Add(90 * 24 * time.Hour), // Well within safe range
	}
	svc.certificates["expiring-cert"] = &Certificate{
		SerialNumber: "expiring-cert",
		CommonName:   "expiring.example.com",
		Status:       CertStatusActive,
		ExpiresAt:    now.Add(15 * 24 * time.Hour), // Within renewal window
	}
	svc.certificates["expired-cert"] = &Certificate{
		SerialNumber: "expired-cert",
		CommonName:   "expired.example.com",
		Status:       CertStatusActive,
		ExpiresAt:    now.Add(-1 * 24 * time.Hour), // Already expired
	}
	svc.mu.Unlock()

	ctx := context.Background()
	svc.scanAndRenew(ctx)

	svc.mu.RLock()
	defer svc.mu.RUnlock()
	// Active cert should remain active (not within renewal window)
	assert.Equal(t, CertStatusActive, svc.certificates["active-cert"].Status)
	// Expiring cert should be marked as expiring soon
	assert.Equal(t, CertStatusExpiringSoon, svc.certificates["expiring-cert"].Status)
	// Expired cert should be marked as expired
	assert.Equal(t, CertStatusExpired, svc.certificates["expired-cert"].Status)
}

func TestMonitorLoop_StopChannel(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.MonitorInterval = 50 * time.Millisecond
	config.RenewalPolicy = DefaultRenewalPolicy()

	rCtx, rCancel := context.WithCancel(context.Background())
	defer rCancel()

	svc := &Service{
		config:        config,
		logger:        logger,
		certificates:  make(map[string]*Certificate),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		renewalCtx:    rCtx,
		renewalCancel: rCancel,
	}

	ctx := context.Background()
	go svc.monitorLoop(ctx)

	// Let it run for a few iterations
	time.Sleep(200 * time.Millisecond)

	// Signal stop
	close(svc.stopCh)

	select {
	case <-svc.doneCh:
		// Expected
	case <-time.After(2 * time.Second):
		t.Fatal("monitorLoop did not stop within timeout")
	}
}

func TestMonitorLoop_ContextCancel(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.MonitorInterval = 50 * time.Millisecond
	config.RenewalPolicy = DefaultRenewalPolicy()

	rCtx, rCancel := context.WithCancel(context.Background())
	defer rCancel()

	svc := &Service{
		config:        config,
		logger:        logger,
		certificates:  make(map[string]*Certificate),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		renewalCtx:    rCtx,
		renewalCancel: rCancel,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go svc.monitorLoop(ctx)

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	select {
	case <-svc.doneCh:
		// Expected
	case <-time.After(2 * time.Second):
		t.Fatal("monitorLoop did not stop on context cancel within timeout")
	}
}

func TestProcessCertificate_ExpiringSoonWithRenewalPending(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.RenewalPolicy = &RenewalPolicy{
		RenewalWindow: 30 * 24 * time.Hour,
		MaxRetries:    3,
		RetryInterval: time.Hour,
	}

	svc := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	now := time.Now()
	renewalWindow := now.Add(config.RenewalPolicy.RenewalWindow)

	// Certificate already in RenewalPending should not trigger handleExpiringSoon
	cert := &Certificate{
		SerialNumber: "renewal-pending-cert",
		CommonName:   "pending.example.com",
		Status:       CertStatusRenewalPending,
		ExpiresAt:    now.Add(15 * 24 * time.Hour), // Within renewal window
	}

	svc.processCertificate(context.Background(), cert, now, renewalWindow)
	// Status should remain RenewalPending (not changed to ExpiringSoon)
	assert.Equal(t, CertStatusRenewalPending, cert.Status)
}

func TestGetMonitoringReport_RenewalPendingAndFailed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	svc := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	svc.certificates["pending-1"] = &Certificate{
		SerialNumber: "pending-1",
		Status:       CertStatusRenewalPending,
	}
	svc.certificates["failed-1"] = &Certificate{
		SerialNumber: "failed-1",
		Status:       CertStatusRenewalFailed,
	}

	ctx := context.Background()
	report, err := svc.GetMonitoringReport(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, report.TotalCertificates)
	assert.Equal(t, 1, report.RenewalsPending)
	assert.Equal(t, 1, report.RenewalsFailed)
}

func TestIssueCertificate_WithNotifier(t *testing.T) {
	var receivedEvent CertEvent
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		json.Unmarshal(body, &receivedEvent)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/issue/netweave-client": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"certificate":   "cert-pem",
					"private_key":   "key-pem",
					"serial_number": "notifier-serial",
					"issuing_ca":    "ca-cert",
					"expiration":    float64(time.Now().Add(365 * 24 * time.Hour).Unix()),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/master/protocol/openid-connect/token": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{"access_token": "token", "expires_in": 300}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/admin/realms/netweave/users/user-notifier": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				resp := map[string]interface{}{"id": "user-notifier", "username": "test"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		},
	}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: webhookServer.URL,
	}, zaptest.NewLogger(t))
	require.NoError(t, err)
	svc.SetNotifier(notifier)

	req := &CertificateRequest{
		UserID:     "user-notifier",
		TenantID:   "tenant-1",
		CommonName: "notifier-test.example.com",
	}

	cert, err := svc.IssueCertificate(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, cert)

	assert.Equal(t, CertEventIssued, receivedEvent.EventType)
	assert.Equal(t, "notifier-serial", receivedEvent.Certificate.SerialNumber)
}

func TestRevokeCertificate_WithNotification(t *testing.T) {
	var receivedEvent CertEvent
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		json.Unmarshal(body, &receivedEvent)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/revoke": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"revocation_time": float64(time.Now().Unix()),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: webhookServer.URL,
	}, zaptest.NewLogger(t))
	require.NoError(t, err)
	svc.SetNotifier(notifier)

	// Add cert to store
	svc.mu.Lock()
	svc.certificates["revoke-notify"] = &Certificate{
		SerialNumber: "revoke-notify",
		CommonName:   "revoke-notify.example.com",
		Status:       CertStatusActive,
	}
	svc.mu.Unlock()

	err = svc.RevokeCertificate(context.Background(), "revoke-notify")
	require.NoError(t, err)

	assert.Equal(t, CertEventRevoked, receivedEvent.EventType)
}

func TestHandleExpiringSoon_AutoRenewalEnabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.EnableAutoRenewal = true
	config.RenewalPolicy = DefaultRenewalPolicy()

	// Cancel renewal context immediately so spawned goroutine exits early
	// without calling IssueCertificate on nil vault client
	rCtx, rCancel := context.WithCancel(context.Background())
	rCancel()

	svc := &Service{
		config:        config,
		logger:        logger,
		certificates:  make(map[string]*Certificate),
		renewalCtx:    rCtx,
		renewalCancel: rCancel,
	}

	cert := &Certificate{
		SerialNumber: "auto-renew-cert",
		CommonName:   "auto-renew.example.com",
		UserID:       "user-1",
		TenantID:     "tenant-1",
		Status:       CertStatusActive,
		ExpiresAt:    time.Now().Add(7 * 24 * time.Hour),
	}

	svc.handleExpiringSoon(context.Background(), cert)

	// Wait for goroutine to spawn and exit
	time.Sleep(50 * time.Millisecond)

	svc.mu.RLock()
	status := cert.Status
	svc.mu.RUnlock()
	// Status should still be ExpiringSoon because handleExpiringSoon sets it
	// but renewal context was already canceled so renewal goroutine didn't run
	assert.Equal(t, CertStatusExpiringSoon, status)
}

func TestHandleExpiringSoon_AutoRenewalDisabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.EnableAutoRenewal = false
	config.RenewalPolicy = DefaultRenewalPolicy()

	rCtx, rCancel := context.WithCancel(context.Background())
	defer rCancel()

	svc := &Service{
		config:        config,
		logger:        logger,
		certificates:  make(map[string]*Certificate),
		renewalCtx:    rCtx,
		renewalCancel: rCancel,
	}

	cert := &Certificate{
		SerialNumber: "no-auto-renew-cert",
		CommonName:   "no-auto-renew.example.com",
		Status:       CertStatusActive,
		ExpiresAt:    time.Now().Add(7 * 24 * time.Hour),
	}

	svc.handleExpiringSoon(context.Background(), cert)

	svc.mu.RLock()
	assert.Equal(t, CertStatusExpiringSoon, cert.Status)
	svc.mu.RUnlock()
}

func TestRenewCertificate_OldCertRevocationFails(t *testing.T) {
	vaultHandlers := map[string]http.HandlerFunc{
		"/v1/pki_int/issue/netweave-client": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"certificate":   "new-cert",
					"private_key":   "new-key",
					"serial_number": fmt.Sprintf("new-serial-%d", time.Now().UnixNano()),
					"issuing_ca":    "ca",
					"expiration":    float64(time.Now().Add(365 * 24 * time.Hour).Unix()),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/v1/pki_int/revoke": func(w http.ResponseWriter, r *http.Request) {
			// Revocation fails
			w.WriteHeader(http.StatusBadRequest)
			resp := map[string]interface{}{
				"errors": []string{"revocation failed"},
			}
			json.NewEncoder(w).Encode(resp)
		},
	}
	kcHandlers := map[string]http.HandlerFunc{
		"/realms/master/protocol/openid-connect/token": func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{"access_token": "token", "expires_in": 300}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
		"/admin/realms/netweave/users/user-revoke-fail": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				resp := map[string]interface{}{"id": "user-revoke-fail", "username": "test"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		},
	}

	vaultSrv := setupVaultServer(t, vaultHandlers)
	defer vaultSrv.Close()
	kcSrv := setupKeycloakServer(t, kcHandlers)
	defer kcSrv.Close()

	svc := newTestServiceWithServers(t, vaultSrv, kcSrv)
	svc.config.RenewalPolicy = DefaultRenewalPolicy()

	cert := &Certificate{
		SerialNumber: "old-serial-revoke-fail",
		CommonName:   "revoke-fail.example.com",
		UserID:       "user-revoke-fail",
		TenantID:     "tenant-1",
		TTL:          "8760h",
		Status:       CertStatusExpiringSoon,
	}
	svc.mu.Lock()
	svc.certificates[cert.SerialNumber] = cert
	svc.mu.Unlock()

	ctx := context.Background()
	// Should not panic even when revocation of old cert fails
	svc.renewCertificate(ctx, cert)
}
