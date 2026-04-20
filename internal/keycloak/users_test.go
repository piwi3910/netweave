package keycloak

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestClient(t *testing.T) *Client {
	t.Helper()

	config := &Config{
		BaseURL:       "http://localhost:8090",
		Realm:         "netweave",
		ClientID:      "netweave-gateway",
		ClientSecret:  "netweave-gateway-secret-change-in-production",
		AdminUsername: "admin",
		AdminPassword: "admin",
		Timeout:       10 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	return client
}

func TestClient_CreateUser(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	user := &User{
		Username:      fmt.Sprintf("testuser-%d", timestamp),
		Email:         fmt.Sprintf("testuser-%d@example.com", timestamp),
		EmailVerified: true,
		FirstName:     "Test",
		LastName:      "User",
		Enabled:       true,
		Attributes: map[string][]string{
			"tenantId": {"test-tenant"},
		},
	}

	userID, err := client.CreateUser(ctx, user)
	require.NoError(t, err)
	assert.NotEmpty(t, userID)

	t.Cleanup(func() {
		if err := client.DeleteUser(context.Background(), userID); err != nil {
			t.Logf("Failed to cleanup test user: %v", err)
		}
	})

	retrievedUser, err := client.GetUser(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, user.Email, retrievedUser.Email)
	assert.Equal(t, user.FirstName, retrievedUser.FirstName)
	assert.Equal(t, user.LastName, retrievedUser.LastName)
	assert.True(t, retrievedUser.Enabled)
}

func TestClient_GetUser(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()

	user, err := client.GetUserByUsername(ctx, "operator@example.com")
	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "operator@example.com", user.Email)
	assert.True(t, user.Enabled)

	retrievedUser, err := client.GetUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrievedUser.ID)
	assert.Equal(t, user.Email, retrievedUser.Email)
}

func TestClient_GetUserByUsername(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{
			name:     "existing user - operator",
			username: "operator@example.com",
			wantErr:  false,
		},
		{
			name:     "non-existent user with valid email format",
			username: "admin@example.com",
			wantErr:  true,
		},
		{
			name:     "another non-existent user",
			username: "viewer@example.com",
			wantErr:  true,
		},
		{
			name:     "non-existent user",
			username: "nonexistent@example.com",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := client.GetUserByUsername(ctx, tt.username)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, user)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.username, user.Email)
			}
		})
	}
}

func TestClient_UpdateUser(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	user := &User{
		Username:      fmt.Sprintf("updatetest-%d", timestamp),
		Email:         fmt.Sprintf("updatetest-%d@example.com", timestamp),
		EmailVerified: false,
		FirstName:     "Update",
		LastName:      "Test",
		Enabled:       true,
	}

	userID, err := client.CreateUser(ctx, user)
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := client.DeleteUser(context.Background(), userID); err != nil {
			t.Logf("Failed to cleanup test user: %v", err)
		}
	})

	retrievedUser, err := client.GetUser(ctx, userID)
	require.NoError(t, err)

	retrievedUser.FirstName = "Updated"
	retrievedUser.LastName = "Name"
	retrievedUser.EmailVerified = true

	err = client.UpdateUser(ctx, retrievedUser)
	require.NoError(t, err)

	updatedUser, err := client.GetUser(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", updatedUser.FirstName)
	assert.Equal(t, "Name", updatedUser.LastName)
	assert.True(t, updatedUser.EmailVerified)
}

func TestClient_DeleteUser(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	user := &User{
		Username:      fmt.Sprintf("deletetest-%d", timestamp),
		Email:         fmt.Sprintf("deletetest-%d@example.com", timestamp),
		EmailVerified: true,
		FirstName:     "Delete",
		LastName:      "Test",
		Enabled:       true,
	}

	userID, err := client.CreateUser(ctx, user)
	require.NoError(t, err)

	err = client.DeleteUser(ctx, userID)
	require.NoError(t, err)

	_, err = client.GetUser(ctx, userID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user not found")
}

func TestClient_ListUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()

	users, err := client.ListUsers(ctx, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, users)
	assert.GreaterOrEqual(t, len(users), 3)

	foundAdmin := false
	foundOperator := false
	foundViewer := false

	for _, user := range users {
		if user.Email == "admin@example.com" {
			foundAdmin = true
		}
		if user.Email == "operator@example.com" {
			foundOperator = true
		}
		if user.Email == "viewer@example.com" {
			foundViewer = true
		}
	}

	assert.True(t, foundAdmin, "admin user should be in the list")
	assert.True(t, foundOperator, "operator user should be in the list")
	assert.True(t, foundViewer, "viewer user should be in the list")
}

func TestClient_SetUserPassword(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	user := &User{
		Username:      fmt.Sprintf("pwdtest-%d", timestamp),
		Email:         fmt.Sprintf("pwdtest-%d@example.com", timestamp),
		EmailVerified: true,
		FirstName:     "Password",
		LastName:      "Test",
		Enabled:       true,
	}

	userID, err := client.CreateUser(ctx, user)
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := client.DeleteUser(context.Background(), userID); err != nil {
			t.Logf("Failed to cleanup test user: %v", err)
		}
	})

	err = client.SetUserPassword(ctx, userID, "temporary123", true)
	require.NoError(t, err)

	err = client.SetUserPassword(ctx, userID, "permanent456", false)
	require.NoError(t, err)

	tokenResp, err := client.DevExchangePasswordCredentials(ctx, user.Email, "permanent456")
	require.NoError(t, err)
	assert.NotNil(t, tokenResp)
	assert.NotEmpty(t, tokenResp.AccessToken)
}

func TestClient_UserAttributes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	timestamp := time.Now().Unix()

	user := &User{
		Username:      fmt.Sprintf("attrtest-%d", timestamp),
		Email:         fmt.Sprintf("attrtest-%d@example.com", timestamp),
		EmailVerified: true,
		FirstName:     "Attribute",
		LastName:      "Test",
		Enabled:       true,
		Attributes: map[string][]string{
			"tenantId": {"test-tenant"},
		},
	}

	userID, err := client.CreateUser(ctx, user)
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := client.DeleteUser(context.Background(), userID); err != nil {
			t.Logf("Failed to cleanup test user: %v", err)
		}
	})

	tenantID, err := client.GetUserAttribute(ctx, userID, "tenantId")
	require.NoError(t, err)
	assert.Equal(t, "test-tenant", tenantID)

	err = client.SetUserAttribute(ctx, userID, "testAttribute", "testValue")
	require.NoError(t, err)

	value, err := client.GetUserAttribute(ctx, userID, "testAttribute")
	require.NoError(t, err)
	assert.Equal(t, "testValue", value)
}
