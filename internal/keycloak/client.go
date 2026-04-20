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
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// AdminCLIClientID is the public client ID used for Keycloak admin API access.
	// This client does not require a client secret when authenticating with admin credentials.
	AdminCLIClientID = "admin-cli"
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
//
// The admin access token cache (accessToken / tokenExpiry) is guarded by
// tokenMu. Concurrent refreshes are coalesced via tokenGroup so at most one
// admin-credentials grant is ever in flight, preventing Keycloak's
// brute-force detector from locking the master realm under burst load.
type Client struct {
	config     *Config
	httpClient *http.Client

	tokenMu     sync.RWMutex
	accessToken string
	tokenExpiry time.Time
	tokenGroup  singleflight.Group
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

	// ClientSecret is required for all clients except admin-cli when using admin credentials.
	// admin-cli is a public client in Keycloak that authenticates using username/password
	// for admin API access and does not require a client secret.
	isAdminCLI := config.ClientID == AdminCLIClientID
	hasAdminCredentials := config.AdminUsername != "" && config.AdminPassword != ""
	if config.ClientSecret == "" && (!isAdminCLI || !hasAdminCredentials) {
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

// VerifyToken verifies an OAuth2 access token via Keycloak's introspection
// endpoint and returns the decoded claims map when the token is active.
//
// SECURITY: VerifyToken only asserts that the token is active from Keycloak's
// perspective. It does NOT verify the "aud", "iss", or "client_id" claims,
// because those checks are policy decisions owned by the calling gateway
// (the client_id / realm URL that we trust may differ per deployment).
//
// Callers MUST additionally enforce audience, issuer, and client_id bindings
// before accepting the returned claims — see auth.OAuth2Authenticator which
// pins these via OAuth2Config.ExpectedAudience / ExpectedIssuer /
// AllowedClientIDs. Accepting any active token would otherwise let a
// compromised client in the same Keycloak instance replay tokens here.
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
	defer func() {
		_ = resp.Body.Close()
	}()

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
	defer func() {
		_ = resp.Body.Close()
	}()

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

// DevExchangePasswordCredentials exchanges username/password for an access token
// using the OAuth2 Resource Owner Password Credentials (ROPC) grant.
//
// FOR INTERNAL TOOLING AND DEVELOPMENT ONLY — DO NOT EXPOSE OVER HTTP.
//
// The ROPC grant is deprecated by OAuth 2.1 and insecure for any resource
// server that shares a codebase with IdP credentials. It exists here solely
// to support local development scripts and integration tests that need to
// acquire a bearer token without driving a browser. Production callers MUST
// use the authorization code flow with PKCE and pass the resulting bearer
// token through the normal Authorization header path.
//
// The function's name is prefixed with Dev to make its scope obvious in call
// sites and to steer reviewers toward richer flows (see docs/security/).
func (c *Client) DevExchangePasswordCredentials(
	ctx context.Context,
	username, password string,
) (*TokenResponse, error) {
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
	defer func() {
		_ = resp.Body.Close()
	}()

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
// Tokens are cached and automatically refreshed when expired. Concurrent
// callers share a single in-flight refresh via singleflight, so the master
// realm sees at most one admin-credentials grant per refresh cycle.
func (c *Client) getAdminToken(ctx context.Context) (string, error) {
	// Fast path: RLock-only cache hit.
	c.tokenMu.RLock()
	cachedToken := c.accessToken
	cachedExpiry := c.tokenExpiry
	c.tokenMu.RUnlock()
	if cachedToken != "" && time.Now().Before(cachedExpiry) {
		return cachedToken, nil
	}

	// Slow path: collapse concurrent refreshes into one HTTP call.
	v, err, _ := c.tokenGroup.Do("admin-token", func() (interface{}, error) {
		// Re-check after acquiring the singleflight slot — another goroutine
		// may have refreshed the cache while we were queued.
		c.tokenMu.RLock()
		token := c.accessToken
		expiry := c.tokenExpiry
		c.tokenMu.RUnlock()
		if token != "" && time.Now().Before(expiry) {
			return token, nil
		}
		return c.refreshAdminToken(ctx)
	})
	if err != nil {
		// refreshAdminToken already wraps its HTTP/decoding errors with a
		// descriptive prefix; re-wrap here so the singleflight layer is
		// visible in stack traces.
		return "", fmt.Errorf("admin token refresh failed: %w", err)
	}
	token, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("singleflight returned non-string token of type %T", v)
	}
	return token, nil
}

// refreshAdminToken performs the admin-credentials grant against Keycloak
// and atomically populates the cached token/expiry. Callers must route
// through getAdminToken so the refresh is serialized via singleflight.
func (c *Client) refreshAdminToken(ctx context.Context) (string, error) {
	tokenURL := fmt.Sprintf("%s/realms/master/protocol/openid-connect/token", c.config.BaseURL)

	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", AdminCLIClientID)
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
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("admin token request failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode admin token response: %w", err)
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	c.tokenMu.Lock()
	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = expiry
	c.tokenMu.Unlock()

	return tokenResp.AccessToken, nil
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
	defer func() {
		_ = resp.Body.Close()
	}()

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
