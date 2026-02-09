package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const (
	gatewayClientTimeout = 30 * time.Second
	defaultGatewayURL    = "https://api.netweave.local"
	defaultAuthURL       = "https://auth.netweave.local"
	defaultUsername      = "admin@netweave.local"
	defaultPassword      = "admin"
	defaultClientID      = "netweave-gateway"
	defaultSecretName    = "netweave-secret"
	defaultSecretKey     = "keycloak-client-secret"
)

// GatewayConnection holds an authenticated connection to the gateway admin API.
type GatewayConnection struct {
	token   string
	baseURL string
	client  *http.Client
}

// GatewayConfig holds the configuration for connecting to the gateway.
type GatewayConfig struct {
	GatewayURL   string
	AuthURL      string
	Username     string
	Password     string
	ClientSecret string
	Namespace    string
	Kubeconfig   string
}

// DefaultGatewayConfig returns a GatewayConfig with default values.
func DefaultGatewayConfig() *GatewayConfig {
	return &GatewayConfig{
		GatewayURL: defaultGatewayURL,
		AuthURL:    defaultAuthURL,
		Username:   defaultUsername,
		Password:   defaultPassword,
	}
}

// ConnectGateway authenticates with Keycloak and returns a GatewayConnection.
func ConnectGateway(ctx context.Context, cfg *GatewayConfig) (*GatewayConnection, error) {
	clientSecret := cfg.ClientSecret
	if clientSecret == "" {
		secret, err := readClientSecret(ctx, cfg.Namespace, cfg.Kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to read client secret from K8s (use --client-secret flag): %w", err)
		}
		clientSecret = secret
	}

	httpClient := NewInsecureTLSClient(gatewayClientTimeout)

	token, err := acquireToken(ctx, httpClient, cfg.AuthURL, clientSecret, cfg.Username, cfg.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire OAuth2 token: %w", err)
	}

	return &GatewayConnection{
		token:   token,
		baseURL: strings.TrimRight(cfg.GatewayURL, "/"),
		client:  httpClient,
	}, nil
}

// Get performs an authenticated GET request.
func (g *GatewayConnection) Get(ctx context.Context, path string) ([]byte, error) {
	return g.doRequest(ctx, http.MethodGet, path, nil)
}

// Post performs an authenticated POST request with a JSON body.
func (g *GatewayConnection) Post(ctx context.Context, path string, body interface{}) ([]byte, error) {
	return g.doJSONRequest(ctx, http.MethodPost, path, body)
}

// Put performs an authenticated PUT request with a JSON body.
func (g *GatewayConnection) Put(ctx context.Context, path string, body interface{}) ([]byte, error) {
	return g.doJSONRequest(ctx, http.MethodPut, path, body)
}

// Delete performs an authenticated DELETE request.
func (g *GatewayConnection) Delete(ctx context.Context, path string) error {
	_, err := g.doRequest(ctx, http.MethodDelete, path, nil)
	return err
}

func (g *GatewayConnection) doJSONRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	return g.doRequest(ctx, method, path, bytes.NewReader(jsonBody))
}

func (g *GatewayConnection) doRequest(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	reqURL := g.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// tokenResponse represents the OAuth2 token endpoint response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func acquireToken(
	ctx context.Context, client *http.Client,
	authURL, clientSecret, username, password string,
) (string, error) {
	tokenURL := strings.TrimRight(authURL, "/") + "/realms/netweave/protocol/openid-connect/token"

	data := url.Values{
		"grant_type":    {"password"},
		"client_id":     {defaultClientID},
		"client_secret": {clientSecret},
		"username":      {username},
		"password":      {password},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}

	return tokenResp.AccessToken, nil
}

func readClientSecret(
	ctx context.Context, namespace, kubeconfig string,
) (string, error) {
	if namespace == "" {
		namespace = "netweave"
	}

	args := []string{
		"get", "secret", "-n", namespace, defaultSecretName,
		"-o", "jsonpath={.data." + defaultSecretKey + "}",
	}
	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}

	out, err := exec.CommandContext(ctx, "kubectl", args...).Output()
	if err != nil {
		return "", fmt.Errorf("kubectl get secret failed: %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return "", fmt.Errorf("failed to decode secret: %w", err)
	}

	return strings.TrimSpace(string(decoded)), nil
}
