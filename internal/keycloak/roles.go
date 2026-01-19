package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Role represents a Keycloak role.
type Role struct {
	ID          string              `json:"id,omitempty"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Composite   bool                `json:"composite"`
	ClientRole  bool                `json:"clientRole"`
	ContainerID string              `json:"containerId,omitempty"`
	Attributes  map[string][]string `json:"attributes,omitempty"`
}

// RoleMapping represents roles assigned to a user.
type RoleMapping struct {
	RealmMappings  []Role            `json:"realmMappings,omitempty"`
	ClientMappings map[string][]Role `json:"clientMappings,omitempty"`
}

// CreateRealmRole creates a new realm-level role.
func (c *Client) CreateRealmRole(ctx context.Context, role *Role) error {
	if role.Name == "" {
		return fmt.Errorf("role name is required")
	}

	body, err := json.Marshal(role)
	if err != nil {
		return fmt.Errorf("failed to marshal role: %w", err)
	}

	resp, err := c.doAdminRequest(ctx, http.MethodPost, "/roles", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create role request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("role already exists")
	}

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create role failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GetRealmRole retrieves a realm role by name.
func (c *Client) GetRealmRole(ctx context.Context, roleName string) (*Role, error) {
	if roleName == "" {
		return nil, fmt.Errorf("role name is required")
	}

	path := fmt.Sprintf("/roles/%s", roleName)
	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get role request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("role not found")
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get role failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var role Role
	if err := json.NewDecoder(resp.Body).Decode(&role); err != nil {
		return nil, fmt.Errorf("failed to decode role: %w", err)
	}

	return &role, nil
}

// UpdateRealmRole updates an existing realm role.
func (c *Client) UpdateRealmRole(ctx context.Context, roleName string, role *Role) error {
	if roleName == "" {
		return fmt.Errorf("role name is required")
	}

	body, err := json.Marshal(role)
	if err != nil {
		return fmt.Errorf("failed to marshal role: %w", err)
	}

	path := fmt.Sprintf("/roles/%s", roleName)
	resp, err := c.doAdminRequest(ctx, http.MethodPut, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("update role request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("role not found")
	}

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update role failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteRealmRole deletes a realm role by name.
func (c *Client) DeleteRealmRole(ctx context.Context, roleName string) error {
	if roleName == "" {
		return fmt.Errorf("role name is required")
	}

	path := fmt.Sprintf("/roles/%s", roleName)
	resp, err := c.doAdminRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("delete role request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("role not found")
	}

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete role failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ListRealmRoles retrieves all realm roles.
func (c *Client) ListRealmRoles(ctx context.Context) ([]Role, error) {
	resp, err := c.doAdminRequest(ctx, http.MethodGet, "/roles", nil)
	if err != nil {
		return nil, fmt.Errorf("list roles request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list roles failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var roles []Role
	if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
		return nil, fmt.Errorf("failed to decode roles: %w", err)
	}

	return roles, nil
}

// CreateClientRole creates a new client-level role.
func (c *Client) CreateClientRole(ctx context.Context, clientID string, role *Role) error {
	if clientID == "" {
		return fmt.Errorf("clientID is required")
	}
	if role.Name == "" {
		return fmt.Errorf("role name is required")
	}

	body, err := json.Marshal(role)
	if err != nil {
		return fmt.Errorf("failed to marshal role: %w", err)
	}

	path := fmt.Sprintf("/clients/%s/roles", clientID)
	resp, err := c.doAdminRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create client role request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("client role already exists")
	}

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create client role failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GetClientRole retrieves a client role by name.
func (c *Client) GetClientRole(ctx context.Context, clientID, roleName string) (*Role, error) {
	if clientID == "" {
		return nil, fmt.Errorf("clientID is required")
	}
	if roleName == "" {
		return nil, fmt.Errorf("role name is required")
	}

	path := fmt.Sprintf("/clients/%s/roles/%s", clientID, roleName)
	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get client role request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("client role not found")
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get client role failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var role Role
	if err := json.NewDecoder(resp.Body).Decode(&role); err != nil {
		return nil, fmt.Errorf("failed to decode role: %w", err)
	}

	return &role, nil
}

// ListClientRoles retrieves all roles for a client.
func (c *Client) ListClientRoles(ctx context.Context, clientID string) ([]Role, error) {
	if clientID == "" {
		return nil, fmt.Errorf("clientID is required")
	}

	path := fmt.Sprintf("/clients/%s/roles", clientID)
	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("list client roles request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list client roles failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var roles []Role
	if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
		return nil, fmt.Errorf("failed to decode roles: %w", err)
	}

	return roles, nil
}

// GetUserRoleMappings retrieves all roles assigned to a user.
func (c *Client) GetUserRoleMappings(ctx context.Context, userID string) (*RoleMapping, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}

	path := fmt.Sprintf("/users/%s/role-mappings", userID)
	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get user role mappings request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("user not found")
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get user role mappings failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var roleMapping RoleMapping
	if err := json.NewDecoder(resp.Body).Decode(&roleMapping); err != nil {
		return nil, fmt.Errorf("failed to decode role mappings: %w", err)
	}

	return &roleMapping, nil
}

// AddRealmRolesToUser assigns realm roles to a user.
func (c *Client) AddRealmRolesToUser(ctx context.Context, userID string, roles []Role) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	if len(roles) == 0 {
		return fmt.Errorf("at least one role is required")
	}

	body, err := json.Marshal(roles)
	if err != nil {
		return fmt.Errorf("failed to marshal roles: %w", err)
	}

	path := fmt.Sprintf("/users/%s/role-mappings/realm", userID)
	resp, err := c.doAdminRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("add realm roles request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("user not found")
	}

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add realm roles failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// RemoveRealmRolesFromUser removes realm roles from a user.
func (c *Client) RemoveRealmRolesFromUser(ctx context.Context, userID string, roles []Role) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	if len(roles) == 0 {
		return fmt.Errorf("at least one role is required")
	}

	body, err := json.Marshal(roles)
	if err != nil {
		return fmt.Errorf("failed to marshal roles: %w", err)
	}

	path := fmt.Sprintf("/users/%s/role-mappings/realm", userID)
	resp, err := c.doAdminRequest(ctx, http.MethodDelete, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("remove realm roles request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("user not found")
	}

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove realm roles failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// AddClientRolesToUser assigns client roles to a user.
func (c *Client) AddClientRolesToUser(ctx context.Context, userID, clientID string, roles []Role) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	if clientID == "" {
		return fmt.Errorf("clientID is required")
	}
	if len(roles) == 0 {
		return fmt.Errorf("at least one role is required")
	}

	body, err := json.Marshal(roles)
	if err != nil {
		return fmt.Errorf("failed to marshal roles: %w", err)
	}

	path := fmt.Sprintf("/users/%s/role-mappings/clients/%s", userID, clientID)
	resp, err := c.doAdminRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("add client roles request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("user or client not found")
	}

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add client roles failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// RemoveClientRolesFromUser removes client roles from a user.
func (c *Client) RemoveClientRolesFromUser(ctx context.Context, userID, clientID string, roles []Role) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	if clientID == "" {
		return fmt.Errorf("clientID is required")
	}
	if len(roles) == 0 {
		return fmt.Errorf("at least one role is required")
	}

	body, err := json.Marshal(roles)
	if err != nil {
		return fmt.Errorf("failed to marshal roles: %w", err)
	}

	path := fmt.Sprintf("/users/%s/role-mappings/clients/%s", userID, clientID)
	resp, err := c.doAdminRequest(ctx, http.MethodDelete, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("remove client roles request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("user or client not found")
	}

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove client roles failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GetClientID retrieves the internal Keycloak ID for a client by client ID (e.g., "netweave-gateway").
func (c *Client) GetClientID(ctx context.Context, clientID string) (string, error) {
	if clientID == "" {
		return "", fmt.Errorf("clientID is required")
	}

	resp, err := c.doAdminRequest(ctx, http.MethodGet, "/clients", nil)
	if err != nil {
		return "", fmt.Errorf("list clients request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("list clients failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var clients []struct {
		ID       string `json:"id"`
		ClientID string `json:"clientId"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&clients); err != nil {
		return "", fmt.Errorf("failed to decode clients: %w", err)
	}

	for _, client := range clients {
		if client.ClientID == clientID {
			return client.ID, nil
		}
	}

	return "", fmt.Errorf("client not found")
}
