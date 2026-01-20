package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RevocationRequest represents a certificate revocation request.
type RevocationRequest struct {
	// SerialNumber is the serial number of the certificate to revoke.
	SerialNumber string `json:"serial_number"`
}

// RevocationResponse represents the response from a revocation request.
type RevocationResponse struct {
	// RevocationTime is when the certificate was revoked.
	RevocationTime time.Time `json:"revocation_time"`

	// RevocationTimeRFC3339 is the revocation time in RFC3339 format.
	RevocationTimeRFC3339 string `json:"revocation_time_rfc3339"`
}

// RevokeCertificate revokes a certificate by serial number.
func (c *Client) RevokeCertificate(ctx context.Context, serialNumber string) (*RevocationResponse, error) {
	if serialNumber == "" {
		return nil, fmt.Errorf("serialNumber is required")
	}

	reqData := RevocationRequest{
		SerialNumber: serialNumber,
	}

	body, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	path := fmt.Sprintf("%s/revoke", c.config.PKIPath)
	resp, err := c.doRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("revoke certificate request failed: %w", err)
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

	revocationResp := &RevocationResponse{
		RevocationTimeRFC3339: getString(data, "revocation_time_rfc3339"),
	}

	if revocationTime, ok := data["revocation_time"].(float64); ok {
		revocationResp.RevocationTime = time.Unix(int64(revocationTime), 0)
	}

	return revocationResp, nil
}

// RevokeCertificateBySerial is an alias for RevokeCertificate for clarity.
func (c *Client) RevokeCertificateBySerial(ctx context.Context, serialNumber string) error {
	_, err := c.RevokeCertificate(ctx, serialNumber)
	return err
}

// TidyCertificates tidies up the backend by removing expired certificates and revocation information.
// This operation can be long-running and should be used carefully.
func (c *Client) TidyCertificates(ctx context.Context) error {
	path := fmt.Sprintf("%s/tidy", c.config.PKIPath)

	tidyConfig := map[string]interface{}{
		"tidy_cert_store":    true,
		"tidy_revoked_certs": true,
		"safety_buffer":      "72h",
	}

	body, err := json.Marshal(tidyConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal tidy config: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("tidy request failed: %w", err)
	}
	defer func() {
		// Close response body - ignore error as response was already processed
		_ = resp.Body.Close()
	}()

	_, err = parseVaultResponse(resp)
	if err != nil {
		return fmt.Errorf("failed to parse tidy response: %w", err)
	}

	return nil
}

// GetTidyStatus retrieves the status of a tidy operation.
func (c *Client) GetTidyStatus(ctx context.Context) (map[string]interface{}, error) {
	path := fmt.Sprintf("%s/tidy-status", c.config.PKIPath)

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get tidy status request failed: %w", err)
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

	return data, nil
}

// RotateCRL rotates the Certificate Revocation List.
// This forces generation of a new CRL.
func (c *Client) RotateCRL(ctx context.Context) error {
	path := fmt.Sprintf("%s/crl/rotate", c.config.PKIPath)

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return fmt.Errorf("rotate CRL request failed: %w", err)
	}
	defer func() {
		// Close response body - ignore error as response was already processed
		_ = resp.Body.Close()
	}()

	_, err = parseVaultResponse(resp)
	if err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}
