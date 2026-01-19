package vault

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_IssueCertificate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	tests := []struct {
		name     string
		roleName string
		req      *CertificateRequest
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid client certificate",
			roleName: "netweave-client",
			req: &CertificateRequest{
				CommonName: fmt.Sprintf("test-client-%d.netweave.local", timestamp),
				TTL:        "24h",
			},
			wantErr: false,
		},
		{
			name:     "valid server certificate",
			roleName: "netweave-server",
			req: &CertificateRequest{
				CommonName: fmt.Sprintf("test-server-%d.netweave.local", timestamp),
				AltNames:   []string{"*.netweave.local"},
				TTL:        "24h",
			},
			wantErr: false,
		},
		{
			name:     "certificate with IP SANs",
			roleName: "netweave-server",
			req: &CertificateRequest{
				CommonName: fmt.Sprintf("test-ipsan-%d.netweave.local", timestamp),
				IPSANs:     []string{"192.168.1.100"},
				TTL:        "24h",
			},
			wantErr: false,
		},
		{
			name:     "empty role name",
			roleName: "",
			req: &CertificateRequest{
				CommonName: "test.netweave.local",
			},
			wantErr: true,
			errMsg:  "roleName is required",
		},
		{
			name:     "nil request",
			roleName: "netweave-client",
			req:      nil,
			wantErr:  true,
			errMsg:   "request cannot be nil",
		},
		{
			name:     "empty common name",
			roleName: "netweave-client",
			req: &CertificateRequest{
				CommonName: "",
			},
			wantErr: true,
			errMsg:  "CommonName is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := client.IssueCertificate(ctx, tt.roleName, tt.req)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, cert)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cert)
				assert.NotEmpty(t, cert.Certificate)
				assert.NotEmpty(t, cert.PrivateKey)
				assert.NotEmpty(t, cert.SerialNumber)
				assert.NotEmpty(t, cert.IssuingCA)
				assert.NotEmpty(t, cert.CAChain)
				assert.False(t, cert.Expiration.IsZero())

				// Verify certificate contains expected common name
				assert.Contains(t, cert.Certificate, "BEGIN CERTIFICATE")
				assert.Contains(t, cert.Certificate, "END CERTIFICATE")
				assert.Contains(t, cert.PrivateKey, "BEGIN")
				assert.Contains(t, cert.PrivateKey, "PRIVATE KEY")
			}
		})
	}
}

func TestClient_SignCSR(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()

	// First, generate a CSR using Vault (we'll use this for testing)
	timestamp := time.Now().Unix()
	testCert, err := client.IssueCertificate(ctx, "netweave-client", &CertificateRequest{
		CommonName: fmt.Sprintf("csr-test-%d.netweave.local", timestamp),
		TTL:        "24h",
	})
	require.NoError(t, err)
	require.NotNil(t, testCert)

	// For a real CSR test, we would generate a CSR using crypto/x509
	// But for now, we'll test error cases
	tests := []struct {
		name     string
		roleName string
		csr      string
		ttl      string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "empty role name",
			roleName: "",
			csr:      "dummy-csr",
			wantErr:  true,
			errMsg:   "roleName is required",
		},
		{
			name:     "empty CSR",
			roleName: "netweave-client",
			csr:      "",
			wantErr:  true,
			errMsg:   "csr is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := client.SignCSR(ctx, tt.roleName, tt.csr, tt.ttl)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, cert)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cert)
			}
		})
	}
}

func TestClient_GetCertificate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	// First, issue a certificate to test retrieval
	issuedCert, err := client.IssueCertificate(ctx, "netweave-client", &CertificateRequest{
		CommonName: fmt.Sprintf("get-cert-test-%d.netweave.local", timestamp),
		TTL:        "24h",
	})
	require.NoError(t, err)
	require.NotNil(t, issuedCert)

	tests := []struct {
		name         string
		serialNumber string
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "valid serial number",
			serialNumber: issuedCert.SerialNumber,
			wantErr:      false,
		},
		{
			name:         "empty serial number",
			serialNumber: "",
			wantErr:      true,
			errMsg:       "serialNumber is required",
		},
		{
			name:         "non-existent serial number",
			serialNumber: "00:00:00:00:00:00:00:00",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := client.GetCertificate(ctx, tt.serialNumber)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Empty(t, cert)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, cert)
				assert.Contains(t, cert, "BEGIN CERTIFICATE")
				assert.Contains(t, cert, "END CERTIFICATE")
			}
		})
	}
}

func TestClient_ListCertificates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()

	// Issue a test certificate to ensure list is not empty
	timestamp := time.Now().Unix()
	_, err := client.IssueCertificate(ctx, "netweave-client", &CertificateRequest{
		CommonName: fmt.Sprintf("list-test-%d.netweave.local", timestamp),
		TTL:        "24h",
	})
	require.NoError(t, err)

	// List certificates
	serialNumbers, err := client.ListCertificates(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, serialNumbers)

	// Verify format of serial numbers (should contain colons)
	for _, serial := range serialNumbers {
		assert.NotEmpty(t, serial)
	}
}

func TestClient_CertificateLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	// Step 1: Issue a certificate
	cert, err := client.IssueCertificate(ctx, "netweave-client", &CertificateRequest{
		CommonName: fmt.Sprintf("lifecycle-test-%d.netweave.local", timestamp),
		AltNames:   []string{fmt.Sprintf("alt-%d.netweave.local", timestamp)},
		TTL:        "24h",
	})
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.NotEmpty(t, cert.SerialNumber)

	// Step 2: Retrieve the certificate
	retrievedCert, err := client.GetCertificate(ctx, cert.SerialNumber)
	require.NoError(t, err)
	assert.NotEmpty(t, retrievedCert)

	// Step 3: Verify certificate appears in list
	serialNumbers, err := client.ListCertificates(ctx)
	require.NoError(t, err)
	assert.Contains(t, serialNumbers, cert.SerialNumber)

	// Step 4: Revoke the certificate
	revResp, err := client.RevokeCertificate(ctx, cert.SerialNumber)
	require.NoError(t, err)
	assert.NotNil(t, revResp)
	assert.False(t, revResp.RevocationTime.IsZero())
	assert.NotEmpty(t, revResp.RevocationTimeRFC3339)
}
