package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// User represents a Keycloak user.
type User struct {
	ID               string              `json:"id,omitempty"`
	Username         string              `json:"username"`
	Email            string              `json:"email,omitempty"`
	EmailVerified    bool                `json:"emailVerified"`
	FirstName        string              `json:"firstName,omitempty"`
	LastName         string              `json:"lastName,omitempty"`
	Enabled          bool                `json:"enabled"`
	Attributes       map[string][]string `json:"attributes,omitempty"`
	CreatedTimestamp int64               `json:"createdTimestamp,omitempty"`
}

// UserCredential represents a user's password credential.
type UserCredential struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

// CreateUser creates a new user in Keycloak.
// Returns the created user's ID.
func (c *Client) CreateUser(ctx context.Context, user *User) (string, error) {
	if user.Username == "" {
		return "", fmt.Errorf("username is required")
	}

	body, err := json.Marshal(user)
	if err != nil {
		return "", fmt.Errorf("failed to marshal user: %w", err)
	}

	resp, err := c.doAdminRequest(ctx, http.MethodPost, "/users", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create user request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusConflict {
		return "", ErrUserExists
	}

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create user failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no location header in response")
	}

	// Extract user ID from location header (last segment after final /)
	// Keycloak returns URLs like: /admin/realms/test/users/{userID}
	lastSlash := strings.LastIndex(location, "/")
	if lastSlash == -1 || lastSlash == len(location)-1 {
		return "", fmt.Errorf("invalid location header format: %s", location)
	}

	userID := location[lastSlash+1:]
	if userID == "" {
		return "", fmt.Errorf("empty user ID in location header: %s", location)
	}

	return userID, nil
}

// GetUser retrieves a user by ID.
func (c *Client) GetUser(ctx context.Context, userID string) (*User, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}

	path := fmt.Sprintf("/users/%s", userID)
	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get user request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrUserNotFound
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get user failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode user: %w", err)
	}

	return &user, nil
}

// GetUserByUsername retrieves a user by username.
func (c *Client) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}

	params := url.Values{}
	params.Set("username", username)
	params.Set("exact", "true")

	path := fmt.Sprintf("/users?%s", params.Encode())
	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get user by username request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get user by username failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode users: %w", err)
	}

	if len(users) == 0 {
		return nil, ErrUserNotFound
	}

	return &users[0], nil
}

// UpdateUser updates an existing user.
func (c *Client) UpdateUser(ctx context.Context, user *User) error {
	if user.ID == "" {
		return fmt.Errorf("user ID is required")
	}

	body, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	path := fmt.Sprintf("/users/%s", user.ID)
	resp, err := c.doAdminRequest(ctx, http.MethodPut, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("update user request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update user failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteUser deletes a user by ID.
func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}

	path := fmt.Sprintf("/users/%s", userID)
	resp, err := c.doAdminRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("delete user request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete user failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ListUsers retrieves all users.
// maxResults specifies the maximum number of users to return (0 for all).
func (c *Client) ListUsers(ctx context.Context, maxResults int) ([]User, error) {
	params := url.Values{}
	params.Set("briefRepresentation", "false")
	if maxResults > 0 {
		params.Set("max", fmt.Sprintf("%d", maxResults))
	}

	path := fmt.Sprintf("/users?%s", params.Encode())

	resp, err := c.doAdminRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("list users request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list users failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode users: %w", err)
	}

	return users, nil
}

// SetUserPassword sets a user's password.
func (c *Client) SetUserPassword(ctx context.Context, userID, password string, temporary bool) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}

	credential := UserCredential{
		Type:      "password",
		Value:     password,
		Temporary: temporary,
	}

	body, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("failed to marshal credential: %w", err)
	}

	path := fmt.Sprintf("/users/%s/reset-password", userID)
	resp, err := c.doAdminRequest(ctx, http.MethodPut, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("set password request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("set password failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GetUserAttribute retrieves a specific attribute value for a user.
// Returns the first value if the attribute has multiple values.
func (c *Client) GetUserAttribute(ctx context.Context, userID, attributeName string) (string, error) {
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return "", err
	}

	if user.Attributes == nil {
		return "", ErrAttributeNotFound
	}

	values, ok := user.Attributes[attributeName]
	if !ok || len(values) == 0 {
		return "", ErrAttributeNotFound
	}

	return values[0], nil
}

// SetUserAttribute sets an attribute for a user.
func (c *Client) SetUserAttribute(ctx context.Context, userID, attributeName, attributeValue string) error {
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return err
	}

	if user.Attributes == nil {
		user.Attributes = make(map[string][]string)
	}

	user.Attributes[attributeName] = []string{attributeValue}

	return c.UpdateUser(ctx, user)
}
