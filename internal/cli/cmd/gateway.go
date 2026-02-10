// Package cmd defines the CLI command hierarchy for the netweave-cli tool.
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/service"
)

// GatewayFlags holds the shared gateway connection flags used by commands
// that interact with the gateway admin API.
type GatewayFlags struct {
	GatewayURL   string
	AuthURL      string
	Username     string
	Password     string
	ClientSecret string
}

// AddGatewayFlags registers the gateway connection flags on the given command.
func AddGatewayFlags(c *cobra.Command, gf *GatewayFlags) {
	pf := c.PersistentFlags()
	pf.StringVar(&gf.GatewayURL, "gateway-url", "https://api.netweave.local", "Gateway API URL")
	pf.StringVar(&gf.AuthURL, "auth-url", "https://auth.netweave.local", "Keycloak auth URL")
	pf.StringVar(&gf.Username, "username", "admin@netweave.local", "Admin username")
	pf.StringVar(&gf.Password, "password", "", "Admin password (required)")
	pf.StringVar(&gf.ClientSecret, "client-secret", "", "OAuth2 client secret (auto-read from K8s if empty)")
}

// MustMarkRequired marks a flag as required, panicking if the flag does not exist.
// This is safe because flag names are compile-time constants — a panic here
// indicates a programming error that would be caught immediately.
func MustMarkRequired(c *cobra.Command, name string) {
	if err := c.MarkFlagRequired(name); err != nil {
		panic(fmt.Sprintf("flag %q not found: %v", name, err))
	}
}

// ConnectGateway validates flags and establishes an authenticated connection
// to the gateway admin API.
func ConnectGateway(ctx context.Context, gf *GatewayFlags) (*service.GatewayConnection, error) {
	if gf.Password == "" {
		return nil, fmt.Errorf("--password flag is required")
	}

	gw, err := service.ConnectGateway(ctx, &service.GatewayConfig{
		GatewayURL:   gf.GatewayURL,
		AuthURL:      gf.AuthURL,
		Username:     gf.Username,
		Password:     gf.Password,
		ClientSecret: gf.ClientSecret,
		Namespace:    Global.Namespace,
		Kubeconfig:   Global.Kubeconfig,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gateway: %w", err)
	}

	return gw, nil
}
