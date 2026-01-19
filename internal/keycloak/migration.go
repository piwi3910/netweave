package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// RealmExists checks if a realm exists in Keycloak.
func (c *Client) RealmExists(ctx context.Context, realmName string) (bool, error) {
	if realmName == "" {
		return false, fmt.Errorf("realm name is required")
	}

	// For now, we check if tenant attributes exist in the main realm
	// In production, you might want to check if a separate realm exists
	return c.TenantAttributeExists(ctx, realmName)
}

// TenantAttributeExists checks if tenant attributes exist in Keycloak.
func (c *Client) TenantAttributeExists(ctx context.Context, tenantID string) (bool, error) {
	// Check if any user has this tenantId attribute
	params := url.Values{}
	params.Set("q", fmt.Sprintf("tenantId:%s", tenantID))
	params.Set("max", "1")

	path := fmt.Sprintf("/users?%s", params.Encode())
	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return false, fmt.Errorf("check tenant attribute request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("check tenant attribute failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return false, fmt.Errorf("failed to decode users: %w", err)
	}

	return len(users) > 0, nil
}

// SetTenantAttribute stores tenant metadata as realm attributes.
// For simplicity, we store tenant metadata in realm-level attributes.
// In production, you might create separate realms per tenant.
func (c *Client) SetTenantAttribute(ctx context.Context, tenantID string, attributes map[string]interface{}) error {
	if tenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}

	// Get current realm
	resp, err := c.doAdminRequest(ctx, http.MethodGet, fmt.Sprintf("/realms/%s", c.config.Realm), nil)
	if err != nil {
		return fmt.Errorf("get realm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("get realm failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var realm map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&realm); err != nil {
		return fmt.Errorf("failed to decode realm: %w", err)
	}

	// Get or create attributes map
	realmAttributes, ok := realm["attributes"].(map[string]interface{})
	if !ok {
		realmAttributes = make(map[string]interface{})
	}

	// Store tenant metadata under tenants:{tenantID}
	tenantKey := fmt.Sprintf("tenant:%s", tenantID)
	tenantData, err := json.Marshal(attributes)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant attributes: %w", err)
	}
	realmAttributes[tenantKey] = string(tenantData)
	realm["attributes"] = realmAttributes

	// Update realm
	body, err := json.Marshal(realm)
	if err != nil {
		return fmt.Errorf("failed to marshal realm: %w", err)
	}

	resp2, err := c.doAdminRequest(ctx, http.MethodPut, fmt.Sprintf("/realms/%s", c.config.Realm), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("update realm request failed: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp2.Body)
		return fmt.Errorf("update realm failed: status=%d, body=%s", resp2.StatusCode, string(respBody))
	}

	return nil
}

// RoleExists checks if a role exists in Keycloak.
func (c *Client) RoleExists(ctx context.Context, roleName string) (bool, error) {
	if roleName == "" {
		return false, fmt.Errorf("role name is required")
	}

	_, err := c.GetRealmRole(ctx, roleName)
	if err != nil {
		if err.Error() == "role not found" {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// CreateRole creates a realm role (wrapper for CreateRealmRole).
func (c *Client) CreateRole(ctx context.Context, role *Role) error {
	if role.ClientRole {
		// For client roles, we need the client internal ID
		// For simplicity, create as realm role with attribute marking it as client role
		return c.CreateRealmRole(ctx, role)
	}
	return c.CreateRealmRole(ctx, role)
}

// UserExistsByEmail checks if a user with the given email exists.
func (c *Client) UserExistsByEmail(ctx context.Context, email string) (bool, error) {
	if email == "" {
		return false, fmt.Errorf("email is required")
	}

	params := url.Values{}
	params.Set("email", email)
	params.Set("exact", "true")

	path := fmt.Sprintf("/users?%s", params.Encode())
	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return false, fmt.Errorf("check user by email request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("check user by email failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return false, fmt.Errorf("failed to decode users: %w", err)
	}

	return len(users) > 0, nil
}

// UserExistsByUsername checks if a user with the given username exists.
func (c *Client) UserExistsByUsername(ctx context.Context, username string) (bool, error) {
	if username == "" {
		return false, fmt.Errorf("username is required")
	}

	_, err := c.GetUserByUsername(ctx, username)
	if err != nil {
		if err.Error() == "user not found" {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// AssignRoleToUser assigns a role to a user by email and role name.
func (c *Client) AssignRoleToUser(ctx context.Context, email, roleName string) error {
	// Get user by email
	user, err := c.getUserByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("failed to get user by email: %w", err)
	}

	// Get role
	role, err := c.GetRealmRole(ctx, roleName)
	if err != nil {
		return fmt.Errorf("failed to get role: %w", err)
	}

	// Assign role to user
	return c.AddRealmRolesToUser(ctx, user.ID, []Role{*role})
}

// getUserByEmail is a helper to get a user by email.
func (c *Client) getUserByEmail(ctx context.Context, email string) (*User, error) {
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	params := url.Values{}
	params.Set("email", email)
	params.Set("exact", "true")

	path := fmt.Sprintf("/users?%s", params.Encode())
	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get user by email request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get user by email failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode users: %w", err)
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &users[0], nil
}
