package encryption_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/piwi3910/netweave/internal/encryption"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockVaultServer creates a test HTTP server that simulates Vault transit API.
func newMockVaultServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/transit/encrypt/test-key", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token := r.Header.Get("X-Vault-Token")
		if token != "test-token" {
			w.WriteHeader(http.StatusForbidden)
			resp := map[string]interface{}{"errors": []string{"permission denied"}}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		var req struct {
			Plaintext string `json:"plaintext"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Simulate Vault: prefix with "vault:v1:" and keep the base64 data.
		ciphertext := "vault:v1:" + req.Plaintext

		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"ciphertext": ciphertext,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/v1/transit/decrypt/test-key", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token := r.Header.Get("X-Vault-Token")
		if token != "test-token" {
			w.WriteHeader(http.StatusForbidden)
			resp := map[string]interface{}{"errors": []string{"permission denied"}}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		var req struct {
			Ciphertext string `json:"ciphertext"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Reverse the mock encryption: strip "vault:v1:" prefix.
		plaintext := strings.TrimPrefix(req.Ciphertext, "vault:v1:")

		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"plaintext": plaintext,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestVaultEncryptor_RoundTrip(t *testing.T) {
	t.Parallel()

	server := newMockVaultServer(t)

	enc := encryption.NewVaultEncryptor(encryption.VaultConfig{
		Address:   server.URL,
		Token:     "test-token",
		MountPath: "transit",
		KeyName:   "test-key",
	})

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{
			name:      "simple text",
			plaintext: []byte("hello vault"),
		},
		{
			name:      "json credentials",
			plaintext: []byte(`{"username":"admin","password":"s3cr3t"}`),
		},
		{
			name:      "empty data",
			plaintext: []byte(""),
		},
		{
			name:      "binary data",
			plaintext: []byte{0x00, 0x01, 0xff},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ciphertext, err := enc.Encrypt(tt.plaintext)
			require.NoError(t, err)
			assert.NotEmpty(t, ciphertext)
			assert.True(t, strings.HasPrefix(string(ciphertext), "vault:v1:"))

			decrypted, err := enc.Decrypt(ciphertext)
			require.NoError(t, err)
			assert.Equal(t, string(tt.plaintext), string(decrypted))
		})
	}
}

func TestVaultEncryptor_InvalidToken(t *testing.T) {
	t.Parallel()

	server := newMockVaultServer(t)

	enc := encryption.NewVaultEncryptor(encryption.VaultConfig{
		Address:   server.URL,
		Token:     "wrong-token",
		MountPath: "transit",
		KeyName:   "test-key",
	})

	_, err := enc.Encrypt([]byte("secret"))
	require.Error(t, err)
	assert.ErrorIs(t, err, encryption.ErrVaultEncryptFailed)
}

func TestVaultEncryptor_ServerUnavailable(t *testing.T) {
	t.Parallel()

	enc := encryption.NewVaultEncryptor(encryption.VaultConfig{
		Address:   "http://127.0.0.1:1",
		Token:     "token",
		MountPath: "transit",
		KeyName:   "key",
	})

	_, err := enc.Encrypt([]byte("data"))
	require.Error(t, err)
	assert.ErrorIs(t, err, encryption.ErrVaultEncryptFailed)
}

func TestVaultEncryptor_DefaultMountPath(t *testing.T) {
	t.Parallel()

	server := newMockVaultServer(t)

	// Do not set MountPath; it should default to "transit".
	enc := encryption.NewVaultEncryptor(encryption.VaultConfig{
		Address: server.URL,
		Token:   "test-token",
		KeyName: "test-key",
	})

	ciphertext, err := enc.Encrypt([]byte("test"))
	require.NoError(t, err)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, []byte("test"), decrypted)
}

func TestVaultEncryptor_EncryptReturnsBase64(t *testing.T) {
	t.Parallel()

	server := newMockVaultServer(t)

	enc := encryption.NewVaultEncryptor(encryption.VaultConfig{
		Address:   server.URL,
		Token:     "test-token",
		MountPath: "transit",
		KeyName:   "test-key",
	})

	plaintext := []byte("hello")
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	// The mock server returns "vault:v1:" + base64(plaintext).
	// Verify the base64 portion decodes correctly.
	b64Part := strings.TrimPrefix(string(ciphertext), "vault:v1:")
	decoded, err := base64.StdEncoding.DecodeString(b64Part)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decoded)
}

func TestVaultEncryptor_DecryptNilData(t *testing.T) {
	t.Parallel()

	// Create a server that returns null data for decrypt.
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/transit/decrypt/test-key", func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"data": nil,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	enc := encryption.NewVaultEncryptor(encryption.VaultConfig{
		Address:   server.URL,
		Token:     "test-token",
		MountPath: "transit",
		KeyName:   "test-key",
	})

	_, err := enc.Decrypt([]byte("vault:v1:invalid"))
	require.Error(t, err)
	assert.ErrorIs(t, err, encryption.ErrVaultDecryptFailed)
}

func TestVaultEncryptor_EncryptNilData(t *testing.T) {
	t.Parallel()

	// Create a server that returns null data for encrypt.
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/transit/encrypt/test-key", func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"data": nil,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	enc := encryption.NewVaultEncryptor(encryption.VaultConfig{
		Address:   server.URL,
		Token:     "test-token",
		MountPath: "transit",
		KeyName:   "test-key",
	})

	_, err := enc.Encrypt([]byte("data"))
	require.Error(t, err)
	assert.ErrorIs(t, err, encryption.ErrVaultEncryptFailed)
}
