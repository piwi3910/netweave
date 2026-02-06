// Package tenants provides commands for managing tenants.
package tenants

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/output"
	"github.com/piwi3910/netweave/internal/cli/service"
)

const (
	defaultRealm         = "netweave"
	defaultAdminUser     = "admin"
	defaultAdminPassword = "admin"
)

// NewTenantsCmd creates the parent "tenants" command.
func NewTenantsCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "tenants",
		Short: "Manage tenants",
		Long:  `Full CRUD operations for tenant management via Keycloak.`,
	}

	parent.AddCommand(newListCmd())
	parent.AddCommand(newGetCmd())
	parent.AddCommand(newCreateCmd())
	parent.AddCommand(newUpdateCmd())
	parent.AddCommand(newDeleteCmd())

	return parent
}

func connectKeycloak(ctx context.Context) (*service.KeycloakConnection, error) {
	kc, err := service.ConnectKeycloak(
		ctx,
		cmd.Global.Kubeconfig,
		cmd.Global.Namespace,
		defaultRealm,
		defaultAdminUser,
		defaultAdminPassword,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to keycloak: %w", err)
	}
	return kc, nil
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all tenants",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			kc, err := connectKeycloak(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			tenants, err := kc.Store.ListTenants(ctx)
			if err != nil {
				return fmt.Errorf("failed to list tenants: %w", err)
			}

			return cmd.Printer.PrintResult(tenants, func() {
				tbl := output.NewTable(
					cmd.Printer.Output(),
					"ID", "NAME", "STATUS", "DESCRIPTION",
				)
				for _, t := range tenants {
					tbl.AddRow(t.ID, t.Name, t.Status, t.Description)
				}
				tbl.Render()
			})
		},
	}
}

func newGetCmd() *cobra.Command {
	var tenantID string

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get tenant details",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if tenantID == "" {
				return fmt.Errorf("--id flag is required")
			}

			kc, err := connectKeycloak(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			tenant, err := kc.Store.GetTenant(ctx, tenantID)
			if err != nil {
				return fmt.Errorf("failed to get tenant: %w", err)
			}

			return cmd.Printer.PrintResult(tenant, func() {
				cmd.Printer.Infof("ID:            %s", tenant.ID)
				cmd.Printer.Infof("Name:          %s", tenant.Name)
				cmd.Printer.Infof("Status:        %s", tenant.Status)
				cmd.Printer.Infof("Description:   %s", tenant.Description)
				cmd.Printer.Infof("Contact Email: %s", tenant.ContactEmail)
				cmd.Printer.Infof("Created:       %s", tenant.CreatedAt)
				cmd.Printer.Infof("Quota:")
				cmd.Printer.Infof("  Max Subscriptions:     %d", tenant.Quota.MaxSubscriptions)
				cmd.Printer.Infof("  Max Resource Pools:    %d", tenant.Quota.MaxResourcePools)
				cmd.Printer.Infof("  Max Deployments:       %d", tenant.Quota.MaxDeployments)
				cmd.Printer.Infof("  Max Users:             %d", tenant.Quota.MaxUsers)
				cmd.Printer.Infof("  Max Requests/Min:      %d", tenant.Quota.MaxRequestsPerMinute)
			})
		},
	}

	getCmd.Flags().StringVar(&tenantID, "id", "", "Tenant ID (required)")
	return getCmd
}

func newCreateCmd() *cobra.Command {
	var (
		tenantID         string
		name             string
		description      string
		contactEmail     string
		maxSubscriptions int
		maxResourcePools int
		maxDeployments   int
		maxUsers         int
		maxReqPerMin     int
	)

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a tenant",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if tenantID == "" || name == "" {
				return fmt.Errorf("--id and --name flags are required")
			}

			kc, err := connectKeycloak(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			tenant := &auth.Tenant{
				ID:           tenantID,
				Name:         name,
				Description:  description,
				ContactEmail: contactEmail,
				Status:       auth.TenantStatusActive,
				Quota: auth.TenantQuota{
					MaxSubscriptions:     maxSubscriptions,
					MaxResourcePools:     maxResourcePools,
					MaxDeployments:       maxDeployments,
					MaxUsers:             maxUsers,
					MaxRequestsPerMinute: maxReqPerMin,
				},
			}

			if err := kc.Store.CreateTenant(ctx, tenant); err != nil {
				return fmt.Errorf("failed to create tenant: %w", err)
			}

			return cmd.Printer.PrintResult(tenant, func() {
				cmd.Printer.Infof("Tenant created: %s (ID: %s)", name, tenantID)
			})
		},
	}

	createCmd.Flags().StringVar(&tenantID, "id", "", "Tenant ID (required)")
	createCmd.Flags().StringVar(&name, "name", "", "Tenant display name (required)")
	createCmd.Flags().StringVar(&description, "description", "", "Tenant description")
	createCmd.Flags().StringVar(&contactEmail, "contact-email", "", "Contact email")
	createCmd.Flags().IntVar(&maxSubscriptions, "max-subscriptions", 100, "Max subscriptions")
	createCmd.Flags().IntVar(&maxResourcePools, "max-resource-pools", 50, "Max resource pools")
	createCmd.Flags().IntVar(&maxDeployments, "max-deployments", 100, "Max deployments")
	createCmd.Flags().IntVar(&maxUsers, "max-users", 50, "Max users")
	createCmd.Flags().IntVar(&maxReqPerMin, "max-requests-per-min", 1000, "Max requests per minute")
	return createCmd
}

func newUpdateCmd() *cobra.Command {
	var (
		tenantID    string
		name        string
		status      string
		description string
	)

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update a tenant",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if tenantID == "" {
				return fmt.Errorf("--id flag is required")
			}

			kc, err := connectKeycloak(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			tenant, err := kc.Store.GetTenant(ctx, tenantID)
			if err != nil {
				return fmt.Errorf("failed to get tenant: %w", err)
			}

			if name != "" {
				tenant.Name = name
			}
			if description != "" {
				tenant.Description = description
			}
			if status != "" {
				ts := auth.TenantStatus(status)
				if ts != auth.TenantStatusActive &&
					ts != auth.TenantStatusSuspended &&
					ts != auth.TenantStatusPendingDeletion {
					return fmt.Errorf(
						"--status must be 'active', 'suspended', or 'pending_deletion'",
					)
				}
				tenant.Status = ts
			}

			if err := kc.Store.UpdateTenant(ctx, tenant); err != nil {
				return fmt.Errorf("failed to update tenant: %w", err)
			}

			cmd.Printer.Infof("Tenant %s updated", tenantID)
			return nil
		},
	}

	updateCmd.Flags().StringVar(&tenantID, "id", "", "Tenant ID (required)")
	updateCmd.Flags().StringVar(&name, "name", "", "New display name")
	updateCmd.Flags().StringVar(&description, "description", "", "New description")
	updateCmd.Flags().StringVar(&status, "status", "", "New status: active, suspended, pending_deletion")
	return updateCmd
}

func newDeleteCmd() *cobra.Command {
	var tenantID string

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a tenant",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if tenantID == "" {
				return fmt.Errorf("--id flag is required")
			}

			kc, err := connectKeycloak(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			if err := kc.Store.DeleteTenant(ctx, tenantID); err != nil {
				return fmt.Errorf("failed to delete tenant: %w", err)
			}

			cmd.Printer.Infof("Tenant %s deleted", tenantID)
			return nil
		},
	}

	deleteCmd.Flags().StringVar(&tenantID, "id", "", "Tenant ID (required)")
	return deleteCmd
}
