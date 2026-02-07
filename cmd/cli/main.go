// Package main is the entry point for the netweave-cli tool.
// It provides commands for deploying, managing, and interacting with
// the Netweave O2-IMS gateway.
package main

import (
	"fmt"
	"os"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	apicommands "github.com/piwi3910/netweave/internal/cli/cmd/api"
	"github.com/piwi3910/netweave/internal/cli/cmd/certs"
	"github.com/piwi3910/netweave/internal/cli/cmd/roles"
	"github.com/piwi3910/netweave/internal/cli/cmd/setup"
	"github.com/piwi3910/netweave/internal/cli/cmd/tenants"
	"github.com/piwi3910/netweave/internal/cli/cmd/users"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	root := cmd.NewRootCmd()

	// Register subcommands
	root.AddCommand(cmd.NewVersionCmd())
	root.AddCommand(setup.NewSetupCmd())
	root.AddCommand(apicommands.NewAPICmd())
	root.AddCommand(users.NewUsersCmd())
	root.AddCommand(roles.NewRolesCmd())
	root.AddCommand(tenants.NewTenantsCmd())
	root.AddCommand(certs.NewCertsCmd())

	if err := root.Execute(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}
