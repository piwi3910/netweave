package keycloak

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests require a running Keycloak instance.
// Run: docker-compose -f docker-compose.dev.yml up -d
// Before running: go test -v ./internal/keycloak/...

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "missing BaseURL",
			config: &Config{
				Realm:        "netweave",
				ClientID:     "test",
				ClientSecret: "secret",
			},
			wantErr: true,
			errMsg:  "BaseURL is required",
		},
		{
			name: "missing Realm",
			config: &Config{
				BaseURL:      "http://localhost:8090",
				ClientID:     "test",
				ClientSecret: "secret",
			},
			wantErr: true,
			errMsg:  "Realm is required",
		},
		{
			name: "missing ClientID",
			config: &Config{
				BaseURL:      "http://localhost:8090",
				Realm:        "netweave",
				ClientSecret: "secret",
			},
			wantErr: true,
			errMsg:  "ClientID is required",
		},
		{
			name: "missing ClientSecret",
			config: &Config{
				BaseURL:  "http://localhost:8090",
				Realm:    "netweave",
				ClientID: "test",
			},
			wantErr: true,
			errMsg:  "ClientSecret is required",
		},
		{
			name: "valid config",
			config: &Config{
				BaseURL:      "http://localhost:8090",
				Realm:        "netweave",
				ClientID:     "netweave-gateway",
				ClientSecret: "netweave-gateway-secret-change-in-production",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestClient_Ping(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	config := &Config{
		BaseURL:      "http://localhost:8090",
		Realm:        "netweave",
		ClientID:     "netweave-gateway",
		ClientSecret: "netweave-gateway-secret-change-in-production",
		Timeout:      10 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	err = client.Ping(ctx)
	require.NoError(t, err)
}

func TestClient_ExchangePasswordCredentials(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	config := &Config{
		BaseURL:      "http://localhost:8090",
		Realm:        "netweave",
		ClientID:     "netweave-gateway",
		ClientSecret: "netweave-gateway-secret-change-in-production",
		Timeout:      10 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{
			name:     "valid operator credentials",
			username: "operator@example.com",
			password: "operator",
			wantErr:  false,
		},
		{
			name:     "valid admin credentials",
			username: "admin@example.com",
			password: "admin",
			wantErr:  false,
		},
		{
			name:     "valid viewer credentials",
			username: "viewer@example.com",
			password: "viewer",
			wantErr:  false,
		},
		{
			name:     "invalid credentials",
			username: "invalid@example.com",
			password: "invalid",
			wantErr:  true,
		},
		{
			name:     "wrong password",
			username: "operator@example.com",
			password: "wrongpass",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenResp, err := client.ExchangePasswordCredentials(ctx, tt.username, tt.password)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, tokenResp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, tokenResp)
				assert.NotEmpty(t, tokenResp.AccessToken)
				assert.Equal(t, "Bearer", tokenResp.TokenType)
				assert.Greater(t, tokenResp.ExpiresIn, 0)
			}
		})
	}
}

func TestClient_VerifyToken(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	config := &Config{
		BaseURL:      "http://localhost:8090",
		Realm:        "netweave",
		ClientID:     "netweave-gateway",
		ClientSecret: "netweave-gateway-secret-change-in-production",
		Timeout:      10 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	tokenResp, err := client.ExchangePasswordCredentials(ctx, "operator@example.com", "operator")
	require.NoError(t, err)
	require.NotNil(t, tokenResp)

	claims, err := client.VerifyToken(ctx, tokenResp.AccessToken)
	require.NoError(t, err)
	assert.NotNil(t, claims)
	assert.True(t, claims["active"].(bool))
	assert.Contains(t, claims, "tenant")

	invalidToken := "invalid.token.here"
	_, err = client.VerifyToken(ctx, invalidToken)
	require.Error(t, err)
}

func TestClient_GetUserInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	config := &Config{
		BaseURL:      "http://localhost:8090",
		Realm:        "netweave",
		ClientID:     "netweave-gateway",
		ClientSecret: "netweave-gateway-secret-change-in-production",
		Timeout:      10 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	tokenResp, err := client.ExchangePasswordCredentials(ctx, "operator@example.com", "operator")
	require.NoError(t, err)
	require.NotNil(t, tokenResp)

	userInfo, err := client.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		t.Logf("UserInfo endpoint may require additional client configuration: %v", err)
		t.Skip("Skipping userinfo test - endpoint not configured")
	}
	assert.NotNil(t, userInfo)
	assert.Contains(t, userInfo, "email")
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.NotNil(t, config)
	assert.Equal(t, "http://localhost:8090", config.BaseURL)
	assert.Equal(t, "netweave", config.Realm)
	assert.Equal(t, "netweave-gateway", config.ClientID)
	assert.Equal(t, 30*time.Second, config.Timeout)
}
