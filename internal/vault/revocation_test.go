package vault

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_RevokeCertificate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	// First, issue a certificate to revoke
	cert, err := client.IssueCertificate(ctx, "netweave-client", &CertificateRequest{
		CommonName: fmt.Sprintf("revoke-test-%d.netweave.local", timestamp),
		TTL:        "24h",
	})
	require.NoError(t, err)
	require.NotNil(t, cert)

	tests := []struct {
		name         string
		serialNumber string
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "valid revocation",
			serialNumber: cert.SerialNumber,
			wantErr:      false,
		},
		{
			name:         "empty serial number",
			serialNumber: "",
			wantErr:      true,
			errMsg:       "serialNumber is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			revResp, err := client.RevokeCertificate(ctx, tt.serialNumber)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, revResp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, revResp)
				assert.False(t, revResp.RevocationTime.IsZero())
				assert.NotEmpty(t, revResp.RevocationTimeRFC3339)

				// Verify revocation time is recent
				assert.WithinDuration(t, time.Now(), revResp.RevocationTime, 5*time.Second)
			}
		})
	}
}

func TestClient_RevokeCertificateBySerial(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	// Issue a certificate to revoke
	cert, err := client.IssueCertificate(ctx, "netweave-client", &CertificateRequest{
		CommonName: fmt.Sprintf("revoke-by-serial-test-%d.netweave.local", timestamp),
		TTL:        "24h",
	})
	require.NoError(t, err)
	require.NotNil(t, cert)

	// Revoke using the alias method
	err = client.RevokeCertificateBySerial(ctx, cert.SerialNumber)
	require.NoError(t, err)
}

func TestClient_TidyCertificates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()

	// Tidy operation should succeed
	err := client.TidyCertificates(ctx)
	require.NoError(t, err)
}

func TestClient_GetTidyStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()

	// Trigger a tidy operation first
	err := client.TidyCertificates(ctx)
	require.NoError(t, err)

	// Get tidy status
	status, err := client.GetTidyStatus(ctx)
	require.NoError(t, err)
	assert.NotNil(t, status)

	// Status should contain expected fields
	// Note: Fields may vary depending on Vault version
	t.Logf("Tidy status: %+v", status)
}

func TestClient_RotateCRL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()

	// Get CRL before rotation
	crlBefore, err := client.GetCRL(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, crlBefore)

	// Rotate CRL
	err = client.RotateCRL(ctx)
	require.NoError(t, err)

	// Get CRL after rotation
	crlAfter, err := client.GetCRL(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, crlAfter)

	// CRLs should both be valid PEM
	assert.Contains(t, crlBefore, "BEGIN X509 CRL")
	assert.Contains(t, crlAfter, "BEGIN X509 CRL")
}

func TestClient_RevocationWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	// Step 1: Get initial CRL
	crlBefore, err := client.GetCRL(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, crlBefore)

	// Step 2: Issue multiple certificates
	var certs []*Certificate
	for i := 0; i < 3; i++ {
		cert, err := client.IssueCertificate(ctx, "netweave-client", &CertificateRequest{
			CommonName: fmt.Sprintf("workflow-test-%d-%d.netweave.local", timestamp, i),
			TTL:        "24h",
		})
		require.NoError(t, err)
		certs = append(certs, cert)
	}

	// Step 3: Revoke first certificate
	revResp, err := client.RevokeCertificate(ctx, certs[0].SerialNumber)
	require.NoError(t, err)
	assert.NotNil(t, revResp)

	// Step 4: Rotate CRL to ensure revocation is published
	err = client.RotateCRL(ctx)
	require.NoError(t, err)

	// Step 5: Get updated CRL
	crlAfter, err := client.GetCRL(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, crlAfter)

	// Step 6: Verify CRL changed (in real scenario, would parse and check revoked certs)
	assert.Contains(t, crlAfter, "BEGIN X509 CRL")

	// Step 7: Verify other certificates still retrievable
	for i := 1; i < len(certs); i++ {
		retrievedCert, err := client.GetCertificate(ctx, certs[i].SerialNumber)
		require.NoError(t, err)
		assert.NotEmpty(t, retrievedCert)
	}

	// Step 8: Trigger tidy operation
	err = client.TidyCertificates(ctx)
	require.NoError(t, err)

	// Step 9: Check tidy status
	status, err := client.GetTidyStatus(ctx)
	require.NoError(t, err)
	assert.NotNil(t, status)
}

func TestClient_MultipleRevocations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	// Issue multiple certificates
	var serialNumbers []string
	for i := 0; i < 5; i++ {
		cert, err := client.IssueCertificate(ctx, "netweave-client", &CertificateRequest{
			CommonName: fmt.Sprintf("multi-revoke-%d-%d.netweave.local", timestamp, i),
			TTL:        "24h",
		})
		require.NoError(t, err)
		serialNumbers = append(serialNumbers, cert.SerialNumber)
	}

	// Revoke all certificates
	for _, serial := range serialNumbers {
		revResp, err := client.RevokeCertificate(ctx, serial)
		require.NoError(t, err)
		assert.NotNil(t, revResp)
		assert.False(t, revResp.RevocationTime.IsZero())
	}

	// Rotate CRL to publish revocations
	err := client.RotateCRL(ctx)
	require.NoError(t, err)

	// Verify CRL exists
	crl, err := client.GetCRL(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, crl)
	assert.Contains(t, crl, "BEGIN X509 CRL")
}
