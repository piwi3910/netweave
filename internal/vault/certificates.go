package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// CertificateRequest represents a certificate issuance request.
type CertificateRequest struct {
	// CommonName is the CN (Common Name) for the certificate.
	CommonName string `json:"common_name"`

	// AltNames are Subject Alternative Names (DNS names).
	// Will be converted to comma-separated string for Vault API.
	AltNames []string `json:"-"`

	// IPSANs are IP Subject Alternative Names.
	// Will be converted to comma-separated string for Vault API.
	IPSANs []string `json:"-"`

	// URISANs are URI Subject Alternative Names.
	// Will be converted to comma-separated string for Vault API.
	URISANs []string `json:"-"`

	// TTL is the requested Time To Live for the certificate.
	TTL string `json:"ttl,omitempty"`

	// Format specifies the certificate format (pem, der, pem_bundle).
	Format string `json:"format,omitempty"`

	// PrivateKeyFormat specifies the private key format (der, pkcs8).
	PrivateKeyFormat string `json:"private_key_format,omitempty"`

	// ExcludeCNFromSANs excludes CN from Subject Alternative Names.
	ExcludeCNFromSANs bool `json:"exclude_cn_from_sans,omitempty"`
}

// Certificate represents an issued certificate.
type Certificate struct {
	// Certificate is the PEM-encoded certificate.
	Certificate string `json:"certificate"`

	// PrivateKey is the PEM-encoded private key.
	PrivateKey string `json:"private_key"`

	// PrivateKeyType is the type of private key (rsa, ec, ed25519).
	PrivateKeyType string `json:"private_key_type"`

	// SerialNumber is the certificate serial number.
	SerialNumber string `json:"serial_number"`

	// CAChain is the certificate authority chain.
	CAChain []string `json:"ca_chain"`

	// IssuingCA is the issuing CA certificate.
	IssuingCA string `json:"issuing_ca"`

	// Expiration is the certificate expiration time.
	Expiration time.Time `json:"expiration"`

	// LeaseID is the Vault lease ID for the certificate.
	LeaseID string `json:"lease_id"`

	// LeaseDuration is the duration of the Vault lease.
	LeaseDuration int `json:"lease_duration"`

	// Renewable indicates if the lease is renewable.
	Renewable bool `json:"renewable"`
}

// buildCertRequestData builds a request data map from a CertificateRequest.
func buildCertRequestData(req *CertificateRequest) map[string]interface{} {
	reqData := map[string]interface{}{
		"common_name": req.CommonName,
		"format":      req.Format,
	}

	if req.TTL != "" {
		reqData["ttl"] = req.TTL
	}
	if req.PrivateKeyFormat != "" {
		reqData["private_key_format"] = req.PrivateKeyFormat
	}
	if req.ExcludeCNFromSANs {
		reqData["exclude_cn_from_sans"] = true
	}

	// Convert slice fields to comma-separated strings
	if len(req.AltNames) > 0 {
		reqData["alt_names"] = joinStrings(req.AltNames)
	}
	if len(req.IPSANs) > 0 {
		reqData["ip_sans"] = joinStrings(req.IPSANs)
	}
	if len(req.URISANs) > 0 {
		reqData["uri_sans"] = joinStrings(req.URISANs)
	}

	return reqData
}

// joinStrings joins a slice of strings with commas.
func joinStrings(values []string) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += ","
		}
		result += value
	}
	return result
}

// IssueCertificate issues a new certificate from the specified role.
func (c *Client) IssueCertificate(ctx context.Context, roleName string, req *CertificateRequest) (*Certificate, error) {
	if roleName == "" {
		return nil, fmt.Errorf("roleName is required")
	}
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if req.CommonName == "" {
		return nil, fmt.Errorf("CommonName is required")
	}

	if req.Format == "" {
		req.Format = "pem"
	}

	reqData := buildCertRequestData(req)
	body, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	path := fmt.Sprintf("%s/issue/%s", c.config.PKIPath, roleName)
	resp, err := c.doRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("issue certificate request failed: %w", err)
	}
	defer func() {
		// Close response body - ignore error as response was already processed
		_ = resp.Body.Close()
	}()

	vaultResp, err := parseVaultResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return parseCertificateData(vaultResp, true), nil
}

// parseCertificateData extracts certificate information from Vault response data.
func parseCertificateData(vaultResp *Response, includePrivateKey bool) *Certificate {
	cert := &Certificate{
		LeaseID:       getString(vaultResp.Data, "lease_id"),
		LeaseDuration: getInt(vaultResp.Data, "lease_duration"),
		Renewable:     getBool(vaultResp.Data, "renewable"),
	}

	data, ok := vaultResp.Data["data"].(map[string]interface{})
	if !ok {
		data = vaultResp.Data
	}

	cert.Certificate = getString(data, "certificate")
	cert.SerialNumber = getString(data, "serial_number")
	cert.IssuingCA = getString(data, "issuing_ca")

	if includePrivateKey {
		cert.PrivateKey = getString(data, "private_key")
		cert.PrivateKeyType = getString(data, "private_key_type")
	}

	if caChain, ok := data["ca_chain"].([]interface{}); ok {
		for _, ca := range caChain {
			if caStr, ok := ca.(string); ok {
				cert.CAChain = append(cert.CAChain, caStr)
			}
		}
	}

	if expirationFloat, ok := data["expiration"].(float64); ok {
		cert.Expiration = time.Unix(int64(expirationFloat), 0)
	}

	return cert
}

// SignCSR signs a Certificate Signing Request using the specified role.
func (c *Client) SignCSR(ctx context.Context, roleName, csr string, ttl string) (*Certificate, error) {
	if roleName == "" {
		return nil, fmt.Errorf("roleName is required")
	}
	if csr == "" {
		return nil, fmt.Errorf("csr is required")
	}

	reqData := map[string]interface{}{
		"csr":    csr,
		"format": "pem",
	}
	if ttl != "" {
		reqData["ttl"] = ttl
	}

	body, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	path := fmt.Sprintf("%s/sign/%s", c.config.PKIPath, roleName)
	resp, err := c.doRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sign CSR request failed: %w", err)
	}
	defer func() {
		// Close response body - ignore error as response was already processed
		_ = resp.Body.Close()
	}()

	vaultResp, err := parseVaultResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return parseCertificateData(vaultResp, false), nil
}

// GetCertificate retrieves a certificate by serial number.
func (c *Client) GetCertificate(ctx context.Context, serialNumber string) (string, error) {
	if serialNumber == "" {
		return "", fmt.Errorf("serialNumber is required")
	}

	path := fmt.Sprintf("%s/cert/%s", c.config.PKIPath, serialNumber)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("get certificate request failed: %w", err)
	}
	defer func() {
		// Close response body - ignore error as response was already processed
		_ = resp.Body.Close()
	}()

	vaultResp, err := parseVaultResponse(resp)
	if err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	data, ok := vaultResp.Data["data"].(map[string]interface{})
	if !ok {
		data = vaultResp.Data
	}

	certificate := getString(data, "certificate")
	if certificate == "" {
		return "", fmt.Errorf("certificate not found in response")
	}

	return certificate, nil
}

// ListCertificates lists all certificate serial numbers.
func (c *Client) ListCertificates(ctx context.Context) ([]string, error) {
	path := fmt.Sprintf("%s/certs", c.config.PKIPath)
	resp, err := c.doRequest(ctx, "LIST", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list certificates request failed: %w", err)
	}
	defer func() {
		// Close response body - ignore error as response was already processed
		_ = resp.Body.Close()
	}()

	vaultResp, err := parseVaultResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	data, ok := vaultResp.Data["data"].(map[string]interface{})
	if !ok {
		data = vaultResp.Data
	}

	keys, ok := data["keys"].([]interface{})
	if !ok {
		return []string{}, nil
	}

	serialNumbers := make([]string, 0, len(keys))
	for _, key := range keys {
		if serialStr, ok := key.(string); ok {
			serialNumbers = append(serialNumbers, serialStr)
		}
	}

	return serialNumbers, nil
}

// Helper functions to extract values from map[string]interface{}.
func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

func getInt(data map[string]interface{}, key string) int {
	if val, ok := data[key].(float64); ok {
		return int(val)
	}
	return 0
}

func getBool(data map[string]interface{}, key string) bool {
	if val, ok := data[key].(bool); ok {
		return val
	}
	return false
}
