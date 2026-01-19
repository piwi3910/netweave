// Package vault provides integration with HashiCorp Vault for PKI certificate management.
// It supports certificate issuance, revocation, and CA operations for mTLS authentication.
package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config holds the configuration for connecting to Vault.
type Config struct {
	// Address is the Vault server address (e.g., "http://localhost:8200").
	Address string

	// Token is the Vault authentication token.
	Token string

	// PKIPath is the path to the PKI secrets engine (e.g., "pki_int").
	PKIPath string

	// Timeout is the HTTP client timeout.
	Timeout time.Duration

	// HTTPClient allows providing a custom HTTP client.
	HTTPClient *http.Client
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Address: "http://localhost:8200",
		PKIPath: "pki_int",
		Timeout: 30 * time.Second,
	}
}

// Client provides access to Vault's PKI APIs.
type Client struct {
	config     *Config
	httpClient *http.Client
}

// NewClient creates a new Vault client.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if config.Address == "" {
		return nil, fmt.Errorf("address is required")
	}
	if config.Token == "" {
		return nil, fmt.Errorf("token is required")
	}
	if config.PKIPath == "" {
		return nil, fmt.Errorf("PKIPath is required")
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		timeout := config.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		httpClient = &http.Client{
			Timeout: timeout,
		}
	}

	return &Client{
		config:     config,
		httpClient: httpClient,
	}, nil
}

// Response represents a generic Vault API response.
type Response struct {
	Data     map[string]interface{} `json:"data,omitempty"`
	Warnings []string               `json:"warnings,omitempty"`
	Errors   []string               `json:"errors,omitempty"`
}

// doRequest executes a request to the Vault API.
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	reqURL := fmt.Sprintf("%s/v1/%s", c.config.Address, path)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Vault-Token", c.config.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// parseVaultResponse parses a Vault API response.
func parseVaultResponse(resp *http.Response) (*Response, error) {
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		var vaultResp Response
		if err := json.Unmarshal(body, &vaultResp); err == nil && len(vaultResp.Errors) > 0 {
			return nil, fmt.Errorf("vault error (status=%d): %s", resp.StatusCode, strings.Join(vaultResp.Errors, ", "))
		}
		return nil, fmt.Errorf("vault request failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var vaultResp Response
	if err := json.Unmarshal(body, &vaultResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &vaultResp, nil
}

// Ping checks if the Vault server is accessible and unsealed.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodGet, "sys/health", nil)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Vault returns 200 if initialized, unsealed, and active
	// 429 if unsealed and standby
	// 472 if disaster recovery mode replication secondary and active
	// 473 if performance standby
	// 501 if not initialized
	// 503 if sealed
	healthyStatuses := []int{
		http.StatusOK,
		http.StatusTooManyRequests, // standby
		472,                        // DR secondary
		473,                        // performance standby
	}
	for _, status := range healthyStatuses {
		if resp.StatusCode == status {
			return nil
		}
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("vault is not healthy: status=%d, body=%s", resp.StatusCode, string(body))
}

// GetCAChain retrieves the CA certificate chain.
func (c *Client) GetCAChain(ctx context.Context) (string, error) {
	path := fmt.Sprintf("%s/ca_chain", c.config.PKIPath)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get CA chain: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get CA chain failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	caChain, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read CA chain: %w", err)
	}

	return string(caChain), nil
}

// GetCACertificate retrieves the CA certificate.
func (c *Client) GetCACertificate(ctx context.Context) (string, error) {
	path := fmt.Sprintf("%s/ca/pem", c.config.PKIPath)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get CA certificate: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get CA certificate failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	caCert, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read CA certificate: %w", err)
	}

	return string(caCert), nil
}

// GetCRL retrieves the Certificate Revocation List.
func (c *Client) GetCRL(ctx context.Context) (string, error) {
	path := fmt.Sprintf("%s/crl/pem", c.config.PKIPath)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get CRL: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get CRL failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	crl, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read CRL: %w", err)
	}

	return string(crl), nil
}

// Close closes the client and releases resources.
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}
