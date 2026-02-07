package encryption

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Vault transit errors.
var (
	// ErrVaultUnavailable is returned when the Vault server is unreachable.
	ErrVaultUnavailable = errors.New("vault server unavailable")

	// ErrVaultEncryptFailed is returned when the Vault transit encrypt operation fails.
	ErrVaultEncryptFailed = errors.New("vault encrypt failed")

	// ErrVaultDecryptFailed is returned when the Vault transit decrypt operation fails.
	ErrVaultDecryptFailed = errors.New("vault decrypt failed")

	// ErrVaultInvalidResponse is returned when the Vault API returns an unexpected response.
	ErrVaultInvalidResponse = errors.New("vault returned invalid response")
)

// VaultConfig holds configuration for connecting to HashiCorp Vault's transit engine.
type VaultConfig struct {
	// Address is the Vault server URL (e.g. "https://vault.example.com:8200").
	Address string

	// Token is the Vault authentication token.
	Token string

	// MountPath is the transit secrets engine mount path (default: "transit").
	MountPath string

	// KeyName is the name of the transit encryption key.
	KeyName string
}

// VaultEncryptor implements Encryptor using HashiCorp Vault's transit secrets engine.
type VaultEncryptor struct {
	cfg    VaultConfig
	client *http.Client
}

// NewVaultEncryptor creates a new VaultEncryptor with the given configuration.
func NewVaultEncryptor(cfg VaultConfig) *VaultEncryptor {
	if cfg.MountPath == "" {
		cfg.MountPath = "transit"
	}

	return &VaultEncryptor{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// vaultEncryptRequest is the request body for Vault transit encrypt.
type vaultEncryptRequest struct {
	Plaintext string `json:"plaintext"`
}

// vaultDecryptRequest is the request body for Vault transit decrypt.
type vaultDecryptRequest struct {
	Ciphertext string `json:"ciphertext"`
}

// vaultResponse is the common Vault API response envelope.
type vaultResponse struct {
	Data   *vaultResponseData `json:"data"`
	Errors []string           `json:"errors"`
}

// vaultResponseData holds the data portion of a Vault transit response.
type vaultResponseData struct {
	Ciphertext string `json:"ciphertext"`
	Plaintext  string `json:"plaintext"`
}

// Encrypt encrypts the plaintext using Vault's transit secrets engine.
func (v *VaultEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString(plaintext)
	reqBody := vaultEncryptRequest{Plaintext: encoded}

	url := fmt.Sprintf("%s/v1/%s/encrypt/%s", v.cfg.Address, v.cfg.MountPath, v.cfg.KeyName)
	resp, err := v.doRequest(context.Background(), http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrVaultEncryptFailed, err)
	}

	if resp.Data == nil || resp.Data.Ciphertext == "" {
		return nil, fmt.Errorf("%w: missing ciphertext in response", ErrVaultEncryptFailed)
	}

	return []byte(resp.Data.Ciphertext), nil
}

// Decrypt decrypts the ciphertext using Vault's transit secrets engine.
func (v *VaultEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	reqBody := vaultDecryptRequest{Ciphertext: string(ciphertext)}

	url := fmt.Sprintf("%s/v1/%s/decrypt/%s", v.cfg.Address, v.cfg.MountPath, v.cfg.KeyName)
	resp, err := v.doRequest(context.Background(), http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrVaultDecryptFailed, err)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("%w: missing data in response", ErrVaultDecryptFailed)
	}

	decoded, err := base64.StdEncoding.DecodeString(resp.Data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode base64 plaintext: %w", ErrVaultDecryptFailed, err)
	}

	return decoded, nil
}

// doRequest sends an HTTP request to the Vault API and decodes the response.
func (v *VaultEncryptor) doRequest(ctx context.Context, method, url string, body interface{}) (*vaultResponse, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrVaultUnavailable, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var vaultResp vaultResponse
	if err := json.Unmarshal(respBody, &vaultResp); err != nil {
		return nil, fmt.Errorf("failed to decode vault response: %w", err)
	}

	if len(vaultResp.Errors) > 0 {
		return nil, fmt.Errorf("vault errors: %v", vaultResp.Errors)
	}

	return &vaultResp, nil
}
