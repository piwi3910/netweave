package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// vaultAPIGet makes an authenticated GET request to the Vault API.
func vaultAPIGet(
	ctx context.Context,
	client *http.Client,
	vaultAddr, path, token string,
) (map[string]interface{}, error) {
	return vaultAPIRequest(ctx, client, http.MethodGet, vaultAddr+path, token, nil)
}

// vaultAPIPut makes an authenticated PUT request to the Vault API.
func vaultAPIPut(
	ctx context.Context,
	client *http.Client,
	vaultAddr, path, token string,
	body interface{},
) (map[string]interface{}, error) {
	return vaultAPIRequest(ctx, client, http.MethodPut, vaultAddr+path, token, body)
}

func vaultAPIRequest(
	ctx context.Context,
	client *http.Client,
	method, url, token string,
	body interface{},
) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf(
			"vault API error (status %d): %s", resp.StatusCode, string(respBody),
		)
	}

	var result map[string]interface{}
	if len(respBody) > 0 {
		if jsonErr := json.Unmarshal(respBody, &result); jsonErr != nil {
			return nil, fmt.Errorf("failed to parse response: %w", jsonErr)
		}
	}

	return result, nil
}

// vaultAPIGetRaw makes a GET request to the Vault API and returns the raw
// HTTP status code along with the parsed body. Unlike vaultAPIGet it does
// not treat 4xx/5xx as errors, which is useful for endpoints like
// /v1/sys/health where non-200 codes carry meaningful state (e.g. 501 =
// not initialized, 503 = sealed).
func vaultAPIGetRaw(
	ctx context.Context,
	client *http.Client,
	vaultAddr, path, token string,
) (int, map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, vaultAddr+path, nil,
	)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]interface{}
	if len(respBody) > 0 {
		if jsonErr := json.Unmarshal(respBody, &result); jsonErr != nil {
			return resp.StatusCode, nil, fmt.Errorf("failed to parse response: %w", jsonErr)
		}
	}

	return resp.StatusCode, result, nil
}
