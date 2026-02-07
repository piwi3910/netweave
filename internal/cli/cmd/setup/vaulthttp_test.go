package setup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultAPIGet(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       map[string]interface{}
		token      string
		wantErr    bool
	}{
		{
			name:       "successful GET",
			statusCode: http.StatusOK,
			body:       map[string]interface{}{"initialized": true},
			token:      "test-token",
			wantErr:    false,
		},
		{
			name:       "successful GET without token",
			statusCode: http.StatusOK,
			body:       map[string]interface{}{"status": "ok"},
			token:      "",
			wantErr:    false,
		},
		{
			name:       "server error returns error",
			statusCode: http.StatusInternalServerError,
			body:       map[string]interface{}{"error": "internal"},
			token:      "test-token",
			wantErr:    true,
		},
		{
			name:       "forbidden returns error",
			statusCode: http.StatusForbidden,
			body:       map[string]interface{}{"error": "permission denied"},
			token:      "bad-token",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodGet, r.Method)
					assert.Equal(t, "/v1/sys/init", r.URL.Path)
					assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

					if tt.token != "" {
						assert.Equal(t, tt.token, r.Header.Get("X-Vault-Token"))
					}

					w.WriteHeader(tt.statusCode)
					data, _ := json.Marshal(tt.body)
					_, _ = w.Write(data)
				},
			))
			defer server.Close()

			result, err := vaultAPIGet(
				context.Background(), server.Client(),
				server.URL, "/v1/sys/init", tt.token,
			)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "vault API error")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
		})
	}
}

func TestVaultAPIPut(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		reqBody    interface{}
		respBody   map[string]interface{}
		wantErr    bool
	}{
		{
			name:       "successful PUT with body",
			statusCode: http.StatusOK,
			reqBody:    map[string]interface{}{"key": "value"},
			respBody:   map[string]interface{}{"result": "ok"},
			wantErr:    false,
		},
		{
			name:       "successful PUT with nil body",
			statusCode: http.StatusOK,
			reqBody:    nil,
			respBody:   map[string]interface{}{"result": "ok"},
			wantErr:    false,
		},
		{
			name:       "client error returns error",
			statusCode: http.StatusBadRequest,
			reqBody:    map[string]interface{}{},
			respBody:   map[string]interface{}{"error": "bad request"},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPut, r.Method)
					w.WriteHeader(tt.statusCode)
					data, _ := json.Marshal(tt.respBody)
					_, _ = w.Write(data)
				},
			))
			defer server.Close()

			result, err := vaultAPIPut(
				context.Background(), server.Client(),
				server.URL, "/v1/test", "token", tt.reqBody,
			)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
		})
	}
}

func TestVaultAPIGetRaw(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		body           map[string]interface{}
		wantStatusCode int
		wantErr        bool
	}{
		{
			name:           "healthy vault returns 200",
			statusCode:     http.StatusOK,
			body:           map[string]interface{}{"initialized": true, "sealed": false},
			wantStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "not initialized returns 501 without error",
			statusCode:     http.StatusNotImplemented,
			body:           map[string]interface{}{"initialized": false},
			wantStatusCode: http.StatusNotImplemented,
			wantErr:        false,
		},
		{
			name:           "sealed returns 503 without error",
			statusCode:     http.StatusServiceUnavailable,
			body:           map[string]interface{}{"initialized": true, "sealed": true},
			wantStatusCode: http.StatusServiceUnavailable,
			wantErr:        false,
		},
		{
			name:           "empty response body",
			statusCode:     http.StatusNoContent,
			body:           nil,
			wantStatusCode: http.StatusNoContent,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodGet, r.Method)

					w.WriteHeader(tt.statusCode)
					if tt.body != nil {
						data, _ := json.Marshal(tt.body)
						_, _ = w.Write(data)
					}
				},
			))
			defer server.Close()

			statusCode, result, err := vaultAPIGetRaw(
				context.Background(), server.Client(),
				server.URL, "/v1/sys/health", "",
			)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStatusCode, statusCode)

			if tt.body != nil {
				require.NotNil(t, result)
			}
		})
	}
}

func TestVaultAPIRequest_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := vaultAPIGet(ctx, server.Client(), server.URL, "/v1/test", "")
	require.Error(t, err)
}

func TestVaultAPIRequest_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
		},
	))
	defer server.Close()

	_, err := vaultAPIGet(
		context.Background(), server.Client(),
		server.URL, "/v1/test", "",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestVaultAPIGetRaw_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
		},
	))
	defer server.Close()

	_, _, err := vaultAPIGetRaw(
		context.Background(), server.Client(),
		server.URL, "/v1/test", "",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestVaultAPIRequest_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	result, err := vaultAPIGet(
		context.Background(), server.Client(),
		server.URL, "/v1/test", "",
	)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestVaultAPIRequest_TokenHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-Vault-Token")
			resp := map[string]interface{}{"token_present": token != ""}
			w.WriteHeader(http.StatusOK)
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
		},
	))
	defer server.Close()

	// With token.
	result, err := vaultAPIGet(
		context.Background(), server.Client(),
		server.URL, "/v1/test", "my-token",
	)
	require.NoError(t, err)
	assert.Equal(t, true, result["token_present"])

	// Without token.
	result, err = vaultAPIGet(
		context.Background(), server.Client(),
		server.URL, "/v1/test", "",
	)
	require.NoError(t, err)
	assert.Equal(t, false, result["token_present"])
}
