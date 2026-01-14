package starlingx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AuthClient handles Keystone authentication for StarlingX API access.
type AuthClient struct {
	keystoneEndpoint string
	username         string
	password         string
	projectName      string
	domainName       string
	httpClient       *http.Client
	logger           *zap.Logger

	mu          sync.RWMutex
	token       string
	tokenExpiry time.Time
}

// NewAuthClient creates a new Keystone authentication client.
func NewAuthClient(keystoneEndpoint, username, password, projectName, domainName string, logger *zap.Logger) *AuthClient {
	return &AuthClient{
		keystoneEndpoint: keystoneEndpoint,
		username:         username,
		password:         password,
		projectName:      projectName,
		domainName:       domainName,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// GetToken retrieves a valid authentication token, refreshing if necessary.
func (a *AuthClient) GetToken(ctx context.Context) (string, error) {
	a.mu.RLock()
	if a.token != "" && time.Now().Before(a.tokenExpiry) {
		token := a.token
		a.mu.RUnlock()
		return token, nil
	}
	a.mu.RUnlock()

	// Token is expired or doesn't exist, acquire write lock and refresh
	a.mu.Lock()
	defer a.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed)
	if a.token != "" && time.Now().Before(a.tokenExpiry) {
		return a.token, nil
	}

	return a.authenticate(ctx)
}

// authenticate performs Keystone v3 authentication and updates the token.
func (a *AuthClient) authenticate(ctx context.Context) (string, error) {
	authReq := a.buildAuthRequest()

	body, err := json.Marshal(authReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth request: %w", err)
	}

	req, err := a.createAuthHTTPRequest(ctx, body)
	if err != nil {
		return "", err
	}

	token, expiry, err := a.executeAuthRequest(req)
	if err != nil {
		return "", err
	}

	a.token = token
	a.tokenExpiry = expiry
	a.logger.Info("keystone authentication successful",
		zap.String("username", a.username),
		zap.Time("expires", a.tokenExpiry),
	)

	return token, nil
}

func (a *AuthClient) buildAuthRequest() KeystoneAuthRequest {
	return KeystoneAuthRequest{
		Auth: KeystoneAuth{
			Identity: KeystoneIdentity{
				Methods: []string{"password"},
				Password: KeystonePassword{
					User: KeystoneUser{
						Name: a.username,
						Domain: KeystoneDomain{
							Name: a.domainName,
						},
						Password: a.password,
					},
				},
			},
			Scope: KeystoneScope{
				Project: KeystoneProject{
					Name: a.projectName,
					Domain: KeystoneDomain{
						Name: a.domainName,
					},
				},
			},
		},
	}
}

func (a *AuthClient) createAuthHTTPRequest(ctx context.Context, body []byte) (*http.Request, error) {
	authURL := fmt.Sprintf("%s/v3/auth/tokens", a.keystoneEndpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	a.logger.Debug("authenticating with Keystone",
		zap.String("endpoint", authURL),
		zap.String("username", a.username),
		zap.String("project", a.projectName),
	)

	return req, nil
}

func (a *AuthClient) executeAuthRequest(req *http.Request) (string, time.Time, error) {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to execute auth request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			a.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	if resp.StatusCode != http.StatusCreated {
		return "", time.Time{}, a.handleAuthError(resp)
	}

	token := resp.Header.Get("X-Subject-Token")
	if token == "" {
		return "", time.Time{}, fmt.Errorf("no X-Subject-Token header in response")
	}

	expiry := a.parseTokenExpiry(resp.Body)
	return token, expiry, nil
}

func (a *AuthClient) handleAuthError(resp *http.Response) error {
	bodyBytes, _ := io.ReadAll(resp.Body)
	a.logger.Error("keystone authentication failed",
		zap.Int("status", resp.StatusCode),
		zap.String("body", string(bodyBytes)),
	)
	return fmt.Errorf("keystone authentication failed with status %d: %s", resp.StatusCode, string(bodyBytes))
}

func (a *AuthClient) parseTokenExpiry(body io.Reader) time.Time {
	var authResp map[string]interface{}
	if err := json.NewDecoder(body).Decode(&authResp); err == nil {
		if tokenData, ok := authResp["token"].(map[string]interface{}); ok {
			if expiresAt, ok := tokenData["expires_at"].(string); ok {
				if expiry, err := time.Parse(time.RFC3339, expiresAt); err == nil {
					// Refresh 5 minutes before actual expiry
					a.logger.Debug("token expiry set", zap.Time("expiry", expiry))
					return expiry.Add(-5 * time.Minute)
				}
			}
		}
	}

	// Default expiry if parsing failed
	return time.Now().Add(55 * time.Minute) // Default 1 hour minus 5 min buffer
}

// InvalidateToken clears the cached token, forcing re-authentication on next request.
func (a *AuthClient) InvalidateToken() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = ""
	a.tokenExpiry = time.Time{}
	a.logger.Debug("token invalidated")
}
