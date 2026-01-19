// Package keycloak provides integration with Keycloak for user, tenant, and role management.
// It supports OAuth2/OIDC authentication flows and provides a bridge between Keycloak's
// user management and the gateway's auth.Store interface.
package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config holds the configuration for connecting to Keycloak.
type Config struct {
	// BaseURL is the Keycloak server base URL (e.g., "http://localhost:8090").
	BaseURL string

	// Realm is the Keycloak realm name.
	Realm string

	// ClientID is the OAuth2 client ID for the gateway.
	ClientID string

	// ClientSecret is the OAuth2 client secret.
	ClientSecret string

	// AdminUsername is the Keycloak admin username (for admin API access).
	AdminUsername string

	// AdminPassword is the Keycloak admin password.
	AdminPassword string

	// Timeout is the HTTP client timeout.
	Timeout time.Duration

	// HTTPClient allows providing a custom HTTP client.
	HTTPClient *http.Client
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		BaseURL:  "http://localhost:8090",
		Realm:    "netweave",
		Timeout:  30 * time.Second,
		ClientID: "netweave-gateway",
	}
}

// Client provides access to Keycloak's APIs.
type Client struct {
	config      *Config
	httpClient  *http.Client
	accessToken string
	tokenExpiry time.Time
}

// NewClient creates a new Keycloak client.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if config.BaseURL == "" {
		return nil, fmt.Errorf("BaseURL is required")
	}
	if config.Realm == "" {
		return nil, fmt.Errorf("Realm is required")
	}
	if config.ClientID == "" {
		return nil, fmt.Errorf("ClientID is required")
	}
	if config.ClientSecret == "" {
		return nil, fmt.Errorf("ClientSecret is required")
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

// TokenResponse represents an OAuth2 token response.
type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
}

// VerifyToken verifies and validates an OAuth2 access token.
// Returns the decoded token claims if valid.
func (c *Client) VerifyToken(ctx context.Context, token string) (map[string]interface{}, error) {
	introspectURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token/introspect",
		c.config.BaseURL, c.config.Realm)

	data := url.Values{}
	data.Set("token", token)
	data.Set("client_id", c.config.ClientID)
	data.Set("client_secret", c.config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, introspectURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create introspection request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token introspection request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token introspection failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode introspection response: %w", err)
	}

	active, ok := result["active"].(bool)
	if !ok || !active {
		return nil, fmt.Errorf("token is not active")
	}

	return result, nil
}

// GetUserInfo retrieves user information from the OIDC userinfo endpoint.
func (c *Client) GetUserInfo(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	userinfoURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/userinfo",
		c.config.BaseURL, c.config.Realm)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo request failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo response: %w", err)
	}

	return userInfo, nil
}

// ExchangePasswordCredentials exchanges username/password for an access token.
// This is used for password grant flow (not recommended for production).
func (c *Client) ExchangePasswordCredentials(ctx context.Context, username, password string) (*TokenResponse, error) {
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		c.config.BaseURL, c.config.Realm)

	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", c.config.ClientID)
	data.Set("client_secret", c.config.ClientSecret)
	data.Set("username", username)
	data.Set("password", password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token request failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// getAdminToken obtains an admin access token for making admin API calls.
// Tokens are cached and automatically refreshed when expired.
func (c *Client) getAdminToken(ctx context.Context) (string, error) {
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	tokenURL := fmt.Sprintf("%s/realms/master/protocol/openid-connect/token", c.config.BaseURL)

	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", "admin-cli")
	data.Set("username", c.config.AdminUsername)
	data.Set("password", c.config.AdminPassword)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create admin token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("admin token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("admin token request failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode admin token response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	return c.accessToken, nil
}

// doAdminRequest executes an authenticated request to the Keycloak Admin API.
func (c *Client) doAdminRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	adminToken, err := c.getAdminToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get admin token: %w", err)
	}

	reqURL := fmt.Sprintf("%s/admin/realms/%s%s", c.config.BaseURL, c.config.Realm, path)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+adminToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("admin request failed: %w", err)
	}

	return resp, nil
}

// Ping checks if the Keycloak server is accessible.
func (c *Client) Ping(ctx context.Context) error {
	wellKnownURL := fmt.Sprintf("%s/realms/%s/.well-known/openid-configuration",
		c.config.BaseURL, c.config.Realm)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ping request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("keycloak server returned status %d", resp.StatusCode)
	}

	return nil
}

// Close closes the client and releases resources.
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}
