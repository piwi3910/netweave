// Package plugins provides CLI commands for managing frontend plugins.
package plugins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/output"
)

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
	var gf cmd.GatewayFlags

	parent := &cobra.Command{
		Use:   "plugins",
		Short: "Manage frontend plugins",
		Long:  `List, enable, and disable frontend plugins.`,
	}

	cmd.AddGatewayFlags(parent, &gf)

	parent.AddCommand(newListCmd(&gf))
	parent.AddCommand(newEnableCmd(&gf))
	parent.AddCommand(newDisableCmd(&gf))

	return parent
}

func newListCmd(gf *cmd.GatewayFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all plugins",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			gw, err := cmd.ConnectGateway(ctx, gf)
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

func newEnableCmd(gf *cmd.GatewayFlags) *cobra.Command {
	var pluginName string

	enableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable a plugin",
		RunE: func(c *cobra.Command, _ []string) error {
			return setPluginState(c.Context(), gf, pluginName, true)
		},
	}

	enableCmd.Flags().StringVar(&pluginName, "name", "", "Plugin name")
	cmd.MustMarkRequired(enableCmd, "name")
	return enableCmd
}

func newDisableCmd(gf *cmd.GatewayFlags) *cobra.Command {
	var pluginName string

	disableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable a plugin",
		RunE: func(c *cobra.Command, _ []string) error {
			return setPluginState(c.Context(), gf, pluginName, false)
		},
	}

	disableCmd.Flags().StringVar(&pluginName, "name", "", "Plugin name")
	cmd.MustMarkRequired(disableCmd, "name")
	return disableCmd
}

func setPluginState(ctx context.Context, gf *cmd.GatewayFlags, pluginName string, enabled bool) error {
	gw, err := cmd.ConnectGateway(ctx, gf)
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
