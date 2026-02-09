// Package plugins provides CLI commands for managing frontend plugins.
package plugins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/output"
	"github.com/piwi3910/netweave/internal/cli/service"
)

// gatewayFlags holds the shared gateway connection flags.
type gatewayFlags struct {
	gatewayURL   string
	authURL      string
	username     string
	password     string
	clientSecret string
}

func addGatewayFlags(c *cobra.Command, gf *gatewayFlags) {
	pf := c.PersistentFlags()
	pf.StringVar(&gf.gatewayURL, "gateway-url", "https://api.netweave.local", "Gateway API URL")
	pf.StringVar(&gf.authURL, "auth-url", "https://auth.netweave.local", "Keycloak auth URL")
	pf.StringVar(&gf.username, "username", "admin@netweave.local", "Admin username")
	pf.StringVar(&gf.password, "password", "admin", "Admin password")
	pf.StringVar(&gf.clientSecret, "client-secret", "", "OAuth2 client secret (auto-read from K8s if empty)")
}

func connectGateway(ctx context.Context, gf *gatewayFlags) (*service.GatewayConnection, error) {
	gw, err := service.ConnectGateway(ctx, &service.GatewayConfig{
		GatewayURL:   gf.gatewayURL,
		AuthURL:      gf.authURL,
		Username:     gf.username,
		Password:     gf.password,
		ClientSecret: gf.clientSecret,
		Namespace:    cmd.Global.Namespace,
		Kubeconfig:   cmd.Global.Kubeconfig,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gateway: %w", err)
	}
	return gw, nil
}

// pluginInfo mirrors the API response for a plugin.
type pluginInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Enabled     bool     `json:"enabled"`
	BasePaths   []string `json:"basePaths"`
}

// pluginListResponse is the API response for listing plugins.
type pluginListResponse struct {
	Plugins []pluginInfo `json:"plugins"`
}

// NewPluginsCmd creates the parent "plugins" command.
func NewPluginsCmd() *cobra.Command {
	var gf gatewayFlags

	parent := &cobra.Command{
		Use:   "plugins",
		Short: "Manage frontend plugins",
		Long:  `List, enable, and disable frontend plugins.`,
	}

	addGatewayFlags(parent, &gf)

	parent.AddCommand(newListCmd(&gf))
	parent.AddCommand(newEnableCmd(&gf))
	parent.AddCommand(newDisableCmd(&gf))

	return parent
}

func newListCmd(gf *gatewayFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all plugins",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			gw, err := connectGateway(ctx, gf)
			if err != nil {
				return err
			}

			data, err := gw.Get(ctx, "/admin/platform/plugins")
			if err != nil {
				return fmt.Errorf("failed to list plugins: %w", err)
			}

			var resp pluginListResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			return cmd.Printer.PrintResult(resp, func() {
				tbl := output.NewTable(
					cmd.Printer.Output(),
					"NAME", "DISPLAY NAME", "ENABLED", "BASE PATHS",
				)
				for _, p := range resp.Plugins {
					enabled := "no"
					if p.Enabled {
						enabled = "yes"
					}
					tbl.AddRow(p.Name, p.DisplayName, enabled, fmt.Sprintf("%v", p.BasePaths))
				}
				tbl.Render()
			})
		},
	}
}

func newEnableCmd(gf *gatewayFlags) *cobra.Command {
	var pluginName string

	enableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable a plugin",
		RunE: func(c *cobra.Command, _ []string) error {
			return setPluginState(c.Context(), gf, pluginName, true)
		},
	}

	enableCmd.Flags().StringVar(&pluginName, "name", "", "Plugin name (required)")
	return enableCmd
}

func newDisableCmd(gf *gatewayFlags) *cobra.Command {
	var pluginName string

	disableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable a plugin",
		RunE: func(c *cobra.Command, _ []string) error {
			return setPluginState(c.Context(), gf, pluginName, false)
		},
	}

	disableCmd.Flags().StringVar(&pluginName, "name", "", "Plugin name (required)")
	return disableCmd
}

func setPluginState(ctx context.Context, gf *gatewayFlags, pluginName string, enabled bool) error {
	if pluginName == "" {
		return fmt.Errorf("--name flag is required")
	}

	gw, err := connectGateway(ctx, gf)
	if err != nil {
		return err
	}

	reqBody := map[string]interface{}{
		"enabled": enabled,
	}

	data, err := gw.Put(ctx, "/admin/platform/plugins/"+pluginName, reqBody)
	if err != nil {
		action := "enable"
		if !enabled {
			action = "disable"
		}
		return fmt.Errorf("failed to %s plugin: %w", action, err)
	}

	var p pluginInfo
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return cmd.Printer.PrintResult(p, func() {
		action := "enabled"
		if !enabled {
			action = "disabled"
		}
		cmd.Printer.Infof("Plugin %q %s", p.Name, action)
	})
}
