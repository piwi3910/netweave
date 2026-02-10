package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockOAuth2Server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/realms/netweave/protocol/openid-connect/token" {
			w.Header().Set("Content-Type", "application/json")
			resp := tokenResponse{
				AccessToken: "mock-token-12345",
				TokenType:   "Bearer",
				ExpiresIn:   300,
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
}

func newMockGatewayServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// OAuth2 token endpoint.
	mux.HandleFunc("/realms/netweave/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := tokenResponse{
			AccessToken: "mock-token-12345",
			TokenType:   "Bearer",
			ExpiresIn:   300,
		}
		json.NewEncoder(w).Encode(resp)
	})

	// API endpoints.
	mux.HandleFunc("/admin/infrastructure/backends", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"backends":[],"total":0}`))
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"b-1","name":"test"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/admin/test/ok", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/admin/test/error", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal server error`))
	})

	mux.HandleFunc("/admin/test/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			w.Write([]byte(`{"status":"slow"}`))
		}
	})

	return httptest.NewServer(mux)
}

func connectToMockGateway(t *testing.T, serverURL string) *GatewayConnection {
	t.Helper()
	ctx := context.Background()

	gw, err := ConnectGateway(ctx, &GatewayConfig{
		GatewayURL:   serverURL,
		AuthURL:      serverURL,
		Username:     "test@example.com",
		Password:     "testpass",
		ClientSecret: "mock-secret",
	})
	require.NoError(t, err)
	return gw
}

func TestConnectGateway_Success(t *testing.T) {
	srv := newMockGatewayServer(t)
	defer srv.Close()

	gw := connectToMockGateway(t, srv.URL)
	assert.NotNil(t, gw)
	assert.Equal(t, "mock-token-12345", gw.token)
	assert.Equal(t, srv.URL, gw.baseURL)
}

func TestConnectGateway_TokenFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := ConnectGateway(ctx, &GatewayConfig{
		GatewayURL:   srv.URL,
		AuthURL:      srv.URL,
		Username:     "bad@example.com",
		Password:     "wrong",
		ClientSecret: "bad-secret",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to acquire OAuth2 token")
	assert.Contains(t, err.Error(), "authentication failed")
	assert.NotContains(t, err.Error(), "invalid_client")
}

func TestConnectGateway_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"","token_type":"Bearer"}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := ConnectGateway(ctx, &GatewayConfig{
		GatewayURL:   srv.URL,
		AuthURL:      srv.URL,
		Username:     "test@example.com",
		Password:     "testpass",
		ClientSecret: "mock-secret",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty access token")
}

func TestGatewayConnection_Get(t *testing.T) {
	srv := newMockGatewayServer(t)
	defer srv.Close()

	gw := connectToMockGateway(t, srv.URL)

	data, err := gw.Get(context.Background(), "/admin/test/ok")
	require.NoError(t, err)
	assert.Contains(t, string(data), `"status":"ok"`)
}

func TestGatewayConnection_Get_ErrorStatus(t *testing.T) {
	srv := newMockGatewayServer(t)
	defer srv.Close()

	gw := connectToMockGateway(t, srv.URL)

	_, err := gw.Get(context.Background(), "/admin/test/error")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error (HTTP 500)")
}

func TestGatewayConnection_Post(t *testing.T) {
	srv := newMockGatewayServer(t)
	defer srv.Close()

	gw := connectToMockGateway(t, srv.URL)

	body := map[string]string{"name": "test-backend"}
	data, err := gw.Post(context.Background(), "/admin/infrastructure/backends", body)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"id":"b-1"`)
}

func TestGatewayConnection_Put(t *testing.T) {
	srv := newMockGatewayServer(t)
	defer srv.Close()

	gw := connectToMockGateway(t, srv.URL)

	body := map[string]string{"status": "updated"}
	data, err := gw.Put(context.Background(), "/admin/test/ok", body)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"status":"ok"`)
}

func TestGatewayConnection_Delete(t *testing.T) {
	srv := newMockGatewayServer(t)
	defer srv.Close()

	gw := connectToMockGateway(t, srv.URL)

	err := gw.Delete(context.Background(), "/admin/test/ok")
	require.NoError(t, err)
}

func TestGatewayConnection_Delete_Error(t *testing.T) {
	srv := newMockGatewayServer(t)
	defer srv.Close()

	gw := connectToMockGateway(t, srv.URL)

	err := gw.Delete(context.Background(), "/admin/test/error")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error (HTTP 500)")
}

func TestGatewayConnection_ContextCancelled(t *testing.T) {
	srv := newMockGatewayServer(t)
	defer srv.Close()

	gw := connectToMockGateway(t, srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := gw.Get(ctx, "/admin/test/slow")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")
}

func TestDefaultGatewayConfig(t *testing.T) {
	cfg := DefaultGatewayConfig()

	assert.Equal(t, "https://api.netweave.local", cfg.GatewayURL)
	assert.Equal(t, "https://auth.netweave.local", cfg.AuthURL)
	assert.Equal(t, "admin@netweave.local", cfg.Username)
	assert.Equal(t, "admin", cfg.Password)
	assert.Empty(t, cfg.ClientSecret)
	assert.Empty(t, cfg.Namespace)
	assert.Empty(t, cfg.Kubeconfig)
}

func TestValidateNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid namespace",
			namespace: "netweave",
			wantErr:   false,
		},
		{
			name:      "valid namespace with hyphens",
			namespace: "my-namespace-01",
			wantErr:   false,
		},
		{
			name:      "single character",
			namespace: "a",
			wantErr:   false,
		},
		{
			name:      "starts with uppercase",
			namespace: "Netweave",
			wantErr:   true,
			errMsg:    "not a valid DNS-1123 label",
		},
		{
			name:      "contains underscore",
			namespace: "my_namespace",
			wantErr:   true,
			errMsg:    "not a valid DNS-1123 label",
		},
		{
			name:      "starts with hyphen",
			namespace: "-invalid",
			wantErr:   true,
			errMsg:    "not a valid DNS-1123 label",
		},
		{
			name:      "ends with hyphen",
			namespace: "invalid-",
			wantErr:   true,
			errMsg:    "not a valid DNS-1123 label",
		},
		{
			name:      "too long",
			namespace: "a234567890123456789012345678901234567890123456789012345678901234",
			wantErr:   true,
			errMsg:    "exceeds maximum length",
		},
		{
			name:      "contains spaces",
			namespace: "my namespace",
			wantErr:   true,
			errMsg:    "not a valid DNS-1123 label",
		},
		{
			name:      "contains dots",
			namespace: "my.namespace",
			wantErr:   true,
			errMsg:    "not a valid DNS-1123 label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNamespace(tt.namespace)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAcquireToken_Success(t *testing.T) {
	srv := newMockOAuth2Server(t)
	defer srv.Close()

	client := NewInsecureTLSClient(5 * time.Second)
	token, err := acquireToken(
		context.Background(), client,
		srv.URL, "secret", "user@example.com", "pass",
	)

	require.NoError(t, err)
	assert.Equal(t, "mock-token-12345", token)
}

func TestAcquireToken_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`service unavailable`))
	}))
	defer srv.Close()

	client := NewInsecureTLSClient(5 * time.Second)
	_, err := acquireToken(
		context.Background(), client,
		srv.URL, "secret", "user@example.com", "pass",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed (HTTP 503)")
	assert.NotContains(t, err.Error(), "service unavailable")
}

func TestAcquireToken_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	client := NewInsecureTLSClient(5 * time.Second)
	_, err := acquireToken(
		context.Background(), client,
		srv.URL, "secret", "user@example.com", "pass",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse token response")
}

func TestDoRequest_AuthorizationHeader(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	gw := &GatewayConnection{
		token:   "test-token-xyz",
		baseURL: srv.URL,
		client:  NewInsecureTLSClient(5 * time.Second),
	}

	_, err := gw.Get(context.Background(), "/test")
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token-xyz", capturedAuth)
}

func TestDoRequest_ContentTypeOnBody(t *testing.T) {
	var capturedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	gw := &GatewayConnection{
		token:   "test-token",
		baseURL: srv.URL,
		client:  NewInsecureTLSClient(5 * time.Second),
	}

	_, err := gw.Post(context.Background(), "/test", map[string]string{"key": "val"})
	require.NoError(t, err)
	assert.Equal(t, "application/json", capturedContentType)
}
