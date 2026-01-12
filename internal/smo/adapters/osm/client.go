package osm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client provides a REST API client for OSM NBI (Northbound Interface).
// It handles authentication, request/response marshaling, error handling,
// and automatic token refresh.
type Client struct {
	config     *Config
	httpClient *http.Client
	baseURL    string

	// Authentication state
	mu          sync.RWMutex
	token       string
	tokenExpiry time.Time
}

// NewClient creates a new OSM NBI API client with the provided configuration.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Parse and validate base URL
	baseURL := strings.TrimSuffix(config.NBIURL, "/")
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("invalid nbiUrl: %w", err)
	}

	httpClient := &http.Client{
		Timeout: config.RequestTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &Client{
		config:     config,
		httpClient: httpClient,
		baseURL:    baseURL,
	}, nil
}

// Authenticate authenticates with the OSM NBI and obtains an access token.
// The token is cached and automatically refreshed when expired.
func (c *Client) Authenticate(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we have a valid token
	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	// Prepare authentication request
	authReq := map[string]string{
		"username": c.config.Username,
		"password": c.config.Password,
		"project":  c.config.Project,
	}

	reqBody, err := json.Marshal(authReq)
	if err != nil {
		return fmt.Errorf("failed to marshal auth request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/osm/admin/v1/tokens",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log close error but don't fail the operation
			// Response body has already been processed
		}
	}()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("authentication failed (status %d, failed to read body: %w)", resp.StatusCode, err)
		}
		return fmt.Errorf("authentication failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var authResp struct {
		ID        string `json:"id"`
		ProjectID string `json:"project_id"`
		Expires   string `json:"expires"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	// Parse token expiry (OSM returns ISO 8601 format)
	expiry, err := time.Parse(time.RFC3339, authResp.Expires)
	if err != nil {
		// If parsing fails, set expiry to 1 hour from now
		expiry = time.Now().Add(1 * time.Hour)
	}

	// Store token
	c.token = authResp.ID
	c.tokenExpiry = expiry

	return nil
}

// Health performs a health check on the OSM NBI API.
// It verifies connectivity and authentication status.
func (c *Client) Health(ctx context.Context) error {
	// Ensure we have a valid token
	if err := c.Authenticate(ctx); err != nil {
		return fmt.Errorf("authentication check failed: %w", err)
	}

	// Perform a lightweight API call to verify connectivity
	req, err := c.newRequest(ctx, http.MethodGet, "/osm/admin/v1/tokens", nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log close error but don't fail the operation
			// Response body has already been processed
		}
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("health check failed (status %d)", resp.StatusCode)
	}

	return nil
}

// Close closes the HTTP client and releases resources.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Invalidate token
	c.token = ""
	c.tokenExpiry = time.Time{}

	// Close idle connections
	c.httpClient.CloseIdleConnections()

	return nil
}

// newRequest creates a new HTTP request with authentication and common headers.
func (c *Client) newRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	// Ensure we have a valid token
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()

	if token == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	// Build URL
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid URL path: %w", err)
	}

	// Marshal body if provided
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	return req, nil
}

// doRequest executes an HTTP request and handles the response.
// It automatically retries on transient failures and refreshes authentication if needed.
func (c *Client) doRequest(ctx context.Context, req *http.Request, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if err := c.waitForRetry(ctx, attempt); err != nil {
			return err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		newLastErr, err := c.handleResponse(ctx, req, resp, result)

		// Close response body immediately to avoid resource leak
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log close error but don't fail the operation
		}

		if newLastErr != nil {
			lastErr = newLastErr
		}
		if err != nil {
			if errors.Is(err, errRetryable) {
				continue
			}
			return err
		}
		return nil
	}

	return fmt.Errorf("request failed after %d attempts: %w", c.config.MaxRetries+1, lastErr)
}

// errRetryable is a sentinel error indicating the request should be retried.
var errRetryable = fmt.Errorf("retryable error")

// waitForRetry implements exponential backoff for retry attempts.
func (c *Client) waitForRetry(ctx context.Context, attempt int) error {
	if attempt == 0 {
		return nil
	}

	delay := time.Duration(float64(c.config.RetryDelay) * float64(attempt) * c.config.RetryMultiplier)
	if delay > c.config.RetryMaxDelay {
		delay = c.config.RetryMaxDelay
	}

	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context canceled during retry wait: %w", ctx.Err())
	}
}

// handleResponse processes the HTTP response based on status code.
func (c *Client) handleResponse(
	ctx context.Context,
	req *http.Request,
	resp *http.Response,
	result interface{},
) (error, error) {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
		return c.handleSuccessResponse(resp, result), nil

	case http.StatusUnauthorized:
		return c.handleUnauthorized(ctx, req, resp)

	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return c.handleRetryableError(resp)

	default:
		return c.handleNonRetryableError(resp), nil
	}
}

// handleSuccessResponse processes successful HTTP responses.
// Note: resp.Body is closed by caller's defer.
func (c *Client) handleSuccessResponse(resp *http.Response, result interface{}) error {
	if result != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}

// handleUnauthorized handles 401 responses by refreshing authentication.
// Note: resp.Body is closed by caller's defer.
// Returns (lastErr, retryableErr).
func (c *Client) handleUnauthorized(ctx context.Context, req *http.Request, _ *http.Response) (error, error) {
	if err := c.Authenticate(ctx); err != nil {
		return nil, fmt.Errorf("failed to refresh authentication: %w", err)
	}

	c.mu.RLock()
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.mu.RUnlock()

	return fmt.Errorf("authentication expired, retrying"), errRetryable
}

// handleRetryableError handles retryable HTTP errors (rate limiting, service unavailable).
// Note: resp.Body is closed by caller's defer.
// Returns (lastErr, retryableErr).
func (c *Client) handleRetryableError(resp *http.Response) (error, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("request failed (status %d, failed to read body: %w)", resp.StatusCode, err), errRetryable
	}
	return fmt.Errorf("request failed (status %d): %s", resp.StatusCode, string(body)), errRetryable
}

// handleNonRetryableError handles non-retryable HTTP errors.
// Note: resp.Body is closed by caller's defer.
func (c *Client) handleNonRetryableError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("request failed (status %d, failed to read body: %w)", resp.StatusCode, err)
	}
	return fmt.Errorf("request failed (status %d): %s", resp.StatusCode, string(body))
}

// get performs a GET request to the specified path.
func (c *Client) get(ctx context.Context, path string, result interface{}) error {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return c.doRequest(ctx, req, result)
}

// post performs a POST request to the specified path with the given body.
func (c *Client) post(ctx context.Context, path string, body, result interface{}) error {
	req, err := c.newRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	return c.doRequest(ctx, req, result)
}

// delete performs a DELETE request to the specified path.
func (c *Client) delete(ctx context.Context, path string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	return c.doRequest(ctx, req, nil)
}

// patch performs a PATCH request to the specified path with the given body.
func (c *Client) patch(ctx context.Context, path string, body, result interface{}) error {
	req, err := c.newRequest(ctx, http.MethodPatch, path, body)
	if err != nil {
		return err
	}
	return c.doRequest(ctx, req, result)
}
