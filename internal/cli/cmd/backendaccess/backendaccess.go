// Package backendaccess provides CLI commands for managing tenant backend access.
package backendaccess

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/output"
)

// accessEntry mirrors the API response for a backend access record.
type accessEntry struct {
	ID          string   `json:"id"`
	TenantID    string   `json:"tenantId"`
	BackendID   string   `json:"backendId"`
	Permissions []string `json:"permissions"`
	GrantedAt   string   `json:"grantedAt,omitempty"`
	GrantedBy   string   `json:"grantedBy,omitempty"`
}

// accessListResponse is the API response for listing backend access.
type accessListResponse struct {
	Access []accessEntry `json:"access"`
	Total  int           `json:"total"`
}

// NewBackendAccessCmd creates the parent "backend-access" command.
func NewBackendAccessCmd() *cobra.Command {
	var gf cmd.GatewayFlags

	parent := &cobra.Command{
		Use:   "backend-access",
		Short: "Manage tenant backend access",
		Long:  `Grant, list, and revoke tenant access to infrastructure backends.`,
	}

	cmd.AddGatewayFlags(parent, &gf)

	parent.AddCommand(newListCmd(&gf))
	parent.AddCommand(newGrantCmd(&gf))
	parent.AddCommand(newRevokeCmd(&gf))

	return parent
}

func newListCmd(gf *cmd.GatewayFlags) *cobra.Command {
	var tenantID string

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List backend access for a tenant",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			gw, err := cmd.ConnectGateway(ctx, gf)
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/admin/infrastructure/tenants/%s/backend-access", tenantID)
			data, err := gw.Get(ctx, path)
			if err != nil {
				return fmt.Errorf("failed to list backend access: %w", err)
			}

			var resp accessListResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			return cmd.Printer.PrintResult(resp, func() {
				tbl := output.NewTable(
					cmd.Printer.Output(),
					"ID", "BACKEND", "PERMISSIONS", "GRANTED BY", "GRANTED AT",
				)
				for _, a := range resp.Access {
					tbl.AddRow(
						a.ID,
						a.BackendID,
						strings.Join(a.Permissions, ", "),
						a.GrantedBy,
						a.GrantedAt,
					)
				}
				tbl.Render()
			})
		},
	}

	listCmd.Flags().StringVar(&tenantID, "tenant", "", "Tenant ID")
	cmd.MustMarkRequired(listCmd, "tenant")
	return listCmd
}

func newGrantCmd(gf *cmd.GatewayFlags) *cobra.Command {
	var (
		tenantID    string
		backendID   string
		permissions string
	)

	grantCmd := &cobra.Command{
		Use:   "grant",
		Short: "Grant tenant access to a backend",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			gw, err := cmd.ConnectGateway(ctx, gf)
			if err != nil {
				return err
			}

			permList := strings.Split(permissions, ",")
			for i := range permList {
				permList[i] = strings.TrimSpace(permList[i])
			}

			reqBody := map[string]interface{}{
				"backendId":   backendID,
				"permissions": permList,
			}

			path := fmt.Sprintf("/admin/infrastructure/tenants/%s/backend-access", tenantID)
			data, err := gw.Post(ctx, path, reqBody)
			if err != nil {
				return fmt.Errorf("failed to grant backend access: %w", err)
			}

			var a accessEntry
			if err := json.Unmarshal(data, &a); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			return cmd.Printer.PrintResult(a, func() {
				cmd.Printer.Infof("Backend access granted (ID: %s)", a.ID)
				cmd.Printer.Infof("  Tenant:      %s", a.TenantID)
				cmd.Printer.Infof("  Backend:     %s", a.BackendID)
				cmd.Printer.Infof("  Permissions: %s", strings.Join(a.Permissions, ", "))
			})
		},
	}

	grantCmd.Flags().StringVar(&tenantID, "tenant", "", "Tenant ID")
	grantCmd.Flags().StringVar(&backendID, "backend", "", "Backend ID")
	grantCmd.Flags().StringVar(
		&permissions, "permissions", "",
		"Comma-separated permissions, e.g. read,subscribe",
	)
	cmd.MustMarkRequired(grantCmd, "tenant")
	cmd.MustMarkRequired(grantCmd, "backend")
	cmd.MustMarkRequired(grantCmd, "permissions")
	return grantCmd
}

func newRevokeCmd(gf *cmd.GatewayFlags) *cobra.Command {
	var (
		tenantID string
		accessID string
	)

	revokeCmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke tenant backend access",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			gw, err := cmd.ConnectGateway(ctx, gf)
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/admin/infrastructure/tenants/%s/backend-access/%s", tenantID, accessID)
			if err := gw.Delete(ctx, path); err != nil {
				return fmt.Errorf("failed to revoke backend access: %w", err)
			}

			cmd.Printer.Infof("Backend access %s revoked for tenant %s", accessID, tenantID)
			return nil
		},
	}

	revokeCmd.Flags().StringVar(&tenantID, "tenant", "", "Tenant ID")
	revokeCmd.Flags().StringVar(&accessID, "id", "", "Access ID")
	cmd.MustMarkRequired(revokeCmd, "tenant")
	cmd.MustMarkRequired(revokeCmd, "id")
	return revokeCmd
}
