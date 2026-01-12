package dtias

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
)

// Client provides access to the Dell DTIAS REST API.
// It handles authentication, TLS configuration, retries, and error handling.
type Client struct {
	// httpClient is the underlying HTTP client with TLS configuration
	httpClient *http.Client

	// baseURL is the DTIAS API endpoint
	baseURL string

	// apiKey is the authentication API key
	apiKey string

	// retryAttempts is the number of retry attempts for failed requests
	retryAttempts int

	// retryDelay is the delay between retry attempts
	retryDelay time.Duration

	// logger provides structured logging
	logger *zap.Logger
}

// ClientConfig holds configuration for creating a DTIAS Client.
type ClientConfig struct {
	// Endpoint is the DTIAS API endpoint URL
	Endpoint string

	// APIKey is the authentication API key
	APIKey string

	// ClientCert is the path to the client certificate for mTLS (optional)
	ClientCert string

	// ClientKey is the path to the client key for mTLS (optional)
	ClientKey string

	// CACert is the path to the CA certificate for server verification (optional)
	// If not provided, system root CAs are used
	CACert string

	// Timeout is the HTTP client timeout
	Timeout time.Duration

	// RetryAttempts is the number of retry attempts for failed requests
	RetryAttempts int

	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration

	// Logger provides structured logging
	Logger *zap.Logger
}

// NewClient creates a new DTIAS API client with the provided configuration.
// It configures TLS, authentication, and retry behavior.
func NewClient(cfg *ClientConfig) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Initialize logger
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Create secure TLS configuration - always validate certificates
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	// Load client certificate for mTLS if provided
	if cfg.ClientCert != "" && cfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate for server verification if provided
	if cfg.CACert != "" {
		caCert, err := os.ReadFile(cfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Create HTTP client with TLS configuration
	httpClient := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			TLSClientConfig:     tlsConfig,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &Client{
		httpClient:    httpClient,
		baseURL:       cfg.Endpoint,
		apiKey:        cfg.APIKey,
		retryAttempts: cfg.RetryAttempts,
		retryDelay:    cfg.RetryDelay,
		logger:        cfg.Logger,
	}, nil
}

// marshalRequestBody converts a request body to a JSON reader.
// Returns nil reader when body is nil (valid for GET/DELETE requests).
func (c *Client) marshalRequestBody(body interface{}) (io.Reader, error) {
	if body == nil {
		return http.NoBody, nil
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	return bytes.NewReader(jsonBody), nil
}

// createHTTPRequest creates an HTTP request with authentication headers.
func (c *Client) createHTTPRequest(
	ctx context.Context,
	method, url string,
	bodyReader io.Reader,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "netweave-o2ims-gateway/1.0")

	return req, nil
}

// isRetryableResponse checks if a response should trigger a retry.
func (c *Client) isRetryableResponse(resp *http.Response) (bool, error) {
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return true, fmt.Errorf("failed to close response body: %w", closeErr)
		}
		return true, fmt.Errorf("server error: %s", resp.Status)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return true, fmt.Errorf("failed to close response body: %w", closeErr)
		}
		return true, fmt.Errorf("rate limited: %s", resp.Status)
	}

	return false, nil
}

// doRequest performs an HTTP request with authentication, retries, and error handling.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	bodyReader, err := c.marshalRequestBody(body)
	if err != nil {
		return nil, err
	}

	url := c.baseURL + path
	var lastErr error

	for attempt := 0; attempt <= c.retryAttempts; attempt++ {
		if attempt > 0 {
			c.logger.Debug("retrying request",
				zap.Int("attempt", attempt),
				zap.String("method", method),
				zap.String("url", url))
			time.Sleep(c.retryDelay)
		}

		req, err := c.createHTTPRequest(ctx, method, url, bodyReader)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		retryable, retryErr := c.isRetryableResponse(resp)
		if retryable {
			lastErr = retryErr
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", c.retryAttempts+1, lastErr)
}

// parseResponse parses a JSON response body into the target structure.
func (c *Client) parseResponse(resp *http.Response, target interface{}) error {
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error status codes
	if resp.StatusCode >= 400 {
		// Try to parse error response
		var apiErr APIError
		if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
			return &apiErr
		}
		return fmt.Errorf("API error: %s (status %d)", string(body), resp.StatusCode)
	}

	// Parse success response
	if target != nil {
		if err := json.Unmarshal(body, target); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// HealthCheck verifies connectivity to the DTIAS API.
func (c *Client) HealthCheck(ctx context.Context) error {
	// DTIAS uses /v2/serverhealth or /v2/version for health checks
	resp, err := c.doRequest(ctx, http.MethodGet, "/v2/version", nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}

	return nil
}

// Close closes the HTTP client and releases resources.
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// APIError represents an error response from the DTIAS API.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Error implements the error interface for APIError.
func (e *APIError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("DTIAS API error [%s]: %s - %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("DTIAS API error [%s]: %s", e.Code, e.Message)
}
