// Package backends provides CLI commands for managing infrastructure backends.
package backends

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/output"
)

// backendResponse mirrors the API response for a backend instance.
type backendResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	AdapterType    string `json:"adapterType"`
	Description    string `json:"description,omitempty"`
	Status         string `json:"status"`
	StatusMessage  string `json:"statusMessage,omitempty"`
	HasCredentials bool   `json:"hasCredentials"`
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

// listResponse is the API response for listing backends.
type listResponse struct {
	Backends []backendResponse `json:"backends"`
	Total    int               `json:"total"`
}

// NewBackendsCmd creates the parent "backends" command.
func NewBackendsCmd() *cobra.Command {
	var gf cmd.GatewayFlags

	parent := &cobra.Command{
		Use:   "backends",
		Short: "Manage infrastructure backends",
		Long:  `CRUD operations for infrastructure backend instances (IMS/DMS).`,
	}

	cmd.AddGatewayFlags(parent, &gf)

	parent.AddCommand(newListCmd(&gf))
	parent.AddCommand(newGetCmd(&gf))
	parent.AddCommand(newCreateCmd(&gf))
	parent.AddCommand(newUpdateCmd(&gf))
	parent.AddCommand(newDeleteCmd(&gf))

	return parent
}

func newListCmd(gf *cmd.GatewayFlags) *cobra.Command {
	var category string

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all backends",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			gw, err := cmd.ConnectGateway(ctx, gf)
			if err != nil {
				return err
			}

			path := "/admin/infrastructure/backends"
			if category != "" {
				path += "?category=" + category
			}

			data, err := gw.Get(ctx, path)
			if err != nil {
				return fmt.Errorf("failed to list backends: %w", err)
			}

			var resp listResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			return cmd.Printer.PrintResult(resp, func() {
				tbl := output.NewTable(
					cmd.Printer.Output(),
					"ID", "NAME", "CATEGORY", "ADAPTER", "STATUS",
				)
				for _, b := range resp.Backends {
					tbl.AddRow(b.ID, b.Name, b.Category, b.AdapterType, b.Status)
				}
				tbl.Render()
			})
		},
	}

	listCmd.Flags().StringVar(&category, "category", "", "Filter by category (ims, dms)")
	return listCmd
}

func newGetCmd(gf *cmd.GatewayFlags) *cobra.Command {
	var backendID string

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get backend details",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			gw, err := cmd.ConnectGateway(ctx, gf)
			if err != nil {
				return err
			}

			data, err := gw.Get(ctx, "/admin/infrastructure/backends/"+backendID)
			if err != nil {
				return fmt.Errorf("failed to get backend: %w", err)
			}

			var b backendResponse
			if err := json.Unmarshal(data, &b); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			return cmd.Printer.PrintResult(b, func() {
				cmd.Printer.Infof("ID:              %s", b.ID)
				cmd.Printer.Infof("Name:            %s", b.Name)
				cmd.Printer.Infof("Category:        %s", b.Category)
				cmd.Printer.Infof("Adapter Type:    %s", b.AdapterType)
				cmd.Printer.Infof("Description:     %s", b.Description)
				cmd.Printer.Infof("Status:          %s", b.Status)
				cmd.Printer.Infof("Status Message:  %s", b.StatusMessage)
				cmd.Printer.Infof("Has Credentials: %v", b.HasCredentials)
				cmd.Printer.Infof("Created:         %s", b.CreatedAt)
				cmd.Printer.Infof("Updated:         %s", b.UpdatedAt)
			})
		},
	}

	getCmd.Flags().StringVar(&backendID, "id", "", "Backend ID")
	cmd.MustMarkRequired(getCmd, "id")
	return getCmd
}

func newCreateCmd(gf *cmd.GatewayFlags) *cobra.Command {
	var (
		name        string
		category    string
		adapterType string
		description string
		configPairs []string
	)

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a backend",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			gw, err := cmd.ConnectGateway(ctx, gf)
			if err != nil {
				return err
			}

			config := parseKeyValuePairs(configPairs)

			reqBody := map[string]interface{}{
				"name":        name,
				"category":    category,
				"adapterType": adapterType,
			}
			if description != "" {
				reqBody["description"] = description
			}
			if len(config) > 0 {
				reqBody["config"] = config
			}

			data, err := gw.Post(ctx, "/admin/infrastructure/backends", reqBody)
			if err != nil {
				return fmt.Errorf("failed to create backend: %w", err)
			}

			var b backendResponse
			if err := json.Unmarshal(data, &b); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			return cmd.Printer.PrintResult(b, func() {
				cmd.Printer.Infof("Backend created: %s (ID: %s)", b.Name, b.ID)
			})
		},
	}

	createCmd.Flags().StringVar(&name, "name", "", "Backend name")
	createCmd.Flags().StringVar(&category, "category", "", "Category: ims or dms")
	createCmd.Flags().StringVar(&adapterType, "adapter-type", "", "Adapter type, e.g. mock")
	createCmd.Flags().StringVar(&description, "description", "", "Backend description")
	createCmd.Flags().StringArrayVar(&configPairs, "config", nil, "Config key=value pairs (repeatable)")
	cmd.MustMarkRequired(createCmd, "name")
	cmd.MustMarkRequired(createCmd, "category")
	cmd.MustMarkRequired(createCmd, "adapter-type")
	return createCmd
}

func newUpdateCmd(gf *cmd.GatewayFlags) *cobra.Command {
	var (
		backendID   string
		name        string
		description string
		configPairs []string
	)

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update a backend",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			gw, err := cmd.ConnectGateway(ctx, gf)
			if err != nil {
				return err
			}

			reqBody := map[string]interface{}{}
			if name != "" {
				reqBody["name"] = name
			}
			if description != "" {
				reqBody["description"] = description
			}
			if len(configPairs) > 0 {
				reqBody["config"] = parseKeyValuePairs(configPairs)
			}

			data, err := gw.Put(ctx, "/admin/infrastructure/backends/"+backendID, reqBody)
			if err != nil {
				return fmt.Errorf("failed to update backend: %w", err)
			}

			var b backendResponse
			if err := json.Unmarshal(data, &b); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			return cmd.Printer.PrintResult(b, func() {
				cmd.Printer.Infof("Backend %s updated", backendID)
			})
		},
	}

	updateCmd.Flags().StringVar(&backendID, "id", "", "Backend ID")
	updateCmd.Flags().StringVar(&name, "name", "", "New name")
	updateCmd.Flags().StringVar(&description, "description", "", "New description")
	updateCmd.Flags().StringArrayVar(&configPairs, "config", nil, "Config key=value pairs (repeatable)")
	cmd.MustMarkRequired(updateCmd, "id")
	return updateCmd
}

func newDeleteCmd(gf *cmd.GatewayFlags) *cobra.Command {
	var backendID string

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a backend",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			gw, err := cmd.ConnectGateway(ctx, gf)
			if err != nil {
				return err
			}

			if err := gw.Delete(ctx, "/admin/infrastructure/backends/"+backendID); err != nil {
				return fmt.Errorf("failed to delete backend: %w", err)
			}

			cmd.Printer.Infof("Backend %s deleted", backendID)
			return nil
		},
	}

	deleteCmd.Flags().StringVar(&backendID, "id", "", "Backend ID")
	cmd.MustMarkRequired(deleteCmd, "id")
	return deleteCmd
}

// parseKeyValuePairs converts ["key1=value1", "key2=value2"] into a map.
func parseKeyValuePairs(pairs []string) map[string]string {
	result := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if found {
			result[key] = value
		}
	}
	return result
}
