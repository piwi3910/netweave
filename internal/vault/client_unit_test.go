package vault

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := NewClient(&Config{
		Address:    server.URL,
		Token:      "test-token",
		PKIPath:    "pki_int",
		Timeout:    5 * time.Second,
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)
	return client
}

func TestNewClient_CustomHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 10 * time.Second}
	client, err := NewClient(&Config{
		Address:    "http://localhost:8200",
		Token:      "test",
		PKIPath:    "pki",
		HTTPClient: customClient,
	})
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	client, err := NewClient(&Config{
		Address: "http://localhost:8200",
		Token:   "test",
		PKIPath: "pki",
		Timeout: 0,
	})
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestClient_Close(t *testing.T) {
	client, err := NewClient(&Config{
		Address: "http://localhost:8200",
		Token:   "test",
		PKIPath: "pki",
	})
	require.NoError(t, err)
	err = client.Close()
	assert.NoError(t, err)
}

func TestClient_Ping_Healthy(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "initialized unsealed and active",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "unsealed and standby",
			statusCode: http.StatusTooManyRequests,
			wantErr:    false,
		},
		{
			name:       "DR secondary",
			statusCode: 472,
			wantErr:    false,
		},
		{
			name:       "performance standby",
			statusCode: 473,
			wantErr:    false,
		},
		{
			name:       "not initialized",
			statusCode: http.StatusNotImplemented,
			wantErr:    true,
		},
		{
			name:       "sealed",
			statusCode: http.StatusServiceUnavailable,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"initialized":true}`))
			})

			err := client.Ping(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "vault is not healthy")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_Ping_RequestError(t *testing.T) {
	client, err := NewClient(&Config{
		Address: "http://localhost:1",
		Token:   "test",
		PKIPath: "pki",
		Timeout: 100 * time.Millisecond,
	})
	require.NoError(t, err)

	err = client.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "health check failed")
}

func TestClient_GetCAChain_Success(t *testing.T) {
	expectedCA := "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----"
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/pki_int/ca_chain", r.URL.Path)
		assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedCA))
	})

	caChain, err := client.GetCAChain(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedCA, caChain)
}

func TestClient_GetCAChain_Error(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})

	_, err := client.GetCAChain(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get CA chain failed")
}

func TestClient_GetCAChain_RequestError(t *testing.T) {
	client, err := NewClient(&Config{
		Address: "http://localhost:1",
		Token:   "test",
		PKIPath: "pki",
		Timeout: 100 * time.Millisecond,
	})
	require.NoError(t, err)

	_, err = client.GetCAChain(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get CA chain")
}

func TestClient_GetCACertificate_Success(t *testing.T) {
	expectedCert := "-----BEGIN CERTIFICATE-----\nCACERT\n-----END CERTIFICATE-----"
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/pki_int/ca/pem", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedCert))
	})

	cert, err := client.GetCACertificate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedCert, cert)
}

func TestClient_GetCACertificate_Error(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	})

	_, err := client.GetCACertificate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get CA certificate failed")
}

func TestClient_GetCRL_Success(t *testing.T) {
	expectedCRL := "-----BEGIN X509 CRL-----\nCRL\n-----END X509 CRL-----"
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/pki_int/crl/pem", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedCRL))
	})

	crl, err := client.GetCRL(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedCRL, crl)
}

func TestClient_GetCRL_Error(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})

	_, err := client.GetCRL(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get CRL failed")
}

func TestParseVaultResponse_Success(t *testing.T) {
	body := `{"data":{"certificate":"test-cert","serial_number":"aa:bb"},"warnings":["warn1"]}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	vaultResp, err := parseVaultResponse(resp)
	require.NoError(t, err)
	assert.NotNil(t, vaultResp)
	assert.Equal(t, "test-cert", vaultResp.Data["certificate"])
	assert.Len(t, vaultResp.Warnings, 1)
}

func TestParseVaultResponse_VaultErrorWithJSON(t *testing.T) {
	body := `{"errors":["permission denied","token expired"]}`
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	_, err := parseVaultResponse(resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	assert.Contains(t, err.Error(), "token expired")
}

func TestParseVaultResponse_VaultErrorNonJSON(t *testing.T) {
	body := `not json`
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	_, err := parseVaultResponse(resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault request failed")
	assert.Contains(t, err.Error(), "not json")
}

func TestParseVaultResponse_InvalidJSON(t *testing.T) {
	body := `{invalid json}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	_, err := parseVaultResponse(resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode response")
}

func TestHelperFunctions(t *testing.T) {
	data := map[string]interface{}{
		"str_val":   "hello",
		"float_val": float64(42),
		"bool_val":  true,
		"int_val":   10, // not float64, should return default
	}

	t.Run("getString existing key", func(t *testing.T) {
		assert.Equal(t, "hello", getString(data, "str_val"))
	})

	t.Run("getString missing key", func(t *testing.T) {
		assert.Equal(t, "", getString(data, "missing"))
	})

	t.Run("getString wrong type", func(t *testing.T) {
		assert.Equal(t, "", getString(data, "float_val"))
	})

	t.Run("getInt existing key", func(t *testing.T) {
		assert.Equal(t, 42, getInt(data, "float_val"))
	})

	t.Run("getInt missing key", func(t *testing.T) {
		assert.Equal(t, 0, getInt(data, "missing"))
	})

	t.Run("getInt wrong type", func(t *testing.T) {
		assert.Equal(t, 0, getInt(data, "str_val"))
	})

	t.Run("getBool existing key", func(t *testing.T) {
		assert.True(t, getBool(data, "bool_val"))
	})

	t.Run("getBool missing key", func(t *testing.T) {
		assert.False(t, getBool(data, "missing"))
	})

	t.Run("getBool wrong type", func(t *testing.T) {
		assert.False(t, getBool(data, "str_val"))
	})
}

func TestClient_IssueCertificate_Unit(t *testing.T) {
	tests := []struct {
		name       string
		roleName   string
		req        *CertificateRequest
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
		errMsg     string
	}{
		{
			name:     "empty role name",
			roleName: "",
			req:      &CertificateRequest{CommonName: "test"},
			wantErr:  true,
			errMsg:   "roleName is required",
		},
		{
			name:     "nil request",
			roleName: "test-role",
			req:      nil,
			wantErr:  true,
			errMsg:   "request cannot be nil",
		},
		{
			name:     "empty common name",
			roleName: "test-role",
			req:      &CertificateRequest{CommonName: ""},
			wantErr:  true,
			errMsg:   "CommonName is required",
		},
		{
			name:     "successful issuance",
			roleName: "test-role",
			req: &CertificateRequest{
				CommonName:        "test.example.com",
				AltNames:          []string{"alt1.example.com", "alt2.example.com"},
				IPSANs:            []string{"192.168.1.1"},
				URISANs:           []string{"spiffe://example.com/test"},
				TTL:               "24h",
				Format:            "pem",
				PrivateKeyFormat:  "pkcs8",
				ExcludeCNFromSANs: true,
			},
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/pki_int/issue/test-role", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.Header().Set("Content-Type", "application/json")
				resp := Response{
					Data: map[string]interface{}{
						"certificate":      "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----",
						"private_key":      "-----BEGIN RSA PRIVATE KEY-----\nKEY\n-----END RSA PRIVATE KEY-----",
						"private_key_type": "rsa",
						"serial_number":    "aa:bb:cc",
						"issuing_ca":       "-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----",
						"ca_chain":         []interface{}{"ca1", "ca2"},
						"expiration":       float64(1700000000),
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name:     "default format",
			roleName: "test-role",
			req: &CertificateRequest{
				CommonName: "test.example.com",
			},
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := Response{
					Data: map[string]interface{}{
						"certificate":   "cert",
						"serial_number": "aa:bb",
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name:     "vault error response",
			roleName: "test-role",
			req: &CertificateRequest{
				CommonName: "test.example.com",
			},
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				resp := Response{
					Errors: []string{"role not found"},
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: true,
			errMsg:  "failed to parse response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.serverResp != nil {
					tt.serverResp(w, r)
				}
			})
			client := newTestServer(t, handler)

			cert, err := client.IssueCertificate(context.Background(), tt.roleName, tt.req)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, cert)
		})
	}
}

func TestClient_SignCSR_Unit(t *testing.T) {
	tests := []struct {
		name       string
		roleName   string
		csr        string
		ttl        string
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
		errMsg     string
	}{
		{
			name:    "empty role name",
			csr:     "csr-data",
			wantErr: true,
			errMsg:  "roleName is required",
		},
		{
			name:     "empty CSR",
			roleName: "test-role",
			csr:      "",
			wantErr:  true,
			errMsg:   "csr is required",
		},
		{
			name:     "successful sign with TTL",
			roleName: "test-role",
			csr:      "-----BEGIN CERTIFICATE REQUEST-----\nCSR\n-----END CERTIFICATE REQUEST-----",
			ttl:      "72h",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/pki_int/sign/test-role", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				resp := Response{
					Data: map[string]interface{}{
						"certificate":   "cert",
						"serial_number": "aa:bb",
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name:     "successful sign without TTL",
			roleName: "test-role",
			csr:      "csr-data",
			ttl:      "",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := Response{
					Data: map[string]interface{}{
						"certificate":   "cert",
						"serial_number": "aa:bb",
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.serverResp != nil {
					tt.serverResp(w, r)
				}
			})
			client := newTestServer(t, handler)

			cert, err := client.SignCSR(context.Background(), tt.roleName, tt.csr, tt.ttl)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, cert)
		})
	}
}

func TestClient_GetCertificate_Unit(t *testing.T) {
	tests := []struct {
		name         string
		serialNumber string
		serverResp   func(w http.ResponseWriter, r *http.Request)
		wantErr      bool
		errMsg       string
	}{
		{
			name:    "empty serial number",
			wantErr: true,
			errMsg:  "serialNumber is required",
		},
		{
			name:         "successful get",
			serialNumber: "aa:bb:cc",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/pki_int/cert/aa:bb:cc", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				resp := Response{
					Data: map[string]interface{}{
						"data": map[string]interface{}{
							"certificate": "-----BEGIN CERTIFICATE-----\nCERT\n-----END CERTIFICATE-----",
						},
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name:         "certificate not in response",
			serialNumber: "aa:bb:cc",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := Response{
					Data: map[string]interface{}{},
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: true,
			errMsg:  "certificate not found in response",
		},
		{
			name:         "vault error",
			serialNumber: "aa:bb:cc",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors":["not found"]}`))
			},
			wantErr: true,
			errMsg:  "failed to parse response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.serverResp != nil {
					tt.serverResp(w, r)
				}
			})
			client := newTestServer(t, handler)

			cert, err := client.GetCertificate(context.Background(), tt.serialNumber)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, cert)
		})
	}
}

func TestClient_ListCertificates_Unit(t *testing.T) {
	tests := []struct {
		name       string
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
		wantCount  int
	}{
		{
			name: "successful list with keys",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "LIST", r.Method)
				w.Header().Set("Content-Type", "application/json")
				resp := Response{
					Data: map[string]interface{}{
						"data": map[string]interface{}{
							"keys": []interface{}{"aa:bb", "cc:dd", "ee:ff"},
						},
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr:   false,
			wantCount: 3,
		},
		{
			name: "empty list no keys field",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := Response{
					Data: map[string]interface{}{},
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name: "server error",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":["internal error"]}`))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestServer(t, http.HandlerFunc(tt.serverResp))

			serials, err := client.ListCertificates(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, serials, tt.wantCount)
		})
	}
}

func TestClient_RevokeCertificate_Unit(t *testing.T) {
	tests := []struct {
		name         string
		serialNumber string
		serverResp   func(w http.ResponseWriter, r *http.Request)
		wantErr      bool
		errMsg       string
	}{
		{
			name:    "empty serial number",
			wantErr: true,
			errMsg:  "serialNumber is required",
		},
		{
			name:         "successful revocation",
			serialNumber: "aa:bb:cc",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/pki_int/revoke", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				w.Header().Set("Content-Type", "application/json")
				resp := Response{
					Data: map[string]interface{}{
						"data": map[string]interface{}{
							"revocation_time":         float64(1700000000),
							"revocation_time_rfc3339": "2023-11-14T22:13:20Z",
						},
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.serverResp != nil {
					tt.serverResp(w, r)
				}
			})
			client := newTestServer(t, handler)

			revResp, err := client.RevokeCertificate(context.Background(), tt.serialNumber)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, revResp)
		})
	}
}

func TestClient_RevokeCertificateBySerial_Unit(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := Response{
			Data: map[string]interface{}{
				"data": map[string]interface{}{
					"revocation_time": float64(1700000000),
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	err := client.RevokeCertificateBySerial(context.Background(), "aa:bb:cc")
	require.NoError(t, err)
}

func TestClient_TidyCertificates_Unit(t *testing.T) {
	tests := []struct {
		name       string
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
	}{
		{
			name: "successful tidy",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/pki_int/tidy", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(Response{Data: map[string]interface{}{}})
			},
			wantErr: false,
		},
		{
			name: "tidy error",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":["tidy failed"]}`))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestServer(t, http.HandlerFunc(tt.serverResp))

			err := client.TidyCertificates(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestClient_GetTidyStatus_Unit(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/pki_int/tidy-status", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		resp := Response{
			Data: map[string]interface{}{
				"data": map[string]interface{}{
					"state":      "Finished",
					"cert_store": float64(10),
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	status, err := client.GetTidyStatus(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, status)
}

func TestClient_RotateCRL_Unit(t *testing.T) {
	tests := []struct {
		name       string
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
	}{
		{
			name: "successful rotate",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/pki_int/crl/rotate", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(Response{Data: map[string]interface{}{"success": true}})
			},
			wantErr: false,
		},
		{
			name: "rotate error",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":["rotate failed"]}`))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestServer(t, http.HandlerFunc(tt.serverResp))

			err := client.RotateCRL(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBuildCertRequestData(t *testing.T) {
	tests := []struct {
		name     string
		req      *CertificateRequest
		expected map[string]interface{}
	}{
		{
			name: "minimal request",
			req: &CertificateRequest{
				CommonName: "test.example.com",
				Format:     "pem",
			},
			expected: map[string]interface{}{
				"common_name": "test.example.com",
				"format":      "pem",
			},
		},
		{
			name: "full request",
			req: &CertificateRequest{
				CommonName:        "test.example.com",
				Format:            "pem",
				TTL:               "24h",
				PrivateKeyFormat:  "pkcs8",
				ExcludeCNFromSANs: true,
				AltNames:          []string{"a.com", "b.com"},
				IPSANs:            []string{"1.2.3.4"},
				URISANs:           []string{"spiffe://example"},
			},
			expected: map[string]interface{}{
				"common_name":          "test.example.com",
				"format":               "pem",
				"ttl":                  "24h",
				"private_key_format":   "pkcs8",
				"exclude_cn_from_sans": true,
				"alt_names":            "a.com,b.com",
				"ip_sans":              "1.2.3.4",
				"uri_sans":             "spiffe://example",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCertRequestData(tt.req)
			for k, v := range tt.expected {
				assert.Equal(t, v, result[k], "key %s mismatch", k)
			}
		})
	}
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"empty", []string{}, ""},
		{"single", []string{"a"}, "a"},
		{"multiple", []string{"a", "b", "c"}, "a,b,c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, joinStrings(tt.input))
		})
	}
}

func TestParseCertificateData(t *testing.T) {
	tests := []struct {
		name              string
		resp              *Response
		includePrivateKey bool
		wantCert          string
		wantKey           string
	}{
		{
			name: "with private key nested data",
			resp: &Response{
				Data: map[string]interface{}{
					"data": map[string]interface{}{
						"certificate":      "cert-pem",
						"private_key":      "key-pem",
						"private_key_type": "rsa",
						"serial_number":    "aa:bb",
						"issuing_ca":       "ca-pem",
						"ca_chain":         []interface{}{"ca1", "ca2"},
						"expiration":       float64(1700000000),
					},
					"lease_id":       "lease-123",
					"lease_duration": float64(3600),
					"renewable":      true,
				},
			},
			includePrivateKey: true,
			wantCert:          "cert-pem",
			wantKey:           "key-pem",
		},
		{
			name: "without private key",
			resp: &Response{
				Data: map[string]interface{}{
					"certificate":   "cert-pem",
					"serial_number": "aa:bb",
				},
			},
			includePrivateKey: false,
			wantCert:          "cert-pem",
			wantKey:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := parseCertificateData(tt.resp, tt.includePrivateKey)
			assert.Equal(t, tt.wantCert, cert.Certificate)
			assert.Equal(t, tt.wantKey, cert.PrivateKey)
		})
	}
}
