// Package roles provides commands for managing roles and permissions.
package roles

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/output"
)

// NewRolesCmd creates the parent "roles" command.
func NewRolesCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "roles",
		Short: "Manage roles and permissions",
		Long:  `Full CRUD operations for role management via Keycloak.`,
	}

	parent.AddCommand(newListCmd())
	parent.AddCommand(newGetCmd())
	parent.AddCommand(newCreateCmd())
	parent.AddCommand(newUpdateCmd())
	parent.AddCommand(newDeleteCmd())

	return parent
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all roles",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			kc, err := cmd.ConnectKeycloakDefaults(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			roles, err := kc.Store.ListRoles(ctx)
			if err != nil {
				return fmt.Errorf("failed to list roles: %w", err)
			}

			return cmd.Printer.PrintResult(roles, func() {
				tbl := output.NewTable(
					cmd.Printer.Output(),
					"NAME", "TYPE", "DESCRIPTION", "PERMISSIONS",
				)
				for _, r := range roles {
					perms := formatPermissions(r.Permissions)
					tbl.AddRow(r.Name, r.Type, r.Description, perms)
				}
				tbl.Render()
			})
		},
	}
}

func newGetCmd() *cobra.Command {
	var roleName string

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get role details and permissions",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if roleName == "" {
				return fmt.Errorf("--name flag is required")
			}

			kc, err := cmd.ConnectKeycloakDefaults(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			role, err := kc.Store.GetRoleByName(ctx, auth.RoleName(roleName))
			if err != nil {
				return fmt.Errorf("failed to get role: %w", err)
			}

			return cmd.Printer.PrintResult(role, func() {
				cmd.Printer.Infof("Name:        %s", role.Name)
				cmd.Printer.Infof("Type:        %s", role.Type)
				cmd.Printer.Infof("Description: %s", role.Description)
				cmd.Printer.Infof("ID:          %s", role.ID)
				cmd.Printer.Infof("Tenant ID:   %s", role.TenantID)
				cmd.Printer.Infof("Created:     %s", role.CreatedAt)
				cmd.Printer.Infof("Permissions:")
				for _, p := range role.Permissions {
					cmd.Printer.Infof("  - %s", p)
				}
			})
		},
	}

	getCmd.Flags().StringVar(&roleName, "name", "", "Role name (required)")
	return getCmd
}

func newCreateCmd() *cobra.Command {
	var (
		roleName    string
		roleType    string
		description string
		permissions []string
	)

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a role",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if roleName == "" || roleType == "" {
				return fmt.Errorf("--name and --type flags are required")
			}

			rt := auth.RoleType(roleType)
			if rt != auth.RoleTypePlatform && rt != auth.RoleTypeTenant {
				return fmt.Errorf("--type must be 'platform' or 'tenant'")
			}

			kc, err := cmd.ConnectKeycloakDefaults(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			perms := make([]auth.Permission, 0, len(permissions))
			for _, p := range permissions {
				perms = append(perms, auth.Permission(p))
			}

			role := &auth.Role{
				Name:        auth.RoleName(roleName),
				Type:        rt,
				Description: description,
				Permissions: perms,
			}

			if err := kc.Store.CreateRole(ctx, role); err != nil {
				return fmt.Errorf("failed to create role: %w", err)
			}

			return cmd.Printer.PrintResult(role, func() {
				cmd.Printer.Infof("Role created: %s (ID: %s)", roleName, role.ID)
			})
		},
	}

	createCmd.Flags().StringVar(&roleName, "name", "", "Role name (required)")
	createCmd.Flags().StringVar(&roleType, "type", "tenant", "Role type: platform or tenant")
	createCmd.Flags().StringVar(&description, "description", "", "Role description")
	createCmd.Flags().StringSliceVar(&permissions, "permissions", nil, "Comma-separated list of permissions")
	return createCmd
}

func newUpdateCmd() *cobra.Command {
	var (
		roleName    string
		permissions []string
		description string
	)

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update role permissions",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if roleName == "" {
				return fmt.Errorf("--name flag is required")
			}

			kc, err := cmd.ConnectKeycloakDefaults(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			role, err := kc.Store.GetRoleByName(ctx, auth.RoleName(roleName))
			if err != nil {
				return fmt.Errorf("failed to get role: %w", err)
			}

			if description != "" {
				role.Description = description
			}
			if len(permissions) > 0 {
				perms := make([]auth.Permission, 0, len(permissions))
				for _, p := range permissions {
					perms = append(perms, auth.Permission(p))
				}
				role.Permissions = perms
			}

			if err := kc.Store.UpdateRole(ctx, role); err != nil {
				return fmt.Errorf("failed to update role: %w", err)
			}

			cmd.Printer.Infof("Role %s updated", roleName)
			return nil
		},
	}

	updateCmd.Flags().StringVar(&roleName, "name", "", "Role name (required)")
	updateCmd.Flags().StringVar(&description, "description", "", "New description")
	updateCmd.Flags().StringSliceVar(&permissions, "permissions", nil, "New permission list")
	return updateCmd
}

func newDeleteCmd() *cobra.Command {
	var roleName string

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a role",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if roleName == "" {
				return fmt.Errorf("--name flag is required")
			}

			kc, err := cmd.ConnectKeycloakDefaults(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			role, err := kc.Store.GetRoleByName(ctx, auth.RoleName(roleName))
			if err != nil {
				return fmt.Errorf("failed to get role: %w", err)
			}

			if err := kc.Store.DeleteRole(ctx, role.ID); err != nil {
				return fmt.Errorf("failed to delete role: %w", err)
			}

			cmd.Printer.Infof("Role %s deleted", roleName)
			return nil
		},
	}

	deleteCmd.Flags().StringVar(&roleName, "name", "", "Role name (required)")
	return deleteCmd
}

func formatPermissions(perms []auth.Permission) string {
	if len(perms) == 0 {
		return "(none)"
	}
	strs := make([]string, 0, len(perms))
	for _, p := range perms {
		strs = append(strs, string(p))
	}
	return strings.Join(strs, ", ")
}
