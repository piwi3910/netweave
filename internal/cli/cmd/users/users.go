// Package users provides commands for managing Keycloak users.
package users

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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

// NewUsersCmd creates the parent "users" command.
func NewUsersCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "users",
		Short: "Manage users",
		Long:  `Full CRUD operations for user management via Keycloak.`,
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
	var tenantID string

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List users",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			kc, err := connectKeycloak(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			if tenantID == "" {
				return fmt.Errorf("--tenant flag is required")
			}

			users, err := kc.Store.ListUsersByTenant(ctx, tenantID)
			if err != nil {
				return fmt.Errorf("failed to list users: %w", err)
			}

			return cmd.Printer.PrintResult(users, func() {
				tbl := output.NewTable(
					cmd.Printer.Output(),
					"ID", "COMMON NAME", "EMAIL", "ROLE ID", "ACTIVE",
				)
				for _, u := range users {
					tbl.AddRow(
						u.ID, u.CommonName, u.Email,
						u.RoleID, u.IsActive,
					)
				}
				tbl.Render()
			})
		},
	}

	listCmd.Flags().StringVar(&tenantID, "tenant", "", "Tenant ID (required)")
	return listCmd
}

func newGetCmd() *cobra.Command {
	var userID string

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get user details",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if userID == "" {
				return fmt.Errorf("--id flag is required")
			}

			kc, err := connectKeycloak(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			user, err := kc.Store.GetUser(ctx, userID)
			if err != nil {
				return fmt.Errorf("failed to get user: %w", err)
			}

			return cmd.Printer.PrintResult(user, func() {
				cmd.Printer.Infof("ID:          %s", user.ID)
				cmd.Printer.Infof("Tenant ID:   %s", user.TenantID)
				cmd.Printer.Infof("Common Name: %s", user.CommonName)
				cmd.Printer.Infof("Email:       %s", user.Email)
				cmd.Printer.Infof("Subject:     %s", user.Subject)
				cmd.Printer.Infof("Role ID:     %s", user.RoleID)
				cmd.Printer.Infof("Active:      %v", user.IsActive)
				cmd.Printer.Infof("Created:     %s", user.CreatedAt)
			})
		},
	}

	getCmd.Flags().StringVar(&userID, "id", "", "User ID (required)")
	return getCmd
}

func newCreateCmd() *cobra.Command {
	var (
		tenantID string
		certCN   string
		email    string
		roleName string
	)

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if tenantID == "" || certCN == "" || email == "" || roleName == "" {
				return fmt.Errorf(
					"--tenant, --cert-cn, --email, and --role flags are required",
				)
			}

			kc, err := connectKeycloak(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			role, err := kc.Store.GetRoleByName(ctx, auth.RoleName(roleName))
			if err != nil {
				return fmt.Errorf("failed to find role %q: %w", roleName, err)
			}

			user := &auth.TenantUser{
				ID:         uuid.New().String(),
				TenantID:   tenantID,
				CommonName: certCN,
				Email:      email,
				Subject:    fmt.Sprintf("CN=%s,O=Netweave", certCN),
				RoleID:     role.ID,
				IsActive:   true,
			}

			if err := kc.Store.CreateUser(ctx, user); err != nil {
				return fmt.Errorf("failed to create user: %w", err)
			}

			return cmd.Printer.PrintResult(user, func() {
				cmd.Printer.Infof("User created: %s (ID: %s)", certCN, user.ID)
			})
		},
	}

	createCmd.Flags().StringVar(&tenantID, "tenant", "", "Tenant ID (required)")
	createCmd.Flags().StringVar(&certCN, "cert-cn", "", "Certificate CN (required)")
	createCmd.Flags().StringVar(&email, "email", "", "Email address (required)")
	createCmd.Flags().StringVar(&roleName, "role", "", "Role name (required)")
	return createCmd
}

func newUpdateCmd() *cobra.Command {
	var (
		userID   string
		email    string
		tenantID string
		roleName string
	)

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update a user",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if userID == "" {
				return fmt.Errorf("--id flag is required")
			}

			kc, err := connectKeycloak(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			user, err := kc.Store.GetUser(ctx, userID)
			if err != nil {
				return fmt.Errorf("failed to get user: %w", err)
			}

			if email != "" {
				user.Email = email
			}
			if tenantID != "" {
				user.TenantID = tenantID
			}
			if roleName != "" {
				role, roleErr := kc.Store.GetRoleByName(ctx, auth.RoleName(roleName))
				if roleErr != nil {
					return fmt.Errorf("failed to find role %q: %w", roleName, roleErr)
				}
				user.RoleID = role.ID
			}

			if err := kc.Store.UpdateUser(ctx, user); err != nil {
				return fmt.Errorf("failed to update user: %w", err)
			}

			cmd.Printer.Infof("User %s updated", userID)
			return nil
		},
	}

	updateCmd.Flags().StringVar(&userID, "id", "", "User ID (required)")
	updateCmd.Flags().StringVar(&email, "email", "", "New email address")
	updateCmd.Flags().StringVar(&tenantID, "tenant", "", "New tenant ID")
	updateCmd.Flags().StringVar(&roleName, "role", "", "New role name")
	return updateCmd
}

func newDeleteCmd() *cobra.Command {
	var userID string

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a user",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if userID == "" {
				return fmt.Errorf("--id flag is required")
			}

			kc, err := connectKeycloak(ctx)
			if err != nil {
				return err
			}
			defer kc.Close()

			if err := kc.Store.DeleteUser(ctx, userID); err != nil {
				return fmt.Errorf("failed to delete user: %w", err)
			}

			cmd.Printer.Infof("User %s deleted", userID)
			return nil
		},
	}

	deleteCmd.Flags().StringVar(&userID, "id", "", "User ID (required)")
	return deleteCmd
}
