package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddGatewayFlags(t *testing.T) {
	c := &cobra.Command{Use: "test"}
	var gf GatewayFlags

	AddGatewayFlags(c, &gf)

	// Verify all flags are registered with correct defaults.
	gwURL, err := c.PersistentFlags().GetString("gateway-url")
	require.NoError(t, err)
	assert.Equal(t, "https://api.netweave.local", gwURL)

	authURL, err := c.PersistentFlags().GetString("auth-url")
	require.NoError(t, err)
	assert.Equal(t, "https://auth.netweave.local", authURL)

	username, err := c.PersistentFlags().GetString("username")
	require.NoError(t, err)
	assert.Equal(t, "admin@netweave.local", username)

	password, err := c.PersistentFlags().GetString("password")
	require.NoError(t, err)
	assert.Empty(t, password, "password default should be empty")

	clientSecret, err := c.PersistentFlags().GetString("client-secret")
	require.NoError(t, err)
	assert.Empty(t, clientSecret)
}

func TestAddGatewayFlags_FlagParsing(t *testing.T) {
	c := &cobra.Command{Use: "test", RunE: func(_ *cobra.Command, _ []string) error { return nil }}
	var gf GatewayFlags

	AddGatewayFlags(c, &gf)
	c.SetArgs([]string{
		"--gateway-url", "https://gw.example.com",
		"--auth-url", "https://auth.example.com",
		"--username", "user@example.com",
		"--password", "secret123",
		"--client-secret", "cs-abc",
	})

	err := c.Execute()
	require.NoError(t, err)

	assert.Equal(t, "https://gw.example.com", gf.GatewayURL)
	assert.Equal(t, "https://auth.example.com", gf.AuthURL)
	assert.Equal(t, "user@example.com", gf.Username)
	assert.Equal(t, "secret123", gf.Password)
	assert.Equal(t, "cs-abc", gf.ClientSecret)
}

func TestConnectGateway_EmptyPassword(t *testing.T) {
	gf := &GatewayFlags{
		GatewayURL: "https://gw.example.com",
		AuthURL:    "https://auth.example.com",
		Username:   "user@example.com",
		Password:   "",
	}

	_, err := ConnectGateway(t.Context(), gf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--password flag is required")
}
