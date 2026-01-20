package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Realm represents a Keycloak realm.
type Realm struct {
	ID          string            `json:"id,omitempty"`
	Realm       string            `json:"realm,omitempty"`
	DisplayName string            `json:"displayName,omitempty"`
	Enabled     bool              `json:"enabled"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// GetRealm retrieves realm details.
func (c *Client) GetRealm(ctx context.Context) (*Realm, error) {
	path := ""
	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get realm request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("realm not found")
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get realm failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var realm Realm
	if err := json.NewDecoder(resp.Body).Decode(&realm); err != nil {
		return nil, fmt.Errorf("failed to decode realm: %w", err)
	}

	return &realm, nil
}

// UpdateRealm updates realm attributes.
func (c *Client) UpdateRealm(ctx context.Context, attributes map[string]string) error {
	// Prepare update payload
	payload := map[string]interface{}{
		"attributes": attributes,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal realm update: %w", err)
	}

	path := ""
	resp, err := c.doAdminRequest(ctx, http.MethodPut, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("update realm request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("realm not found")
	}

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update realm failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}
